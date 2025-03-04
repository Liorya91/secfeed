package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/constants"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
)

const (
	embeddingsMaxTextLength = 8000
	completionMaxTextLength = 2000

	classificationMaxTokens = 2000
	summaryMaxTokens        = 500

	classificationModel = openai.GPT4oMini
	summaryModel        = openai.GPT4o
	embeddingModel      = openai.SmallEmbedding3

	embeddingsChunkSize = 1000
	embeddingsOverlap   = 200
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

func (c *Client) ExtractCategories(ctx context.Context, article types.Article) ([]string, error) {
	// Prepare the input text from the article
	input := fmt.Sprintf("Title: %s\nContent: %s\nCategories: %v", article.Title, article.Content, article.Categories)

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: classificationModel,
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

	c.tokenUsed[classificationModel].add(tokenUsed{
		prompt:     resp.Usage.PromptTokens,
		completion: resp.Usage.CompletionTokens,
	})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateChatCompletion call")

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

	if len(input) > completionMaxTextLength {
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

	c.tokenUsed[summaryModel].add(tokenUsed{
		prompt:     resp.Usage.PromptTokens,
		completion: resp.Usage.CompletionTokens,
	})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateChatCompletion call")

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) CategoryMatching(ctx context.Context, categoriesToMatch []config.Category, article types.Article) ([]types.CategoryRelevance, error) {
	systemPrompt := `You have a list of categories to evaluate. 
For each category, determine how relevant the user's article is to that category. 

Scoring:
- A relevance score on a scale of 0 to 10, where 0 means “no connection” and 10 means “highly relevant.”
- Provide a short explanation for the assigned score.

Output must be valid JSON without markdown formatting. Return an array of objects, where each object has:
{
	"category": "<category name>",
	"relevance": <integer from 0 to 10>,
	"explanation": "<brief explanation>"
}

Categories:
`
	for i, cat := range categoriesToMatch {
		systemPrompt += fmt.Sprintf("%d. %s: %s\n", i+1, cat.Name, cat.Description)
	}

	articleInput := fmt.Sprintf("Title: %s\nDescription: %s\nLink: %s\nContent: %s\n", article.Title, article.Description, article.Link, article.Content)
	userPrompt := `Below is the article to evaluate:

` + articleInput

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: classificationModel,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: userPrompt,
				},
			},
			Temperature:         0, // Low temperature for more deterministic results
			MaxCompletionTokens: classificationMaxTokens,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}

	c.tokenUsed[classificationModel].add(tokenUsed{
		prompt:     resp.Usage.PromptTokens,
		completion: resp.Usage.CompletionTokens,
	})
	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateChatCompletion call")

	var relevance []types.CategoryRelevance
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &relevance)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal relevance scores: %w", err)
	}

	return relevance, nil
}

// callEmbeddingAPI is a helper that sends texts to the OpenAI API without checking length.
func (c *Client) callEmbeddingAPI(ctx context.Context, texts []string) ([][]float32, error) {
	req := openai.EmbeddingRequest{
		Model: embeddingModel,
		Input: texts,
	}

	resp, err := c.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, err
	}

	c.tokenUsed[string(embeddingModel)].add(tokenUsed{
		prompt: resp.Usage.PromptTokens,
	})
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

// Embedding computes embeddings for each text in the input slice.
// If a text exceeds embeddingsMaxTextLength, it will be split into smaller chunks,
// embeddings for each chunk will be computed, and then averaged.
func (c *Client) Embedding(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		// If the text is within the maximum allowed length, process directly.
		if len(text) <= embeddingsMaxTextLength {
			embs, err := c.callEmbeddingAPI(ctx, []string{text})
			if err != nil {
				return nil, err
			}
			results[i] = embs[0]
		} else {
			// For texts that are too long, split into chunks.
			chunks := chunkText(text, embeddingsChunkSize, embeddingsOverlap)
			chunkEmbeddings, err := c.callEmbeddingAPI(ctx, chunks)
			if err != nil {
				return nil, err
			}

			// Average the embeddings of the chunks.
			avgEmbedding, err := averageEmbeddings(chunkEmbeddings)
			if err != nil {
				return nil, err
			}
			results[i] = avgEmbedding
		}
	}

	return results, nil
}

