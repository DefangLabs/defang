package github

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	pkg "github.com/defang-io/defang/src/pkg"
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

func StartDeviceFlow(ctx context.Context, clientId string) (url.Values, error) {
	codeUrl := "https://github.com/login/device/code?client_id=" + clientId
	q, err := pkg.PostForValues(codeUrl, "application/json", nil)
	if err != nil {
		return nil, err
	}

	interval, err := strconv.Atoi(q.Get("interval"))
	if err != nil {
		return nil, err
	}

	fmt.Printf("Please visit %s and enter the code %s\n", q.Get("verification_uri"), q.Get("user_code"))

	values := url.Values{
		"client_id":   {clientId},
		"device_code": q["device_code"],
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	accessTokenUrl := "https://github.com/login/oauth/access_token?" + values.Encode()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		time.Sleep(time.Duration(interval) * time.Second)

		q, err := pkg.PostForValues(accessTokenUrl, "application/json", nil)
		if err != nil || q.Get("error") != "" {
			switch q.Get("error") {
			case "authorization_pending":
				continue
			case "slow_down":
				if interval, err = strconv.Atoi(q.Get("interval")); err == nil {
					continue
				}
			}
			return nil, fmt.Errorf("%w: %v", err, q.Get("error_description"))
		}

		return q, nil
	}
}

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
		// "login":     {";TODO: from state file"},
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
