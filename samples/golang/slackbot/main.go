package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/slack-go/slack"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// if method is GET, return ok
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}

		// only proceed if the request is a POST request
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintf(w, "This endpoint only supports POST requests")
			return
		}

		// only proceed if the request body is json
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			fmt.Fprintf(w, "This endpoint only supports application/json")
			return
		}

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error reading request body: %s", err)
			return
		}

		// Parse the JSON body
		var data map[string]string
		err = json.Unmarshal(body, &data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error parsing JSON: %s", err)
			return
		}

		// Use the value of the "message" key to post a message to Slack
		message := data["message"]
		fmt.Println(message)

		api := slack.New(os.Getenv("SLACK_TOKEN"))

		_, _, err = api.PostMessage(
			os.Getenv("SLACK_CHANNEL_ID"),
			slack.MsgOptionText(
				message,
				false,
			),
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error posting: %s", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.ListenAndServe(":8080", nil)
}
