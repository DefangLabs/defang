package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func Index(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func Prompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}

	reqBody, _ := io.ReadAll(r.Body)
	promptText := string(reqBody)

	messages := []Message{
		//{Role: "system", Content: "You are an experienced software engineer."},
		{Role: "user", Content: promptText},
	}

	apiKey := os.Getenv("OPENAI_KEY")
	url := "https://api.openai.com/v1/chat/completions"

	postBody, _ := json.Marshal(map[string]interface{}{
		"model":    "gpt-3.5-turbo",
		"messages": messages,
	})

	responseBody := bytes.NewBuffer(postBody)

	client := &http.Client{}

	req, _ := http.NewRequest("POST", url, responseBody)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, _ := client.Do(req)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var apiResponse interface{}
	json.Unmarshal(body, &apiResponse)

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "response": apiResponse})
}

func main() {
	http.HandleFunc("/", Index)
	http.HandleFunc("/prompt", Prompt)

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
