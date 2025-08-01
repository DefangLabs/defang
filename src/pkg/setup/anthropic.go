package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Request structures
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
}

// Response structures
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ClaudeResponse struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Role    string    `json:"role"`
	Content []Content `json:"content"`
	Model   string    `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type ClaudeClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewClaudeClient(apiKey string) *ClaudeClient {
	return &ClaudeClient{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com/v1",
		client:  &http.Client{},
	}
}

// Example function for more advanced usage with conversation history
func (c *ClaudeClient) SendConversation(system string, messages []Message) (*ClaudeResponse, error) {
	request := ClaudeRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 2048,
		Messages:  messages,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &claudeResp, nil
}
