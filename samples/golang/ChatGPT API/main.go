package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func Index(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func Prompt(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := ioutil.ReadAll(r.Body)
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

	body, _ := ioutil.ReadAll(resp.Body)

	var apiResponse interface{}
	json.Unmarshal(body, &apiResponse)

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "response": apiResponse})
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/", Index)
	r.HandleFunc("/prompt", Prompt).Methods("POST")

	http.ListenAndServe(":8080", r)
}
