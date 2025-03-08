package types

import (
	"fmt"
	"strings"
	"time"
)

type Article struct {
	Title       string
	Description string
	Link        string
	Content     string
	Categories  []string
	Published   time.Time

	// Enriched later through LLM
	Summary      string
	CatRelevance []CategoryRelevance
}

func (a Article) String() string {
	contentSnippet := a.Content[:min(100, len(a.Content))]
	return fmt.Sprintf("Article: %s\nDescription: %s\nContent (snippet and size): %s... (%d)\nLink: %s\nPublished: %s\nCategories: %v\n\n", a.Title, a.Description, contentSnippet, len(a.Content), a.Link, a.Published, a.Categories)
}

func (a Article) FormatAsMarkdown(debugInfo bool) string {
	t := fmt.Sprintf("# %s\n\n%s\n\nRead more [here](%s)", a.Title, a.Summary, a.Link)

	if debugInfo {
		t = t + "\n\n---\n\n"
		t = t + "**Debug info:**\n\n"
		t = t + fmt.Sprintf("**len(Description):** %d\n", len(a.Description))
		t = t + fmt.Sprintf("**len(Content):** %d\n\n", len(a.Content))
		for _, cr := range a.CatRelevance {
			t = t + fmt.Sprintf("**Category:** %s\n\n**Relevance:** %.1f\n\n**Explanation**: %s\n\n", cr.Category, cr.Relevance, cr.Explanation)
		}
		t = t + "\n\n---\n\n"
	}

	return t
}

func (a Article) FormatAsSlackMrkdwn(debugInfo bool) string {
	a.Summary = strings.ReplaceAll(a.Summary, "- ", "• ")
	a.Summary = strings.ReplaceAll(a.Summary, "**", "*")

	t := fmt.Sprintf("*%s*\n\n%s\n\nRead more <%s|here>", a.Title, a.Summary, a.Link)

	if debugInfo {
		t = t + "\n---\n"
		t = t + "*Debug info:*\n"
		t = t + fmt.Sprintf("*len(Description):* %d\n", len(a.Description))
		t = t + fmt.Sprintf("*len(Content):* %d\n", len(a.Content))
		for _, cr := range a.CatRelevance {
			t = t + fmt.Sprintf("*Category:* %s\n*Relevance:* %.1f\n*Explanation*: %s\n\n", cr.Category, cr.Relevance, cr.Explanation)
		}
		t = t + "\n---\n"
	}

	return t
}

// CategoryRelevance represents the relevance of a category to an article.
// Relevance is a number between 0 (not relevant) to 10 (very relevant).
type CategoryRelevance struct {
	Category    string  `json:"category"`
	Relevance   float32 `json:"relevance"`
	Explanation string  `json:"explanation,omitempty"`
}
