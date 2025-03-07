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
		openai.O1: {
			input:       15,
			cachedInput: 7.5,
			output:      60,
		},
		openai.O3Mini: {
			input:       1.1,
			cachedInput: 0.55,
			output:      4.4,
		},
		openai.O1Mini: {
			input:       1.1,
			cachedInput: 0.55,
			output:      4.4,
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
		string(openai.LargeEmbedding3): {
			input:       0.13,
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
		client:    client,
		tokenUsed: make(map[string]*tokenUsed),
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

	// TODO: implement json formatting.

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

	c.updateTokenUsed(model, tokenUsed{prompt: resp.Usage.PromptTokens, completion: resp.Usage.CompletionTokens})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateChatCompletion call")

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) CreateEmbeddings(ctx context.Context, model string, texts []string) ([][]float32, error) {
	req := openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: texts,
	}

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, err
	}

	c.updateTokenUsed(model, tokenUsed{prompt: resp.Usage.PromptTokens, completion: resp.Usage.CompletionTokens})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateEmbeddings call")

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("number of embeddings returned does not match number of texts")
	}

	embeddings := make([][]float32, len(texts))
	for i, embedding := range resp.Data {
		embeddings[i] = embedding.Embedding
	}

	return embeddings, nil
}

func (c *Client) totalCost() float32 {
	var total float32
	for model, cost := range c.tokenUsed {
		total += float32(cost.prompt) / 1000000 * modelInputCosts[model].input
		total += float32(cost.completion) / 1000000 * modelInputCosts[model].output
	}

	return total
}

func (c *Client) updateTokenUsed(model string, used tokenUsed) {
	if _, ok := c.tokenUsed[model]; !ok {
		c.tokenUsed[model] = &tokenUsed{}
	}

	c.tokenUsed[model].add(tokenUsed{
		prompt:     used.prompt,
		completion: used.completion,
	})
}
