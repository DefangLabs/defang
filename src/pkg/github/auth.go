package github

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/google/uuid"
	"github.com/pkg/browser"
)

const (
	authTemplateString = `<!DOCTYPE html>
<html>
	<head>
		<title>Defang | Authentication Status</title>
		<style>
			body {
				font-family: 'Exo', sans-serif;
				background: linear-gradient(to right, #1e3c72, #2a5298);
				color: white;
				display: flex;
				justify-content: center;
				align-items: center;
				height: 100vh;
				margin: 0;
			}
			.container {
				text-align: center;
			}
			.status-message {
				font-size: 2em;
				margin-bottom: 1em;
			}
			.close-link {
				font-size: 1em;
				cursor: pointer;
			}
		</style>
	</head>
	<body>
		<div class="container">
			<h1>Welcome to Defang</h1>
			<p class="status-message">{{.StatusMessage}}</p>
			<p class="close-link" onclick="window.close()">You can close this window.</p>
		</div>
	</body>
</html>`
)

var (
	authTemplate = template.Must(template.New("auth").Parse(authTemplateString))
)

func StartAuthCodeFlow(ctx context.Context, clientId string, prompt bool) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Generate random state
	state := uuid.NewString()

	// Create a channel to wait for the server to finish
	ch := make(chan string)
	defer close(ch)

	var authorizeUrl string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, authorizeUrl, http.StatusFound)
			return
		}
		if r.URL.Path != "/auth" {
			http.NotFound(w, r)
			return
		}
		var msg string
		query := r.URL.Query()
		if query.Get("state") != state {
			msg = "Authentication error: wrong state"
		} else {
			msg = "Authentication successful"
			if query.Get("error") != "" {
				msg = "Authentication failed: " + query.Get("error_description")
			}
			ch <- query.Get("code")
		}
		authTemplate.Execute(w, struct{ StatusMessage string }{msg})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	values := url.Values{
		"client_id":    {clientId},
		"state":        {state},
		"redirect_uri": {server.URL + "/auth"},
		"scope":        {"read:org user:email"}, // required for membership check; space-delimited
		// "login":     {";TODO: from state file"},
	}
	authorizeUrl = "https://github.com/login/oauth/authorize?" + values.Encode()

	// TODO:This is used to open the browser for GitHub Auth before blocking
	if !prompt {
		browser.OpenURL(server.URL)
	} else {
		n, _ := term.Printf("Please visit %s and log in. (Right click the URL or press ENTER to open browser)\r", server.URL)
		defer term.Print(strings.Repeat(" ", n), "\r") // TODO: use termenv to clear line
		input := term.NewNonBlockingStdin()
		defer input.Close() // abort the read
		go func() {
			var b [1]byte
			for {
				if _, err := input.Read(b[:]); err != nil {
					return // exit goroutine
				}
				switch b[0] {
				case 3: // Ctrl-C
					cancel()
				case 10, 13: // Enter or Return
					browser.OpenURL(server.URL)
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case code := <-ch:
		return code, nil
	}
}
