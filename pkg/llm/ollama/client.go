package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/alex-ilgayev/secfeed/pkg/constants"
	log "github.com/sirupsen/logrus"
)

// API Documentation:
// https://github.com/ollama/ollama/blob/main/docs/api.md#generate-a-chat-completion

const (
	completionAPI = "/api/chat"
)

type Client struct {
	client  *http.Client
	baseUrl string
}

// NewClient creates a new ollama client, and loads specified models.
func NewClient(ctx context.Context, models []string) (*Client, error) {
	baseUrl := os.Getenv(constants.EnvOllamaBaseUrl)
	if baseUrl == "" {
		baseUrl = "http://localhost:11434"
	}

	client := &Client{
		client:  http.DefaultClient,
		baseUrl: baseUrl,
	}

	for _, model := range models {
		if err := client.loadModel(ctx, model); err != nil {
			return nil, fmt.Errorf("failed to load model %s: %w", model, err)
		}
	}

	return client, nil
}

type ChatCompletionRequest struct {
	Model     string                  `json:"model"`
	Messages  []ChatCompletionMessage `json:"messages,omitempty"`
	Option    ChatCompletionOptions   `json:"options,omitempty"`
	Stream    bool                    `json:"stream,omitempty"`
	Format    string                  `json:"format,omitempty"`
	KeepAlive int                     `json:"keep_alive,omitempty"`
}

type ChatCompletionMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type ChatCompletionOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
}

var (
	ChatMessageRoleSystem    = "system"
	ChatMessageRoleUser      = "user"
	ChatMessageRoleAssistant = "assistant"
)

type ChatCompletionResponse struct {
	Model      string                `json:"model"`
	Message    ChatCompletionMessage `json:"message,omitempty"`
	Done       bool                  `json:"done,omitempty"`
	DoneReason string                `json:"done_reason,omitempty"`
}

func (c *Client) loadModel(ctx context.Context, model string) error {
	// Loading model is equivalent to sending chat completion message
	// without a content.

	chatReq := ChatCompletionRequest{
		Model:    model,
		Messages: nil,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return fmt.Errorf("failed to marshal ChatCompletionRequest: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s%s", c.baseUrl, completionAPI),
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call ollama completion API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("request failed with status code %d", resp.StatusCode)
	}

	// Parse the response
	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return fmt.Errorf("failed to parse ChatCompletionResponse")
	}

	if chatResp.DoneReason != "load" {
		return fmt.Errorf("model load failed with different done reason: %s", chatResp.DoneReason)
	}

	log.WithFields(log.Fields{"model": chatResp.Model}).Debug("Ollama API ChatCompletion (load model) call")

	return nil
}

func (c *Client) ChatCompletion(ctx context.Context, model string, systemMsg, userMsg string, temperature float32, maxTokens int, jsonFormat bool) (string, error) {
	messages := []ChatCompletionMessage{}
	if systemMsg != "" {
		messages = append(messages, ChatCompletionMessage{
			Role:    ChatMessageRoleSystem,
			Content: systemMsg,
		})
	}
	if userMsg != "" {
		messages = append(messages, ChatCompletionMessage{
			Role:    ChatMessageRoleUser,
			Content: userMsg,
		})
	}

	// Call Ollama API
	chatReq := ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Option: ChatCompletionOptions{
			Temperature: temperature,
		},
	}

	if jsonFormat {
		chatReq.Format = "json"
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ChatCompletionRequest: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s%s", c.baseUrl, completionAPI),
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call ollama completion API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("request failed with status code %d", resp.StatusCode)
	}

	// Parse the response
	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to parse ChatCompletionResponse")
	}

	log.WithFields(log.Fields{"model": chatResp.Model}).Debug("Ollama API ChatCompletion call")

	return chatResp.Message.Content, nil
}
