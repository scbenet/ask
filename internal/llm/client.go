package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LLMClient defines the interface for interacting with an LLM.
type LLMClient interface {
	Generate(ctx context.Context, modelName string, prompt string, history []Message) (string, error)
	StreamGenerate(ctx context.Context, modelName string, history []Message, msgChan chan<- tea.Msg)
}

type GenerationErrorMsg struct{ Err error }

// Message struct for conversation history
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMReplyMsg struct{ Content string }

type StreamChunkMsg struct{ Content string }
type StreamEndMsg struct{ FullResponse string }
type StreamErrorMsg struct{ Err error }

type OpenRouterClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

type OpenRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// single choice's non-streaming response message content
type OpenRouterResponseChoiceMessage struct {
	Content string `json:"content"`
}

// single choice in a non-streaming response
type OpenRouterResponseChoice struct {
	Message OpenRouterResponseChoiceMessage `json:"message"`
}

type OpenRouterResponseError struct {
	Message string `json:"message"`
}

// for non-streaming responses
type OpenRouterResponse struct {
	Choices []OpenRouterResponseChoice `json:"choices"`
	Error   *OpenRouterResponseError   `json:"error,omitempty"`
}

// holds content difference in a stream chunk
type OpenRouterStreamDelta struct {
	Content string `json:"content"`
}

// holds a choice in a stream chunk
type OpenRouterStreamChoice struct {
	Delta        OpenRouterStreamDelta `json:"delta"`
	FinishReason *string               `json:"finish_reason,omitempty"`
}

// structure of an individual SSE data event
type OpenRouterStreamChunk struct {
	Choices []OpenRouterStreamChoice `json:"choices"`
	Error   *OpenRouterResponseError `json:"error,omitempty"` // check for errors in chunks too
}

func NewOpenRouterClient() (*OpenRouterClient, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENROUTER_API_KEY environment variable not set")
	}

	return &OpenRouterClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 360 * time.Second},
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
		Stream:   false,
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

func (c *OpenRouterClient) StreamGenerate(ctx context.Context, modelName string, historyWithLatestPrompt []Message, msgChan chan<- tea.Msg) {
	go func() {
		defer close(msgChan) // close channel when done to signal end of stream

		requestBody := OpenRouterRequest{
			Model:    modelName,
			Messages: historyWithLatestPrompt,
			Stream:   true,
		}

		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			msgChan <- StreamErrorMsg{Err: fmt.Errorf("failed to marshal stream request: %w", err)}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			msgChan <- StreamErrorMsg{Err: fmt.Errorf("failed to created stream HTTP request: %w", err)}
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
		req.Header.Set("HTTP-Referer", "https://github.com/scbenet/ask")
		req.Header.Set("X-Title", "Ask CLI")

		log.Printf("sending streaming request to OpenRouter for model: %s with %d messages", modelName, len(historyWithLatestPrompt))
		resp, err := c.httpClient.Do(req)
		if err != nil {
			msgChan <- StreamErrorMsg{Err: fmt.Errorf("stream HTTP request failed: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body) // read body for error details
			msgChan <- StreamErrorMsg{Err: fmt.Errorf("API stream request failed with status %d: %s", resp.StatusCode, string(bodyBytes))}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		var fullResponseContent strings.Builder
		CHUNK_PREFIX := "data: " // data chunks are prefixed with this, indicates a valid response chunk

		// track if we've seen a response error in a stream chunk so far
		// this gives us a bit of leeway, will attempt to keep reading after the first bad chunk
		// but if we encounter a second error, send an ErrorMsg and return
		responseStreamingErrorSeen := false

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			if strings.HasPrefix(line, CHUNK_PREFIX) {
				jsonDataStr := strings.TrimPrefix(line, CHUNK_PREFIX)
				if jsonDataStr == "[DONE]" {
					log.Println("stream indicated [DONE]")
					break
				}

				var chunk OpenRouterStreamChunk
				if err := json.Unmarshal([]byte(jsonDataStr), &chunk); err != nil {
					log.Printf("Error unmarshalling stream chunk JSON: '%s', data: %s", err, jsonDataStr)
					if responseStreamingErrorSeen {
						msgChan <- StreamErrorMsg{Err: fmt.Errorf("error unmarshalling stream chunk: %w (data: %s)", err, jsonDataStr)}
						return
					}

					responseStreamingErrorSeen = true
					continue
				}

				if chunk.Error != nil {
					log.Printf("Error in stream chunk: %s", chunk.Error.Message)
					msgChan <- StreamErrorMsg{Err: fmt.Errorf("API error in stream chunk: %s", chunk.Error.Message)}
					return
				}

				if len(chunk.Choices) > 0 {
					content := chunk.Choices[0].Delta.Content
					if content != "" {
						fullResponseContent.WriteString(content)
						msgChan <- StreamChunkMsg{Content: content}
					}
					if chunk.Choices[0].FinishReason != nil {
						log.Printf("stream chunk indicates FinishReason: %s", *chunk.Choices[0].FinishReason)
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			msgChan <- StreamErrorMsg{Err: fmt.Errorf("error reading stream: %w", err)}
			return
		}

		log.Println("stream processing finished")
		msgChan <- StreamEndMsg{FullResponse: fullResponseContent.String()}
	}()
}
