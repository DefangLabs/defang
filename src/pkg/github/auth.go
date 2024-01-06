package github

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/google/uuid"
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

func StartAuthCodeFlow(ctx context.Context, clientId string) (string, error) {
	// Generate random state
	state := uuid.NewString()

	// Create a channel to wait for the server to finish
	ch := make(chan string)

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
		defer close(ch)
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		msg := "Authentication successful"
		if query.Get("error") != "" {
			msg = "Authentication failed: " + query.Get("error_description")
		}
		ch <- query.Get("code")
		authTemplate.Execute(w, struct{ StatusMessage string }{msg})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	values := url.Values{
		"client_id":    {clientId},
		"state":        {state},
		"redirect_uri": {server.URL + "/auth"},
		"scope":        {"read:org"}, // required for membership check
		// "login":     {"TODO: from state file"},
	}
	authorizeUrl = "https://github.com/login/oauth/authorize?" + values.Encode()

	n, _ := fmt.Printf("Please visit %s and log in. (Right click to open)\r", server.URL)
	defer fmt.Print(strings.Repeat(" ", n), "\r")

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case code := <-ch:
		return code, nil
	}
}
