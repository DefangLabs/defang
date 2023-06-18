package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type RequestInfo struct {
	Path    string            `json:"path"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Args    map[string]string `json:"args"`
	Body    string            `json:"body"`
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		headers := make(map[string]string)
		for name, values := range r.Header {
			for _, value := range values {
				headers[name] = value
			}
		}
		args := make(map[string]string)
		for name, values := range r.URL.Query() {
			for _, value := range values {
				args[name] = value
			}
		}
		info := RequestInfo{
			Path:    r.URL.Path,
			Method:  r.Method,
			Headers: headers,
			Args:    args,
			Body:    string(body),
		}
		jsonResponse, _ := json.Marshal(info)

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})

	http.ListenAndServe(":8080", nil)
}
