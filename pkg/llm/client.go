package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alex-ilgayev/secfeed/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
)

const (
	maxTextLength = 4000

	classificationMaxTokens = 200
	summaryMaxTokens        = 500

	classificationModel = openai.GPT4oMini
	summaryModel        = openai.GPT4o
	embeddingModel      = openai.AdaEmbeddingV2
)

// Client is a generic interface that wraps OpenAI at the moment.
type Client struct {
	client *openai.Client
}

func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	client := openai.NewClient(apiKey)

	return &Client{
		client: client,
	}, nil
}

func (c *Client) ExtractCategories(ctx context.Context, article types.Article) ([]string, error) {
	// Prepare the input text from the article
	input := fmt.Sprintf("Title: %s\nContent: %s\nCategories: %v", article.Title, article.Content, article.Categories)

	if len(input) > maxTextLength {
		return nil, fmt.Errorf("input text for category extraction is too long")
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "system",
					Content: "Extract key categories and topics from this article. Return a JSON array of strings without markdown format with no explanation.",
				},
				{
					Role:    "user",
					Content: input,
				},
			},
			Temperature:         0.2, // Low temperature for more deterministic results
			MaxCompletionTokens: classificationMaxTokens,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens}).Debug("OpenAI API CreateChatCompletion call")

	var categories []string
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &categories)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal categories: %w", err)
	}

	log.WithFields(log.Fields{"categories": categories}).Debug("Extracted categories")

	return categories, nil
}

func (c *Client) Summarize(ctx context.Context, article types.Article) (string, error) {
	// Prepare the input text from the article
	input := fmt.Sprintf("Title: %s\nContent: %s\nCategories: %v", article.Title, article.Content, article.Categories)

	if len(input) > 4000 {
		return "", fmt.Errorf("input text for summary is too long")
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: summaryModel,
			Messages: []openai.ChatCompletionMessage{
				{
					Role: "system",
					Content: `You are an AI assistant specialized in summarizing articles. Your task is to generate concise, accurate, and clear summaries of the content. When given a article, follow these guidelines:

1. Accuracy and Fidelity: Extract and convey the key points, methodologies, results, and conclusions as presented in the original text without introducing new interpretations.
2. Clarity and Brevity: Create summaries that are succinct and understandable even for complex topics. Use plain language and avoid unnecessary jargon.
3. Structure: Organize the summary logically. Consider using bullet points or short paragraphs to highlight:
   - The main objective or problem addressed.
   - The methodology or approach taken.
   - Key findings and results.
   - Conclusions or implications.
4. Neutrality: Maintain an objective tone. Do not include personal opinions or commentary.
5. Adaptability: Adjust the level of detail based on the article’s complexity and length. For highly technical or detailed articles, ensure the summary captures essential data without oversimplification.
6. Uncertainty: If certain parts of the article are ambiguous or contain conflicting information, note these uncertainties clearly in the summary.

Your goal is to help readers quickly grasp the essence of technical articles while preserving the integrity of the original content.`,
				},
				{
					Role:    "user",
					Content: input,
				},
			},
			Temperature:         0.5,
			MaxCompletionTokens: summaryMaxTokens,
			ResponseFormat:      &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeText},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens}).Debug("OpenAI API CreateChatCompletion call")

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) Embedding(ctx context.Context, texts []string) ([][]float32, error) {
	for _, text := range texts {
		if len(text) > maxTextLength {
			return nil, fmt.Errorf("text is too long: %d characters", len(text))
		}
	}

	req := openai.EmbeddingRequest{
		Model: embeddingModel,
		Input: texts,
	}

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens}).Debug("OpenAI API CreateEmbeddings call")

	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("number of embeddings returned does not match number of texts")
	}

	embeddings := make([][]float32, len(texts))
	for i, embedding := range resp.Data {
		embeddings[i] = embedding.Embedding
	}

	return embeddings, nil
}
