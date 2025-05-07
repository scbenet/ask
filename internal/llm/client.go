package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// LLMClient defines the interface for interacting with an LLM.
type LLMClient interface {
	Generate(ctx context.Context, modelName string, prompt string, history []Message) (string, error)
}

type ErrorMsg struct{ Err error }

// Message struct for conversation history
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

type OpenRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewOpenRouterClient() (*OpenRouterClient, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENROUTER_API_KEY environment variable not set")
	}

	return &OpenRouterClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		baseURL:    "https://openrouter.ai/api/v1/chat/completions",
	}, nil
}

func (c *OpenRouterClient) Generate(ctx context.Context, modelName string, prompt string, history []Message) (string, error) {
	// create message array with user's prompt
	var messages []Message

	// First add any history if available
	if len(history) > 0 {
		messages = append(messages, history...)
	}

	// Then add the current prompt
	messages = append(messages, Message{
		Role:    "user",
		Content: prompt,
	})

	// create request body
	requestBody := OpenRouterRequest{
		Model:    modelName,
		Messages: messages,
	}

	// marshal request to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)

	}

	// create http request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	req.Header.Set("HTTP-Referer", "https://github.com/scbenet/ask")
	req.Header.Set("X-Title", "Ask CLI")

	// make http request
	log.Printf("Sending request to openrouter for model : %s with %d messages", modelName, len(messages))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// parse response JSON
	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// check for API error
	if openRouterResp.Error != nil {
		return "", fmt.Errorf("API error: %s", openRouterResp.Error.Message)
	}

	// check if we have valid choices
	if len(openRouterResp.Choices) == 0 {
		return "", errors.New("no response choices returned")
	}

	// return first choice
	return openRouterResp.Choices[0].Message.Content, nil
}
