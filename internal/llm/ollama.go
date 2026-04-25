package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaClient calls the local Ollama REST API.
type OllamaClient struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllamaClient creates an Ollama-backed LLM client.
// endpoint defaults to "http://localhost:11434" if empty.
func NewOllamaClient(endpoint, model string) *OllamaClient {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaClient{
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{},
	}
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

// Complete sends systemPrompt + userPrompt to the local Ollama instance.
func (o *OllamaClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := ollamaRequest{
		Model: o.model,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling ollama request: %w", err)
	}

	url := o.endpoint + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama API call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("parsing ollama response: %w", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}
	if ollamaResp.Message.Content == "" {
		return "", fmt.Errorf("ollama returned empty content")
	}

	return ollamaResp.Message.Content, nil
}
