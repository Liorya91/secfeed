package similarity

import (
	"context"
	"fmt"
	"math"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/llm"
	"github.com/alex-ilgayev/secfeed/pkg/types"
)

type Similarity struct {
	client *llm.Client

	categories []config.Category
	threshold  int
	// inputEmbeddings map[string][]float32
}

// Creating a new similarity engine, using the OpenAI API.
// We pre-encode the categories to save time on repeated requests.
func New(ctx context.Context, client *llm.Client, categories []config.Category, threshold int) (*Similarity, error) {
	s := &Similarity{
		client:     client,
		categories: categories,
		threshold:  threshold,
		// inputEmbeddings: make(map[string][]float32, len(categories)),
	}

	// Pre-encode the input categories.
	// For the embedding algorithm, that we won't implement.
	//
	// log.WithFields(log.Fields{"categories": categories}).Info("Pre-encoding input categories")
	// embVecs, err := s.client.Embedding(ctx, categories)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get embedding for categories: %w", err)
	// }

	// if len(embVecs) != len(inputCategories) {
	// 	return nil, fmt.Errorf("number of embeddings returned does not match number of categories")
	// }

	// for i, cat := range inputCategories {
	// 	s.inputEmbeddings[cat] = embVecs[i]
	// }

	return s, nil
}

func (s *Similarity) CheckSimilarity(ctx context.Context, article types.Article) ([]types.CategoryRelevance, error) {
	catMatching, err := s.client.CategoryMatching(ctx, s.categories, article)
	if err != nil {
		return nil, fmt.Errorf("failed to get category matches: %w", err)
	}

	matchedCategories := make([]types.CategoryRelevance, 0)
	for _, match := range catMatching {
		if match.Relevance >= s.threshold {
			matchedCategories = append(matchedCategories, match)
		}
	}

	return matchedCategories, nil
}

// Doing embeddings for the entire article.
// There is problem with this, because we do not always have the entire article.
// func (s *Similarity) CheckSimilarity(ctx context.Context, articleContent string) (map[string]float32, error) {
// 	if articleContent == "" {
// 		return nil, fmt.Errorf("article content is empty")
// 	}

// 	articleEmbVecs, err := s.client.Embedding(ctx, []string{articleContent})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get embedding for article categories: %w", err)
// 	}
// 	vec := articleEmbVecs[0]

// 	// Compute the similarity to each category, track the best match per category.
// 	bestMatchPerCat := make(map[string]float32, len(s.inputEmbeddings))
// 	for cat := range s.inputEmbeddings {
// 		bestMatchPerCat[cat] = -1.0
// 	}

// 	for name, catVec := range s.inputEmbeddings {
// 		sim := cosineSimilarity(vec, catVec)
// 		if sim > bestMatchPerCat[name] {
// 			bestMatchPerCat[name] = sim
// 		}
// 	}

// 	return bestMatchPerCat, nil
// }

// cosineSimilarity calculates the cosine similarity between two vectors.
// Used for the embeddings. Not relevant at the moment.
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
