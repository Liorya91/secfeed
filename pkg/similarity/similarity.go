package similarity

import (
	"context"
	"fmt"
	"math"

	"github.com/alex-ilgayev/secfeed/pkg/llm"
)

type Similarity struct {
	client *llm.Client

	inputEmbeddings map[string][]float32
}

// Creating a new similarity engine, using the OpenAI API.
// We pre-encode the categories to save time on repeated requests.
func New(ctx context.Context, client *llm.Client, inputCategories []string) (*Similarity, error) {
	s := &Similarity{
		client:          client,
		inputEmbeddings: make(map[string][]float32, len(inputCategories)),
	}

	// Pre-encode the input categories.
	embVecs, err := s.client.Embedding(ctx, inputCategories)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding for categories: %w", err)
	}

	if len(embVecs) != len(inputCategories) {
		return nil, fmt.Errorf("number of embeddings returned does not match number of categories")
	}

	for i, cat := range inputCategories {
		s.inputEmbeddings[cat] = embVecs[i]
	}

	return s, nil
}

func (s *Similarity) CheckSimilarity(ctx context.Context, articleCategories []string) (map[string]float32, error) {
	articleEmbVecs, err := s.client.Embedding(ctx, articleCategories)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding for article categories: %w", err)
	}

	// Compute the similarity to each category, track the best match per category.
	bestMatchPerCat := make(map[string]float32, len(s.inputEmbeddings))
	for cat := range s.inputEmbeddings {
		bestMatchPerCat[cat] = -1.0
	}

	for name, catVec := range s.inputEmbeddings {
		for _, articleEmbVec := range articleEmbVecs {
			sim := cosineSimilarity(articleEmbVec, catVec)
			if sim > bestMatchPerCat[name] {
				bestMatchPerCat[name] = sim
			}
		}
	}

	return bestMatchPerCat, nil
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		// Should not happen if embeddings use the same model
		return -1
	}
	var dot, normA, normB float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
