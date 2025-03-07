package classification

import (
	"context"
	"fmt"
	"math"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/llm"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	log "github.com/sirupsen/logrus"
)

type ClassificationEngineType string

// Implementing the pflag.Value interface for ClassificationEngine
func (l *ClassificationEngineType) String() string {
	return string(*l)
}

// Implementing the pflag.Value interface for ClassificationEngine
func (l *ClassificationEngineType) Set(value string) error {
	*l = ClassificationEngineType(value)
	return nil
}

// Implementing the pflag.Value interface for ClassificationEngine
func (l *ClassificationEngineType) Type() string {
	return "classificationEngineType"
}

const (
	LLM        ClassificationEngineType = "llm"
	Embeddings ClassificationEngineType = "embeddings"
)

type ClassificationEngine struct {
	clsType ClassificationEngineType
	client  *llm.Client

	categories []config.Category
	threshold  float32

	// Encoded categories for similarity comparison
	// Only used when clsType == Embeddings
	encCategories map[string][]float32
}

func New(ctx context.Context, classificationType ClassificationEngineType, client *llm.Client, categories []config.Category, threshold float32) (*ClassificationEngine, error) {
	c := &ClassificationEngine{
		clsType:       classificationType,
		client:        client,
		categories:    categories,
		threshold:     threshold,
		encCategories: make(map[string][]float32, len(categories)),
	}

	if classificationType == Embeddings {
		var err error
		// If choose classification using embeddings, we need to pre-encode the input categories.
		log.WithFields(log.Fields{"categories": categories}).Info("Pre-encoding input categories")
		c.encCategories, err = c.client.EncodeCategories(ctx, categories)
		if err != nil {
			return nil, fmt.Errorf("failed to get embeddings for categories: %w", err)
		}
	}

	return c, nil
}

func (c *ClassificationEngine) Classify(ctx context.Context, article types.Article) ([]types.CategoryRelevance, error) {
	if c.clsType == LLM {
		return c.classifyWithLLM(ctx, article)
	} else if c.clsType == Embeddings {
		return c.classifyWithEmbeddings(ctx, article)
	} else {
		return nil, fmt.Errorf("unknown classification engine type: %s", c.clsType)
	}
}

func (c *ClassificationEngine) classifyWithLLM(ctx context.Context, article types.Article) ([]types.CategoryRelevance, error) {
	catMatching, err := c.client.CategoryMatchingWithLLM(ctx, c.categories, article)
	if err != nil {
		return nil, fmt.Errorf("failed to get category matches: %w", err)
	}

	for _, match := range catMatching {
		log.WithFields(log.Fields{
			"category":    match.Category,
			"relevance":   match.Relevance,
			"explanation": match.Explanation,
		}).Debug("Category match")
	}

	matchedCategories := make([]types.CategoryRelevance, 0)
	for _, match := range catMatching {
		if match.Relevance >= c.threshold {
			matchedCategories = append(matchedCategories, match)
		}
	}

	return matchedCategories, nil
}

func (c *ClassificationEngine) classifyWithEmbeddings(ctx context.Context, article types.Article) ([]types.CategoryRelevance, error) {
	articleEmbVecs, err := c.client.EncodeArticle(ctx, article)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding for article categories: %w", err)
	}

	// Compute the similarity to each category, and normalize it from 0 to 10.
	matchedCategories := make([]types.CategoryRelevance, 0)
	for name, catVec := range c.encCategories {
		sim := cosineSimilarity(articleEmbVecs, catVec)
		relevance := sim * 10

		log.WithFields(log.Fields{
			"category":  name,
			"sim":       sim,
			"relevance": relevance,
		}).Debug("Category match")

		if relevance >= c.threshold {
			matchedCategories = append(matchedCategories, types.CategoryRelevance{
				Category:    name,
				Relevance:   relevance,
				Explanation: "",
			})
		}
	}

	return matchedCategories, nil
}

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
