package openai

import (
	"context"
	"fmt"
	"os"

	"github.com/alex-ilgayev/secfeed/pkg/constants"
	openai "github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
)

var (
	// An attempt to calculate the cost of the API calls
	// The costs is per 1M tokens.
	// There is a different cost per input tokens, cached input, and output tokens.
	// Currently, we ignoring cached input.
	modelInputCosts = map[string]modelCost{
		openai.GPT4oMini: {
			input:       0.15,
			cachedInput: 0.075,
			output:      0.6,
		},
		openai.GPT4o: {
			input:       2.5,
			cachedInput: 1.25,
			output:      10,
		},
		string(openai.AdaEmbeddingV2): {
			input:       0.10,
			cachedInput: 0,
			output:      0,
		},
		string(openai.SmallEmbedding3): {
			input:       0.02,
			cachedInput: 0,
			output:      0,
		},
	}
)

type modelCost struct {
	input       float32
	cachedInput float32
	output      float32
}

type tokenUsed struct {
	prompt     int
	completion int
}

func (t *tokenUsed) add(other tokenUsed) {
	t.prompt += other.prompt
	t.completion += other.completion
}

// Client is a generic interface that wraps OpenAI at the moment.
type Client struct {
	client *openai.Client

	// Used to calculate approximate costs
	tokenUsed map[string]*tokenUsed
}

func NewClient() (*Client, error) {
	apiKey := os.Getenv(constants.EnvOpenAiApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s environment variable is not set", constants.EnvOpenAiApiKey)
	}

	client := openai.NewClient(apiKey)

	return &Client{
		client: client,
		tokenUsed: map[string]*tokenUsed{
			openai.GPT4oMini:               {},
			openai.GPT4o:                   {},
			string(openai.AdaEmbeddingV2):  {},
			string(openai.SmallEmbedding3): {},
		},
	}, nil
}

func (c *Client) ChatCompletion(ctx context.Context, model string, systemMsg, userMsg string, temperature float32, maxTokens int, jsonFormat bool) (string, error) {
	messages := []openai.ChatCompletionMessage{}
	if systemMsg != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMsg,
		})
	}
	if userMsg != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: userMsg,
		})
	}

	// TODO: implemen json formatting.

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:               model,
			Messages:            messages,
			Temperature:         temperature,
			MaxCompletionTokens: maxTokens,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	c.tokenUsed[string(model)].add(tokenUsed{
		prompt:     resp.Usage.PromptTokens,
		completion: resp.Usage.CompletionTokens,
	})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateChatCompletion call")

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) totalCost() float32 {
	var total float32
	for model, cost := range c.tokenUsed {
		total += float32(cost.prompt) / 1000000 * modelInputCosts[model].input
		total += float32(cost.completion) / 1000000 * modelInputCosts[model].output
	}

	return total
}
