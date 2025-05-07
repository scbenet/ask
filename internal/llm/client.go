package llm

import (
	"context"
	"fmt"
	"log"
	"time"
)

// LLMClient defines the interface for interacting with an LLM.
type LLMClient interface {
	Generate(ctx context.Context, modelName string, prompt string /*, history []Message */) (string, error)
	// Maybe add ListModels() method later
}

type ErrorMsg struct{ Err error }

// Message struct for conversation history
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// --- Dummy Implementation (for testing flow) ---
type DummyClient struct{}

func NewDummyClient() (*DummyClient, error) {
	return &DummyClient{}, nil
}

func (c *DummyClient) Generate(ctx context.Context, modelName string, prompt string /*, history []Message */) (string, error) {
	// Simulate network latency
	log.Printf("sleeping...")
	time.Sleep(800 * time.Millisecond)
	log.Printf("slept")

	// Simulate an error occasionally
	// if time.Now().Unix()%5 == 0 {
	// 	return "", errors.New("dummy LLM failed")
	// }
	log.Printf("returning response...")
	resp := fmt.Sprintf("Dummy response for model '%s'. You asked: '%s'.", modelName, prompt)
	return resp, nil
}
