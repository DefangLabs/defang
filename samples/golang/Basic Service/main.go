package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	// Register signal handler for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	server := &http.Server{Addr: ":8080"}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server Serve: %v\n", err)
		}
	}()

	sig := <-sigs
	log.Printf("Received signal %v, shutting down...\n", sig)
	log.Fatal(server.Shutdown(context.Background()))
}