// func (c *Client) Embedding(ctx context.Context, texts []string) ([][]float32, error) {
// 	for _, text := range texts {
// 		if len(text) > embeddingsMaxTextLength {
// 			return nil, fmt.Errorf("text is too long (%d)", len(text))
// 		}
// 	}

// 	req := openai.EmbeddingRequest{
// 		Model: embeddingModel,
// 		Input: texts,
// 	}

// 	resp, err := c.client.CreateEmbeddings(ctx, req)
// 	if err != nil {
// 		return nil, err
// 	}

// 	c.tokenUsed[string(embeddingModel)].add(tokenUsed{
// 		prompt: resp.Usage.PromptTokens,
// 	})
// 	log.WithFields(log.Fields{"model": resp.Model, "tokens": resp.Usage.TotalTokens, "total_cost": c.totalCost()}).Debug("OpenAI API CreateEmbeddings call")

// 	if len(resp.Data) != len(texts) {
// 		return nil, fmt.Errorf("number of embeddings returned does not match number of texts")
// 	}

// 	embeddings := make([][]float32, len(texts))
// 	for i, embedding := range resp.Data {
// 		embeddings[i] = embedding.Embedding
// 	}

// 	return embeddings, nil
// }

func (c *Client) totalCost() float32 {
	var total float32
	for model, cost := range c.tokenUsed {
		total += float32(cost.prompt) / 1000000 * modelInputCosts[model].input
		total += float32(cost.completion) / 1000000 * modelInputCosts[model].output
	}

	return total
}

// chunkText splits a text into chunks of at most chunkSize characters with a given overlap.
func chunkText(text string, chunkSize, overlap int) []string {
	var chunks []string
	runes := []rune(text)
	n := len(runes)
	start := 0
	for start < n {
		end := start + chunkSize
		if end > n {
			end = n
		}
		chunks = append(chunks, string(runes[start:end]))
		// Move forward by chunkSize-overlap to allow overlapping.
		start += (chunkSize - overlap)
	}
	return chunks
}

// averageEmbeddings calculates the element-wise average of the provided embeddings.
// All embeddings must have the same dimension.
func averageEmbeddings(embeddings [][]float32) ([]float32, error) {
	if len(embeddings) == 0 {
		return nil, errors.New("no embeddings provided")
	}

	dim := len(embeddings[0])
	avg := make([]float32, dim)
	count := float32(len(embeddings))

	for _, emb := range embeddings {
		if len(emb) != dim {
			return nil, errors.New("embeddings have inconsistent dimensions")
		}
		for i, value := range emb {
			avg[i] += value
		}
	}

	for i := range avg {
		avg[i] /= count
	}

	return avg, nil
}

// weightedAverageEmbeddings calculates the element-wise weighted average of embeddings.
// The weights slice should have the same length as embeddings and its values don't have to sum to 1.
func weightedAverageEmbeddings(embeddings [][]float32, weights []float32) ([]float32, error) {
	if len(embeddings) == 0 {
		return nil, errors.New("no embeddings provided")
	}
	if len(embeddings) != len(weights) {
		return nil, errors.New("number of weights must match number of embeddings")
	}

	dim := len(embeddings[0])
	weightedAvg := make([]float32, dim)
	var totalWeight float32

	// Normalize weights and aggregate
	for idx, emb := range embeddings {
		if len(emb) != dim {
			return nil, errors.New("embeddings have inconsistent dimensions")
		}
		totalWeight += weights[idx]
		for i, value := range emb {
			weightedAvg[i] += value * weights[idx]
		}
	}

	// Normalize by the total weight
	if totalWeight == 0 {
		return nil, errors.New("total weight is zero")
	}

	for i := range weightedAvg {
		weightedAvg[i] /= totalWeight
	}

	return weightedAvg, nil
}
