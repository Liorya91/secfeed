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
	Summary string
}

func (a Article) String() string {
	contentSnippet := a.Content[:min(100, len(a.Content))]
	return fmt.Sprintf("Article: %s\nDescription: %s\nContent (snippet and size): %s... (%d)\nLink: %s\nPublished: %s\nCategories: %v\n\n", a.Title, a.Description, contentSnippet, len(a.Content), a.Link, a.Published, a.Categories)
}

func (a Article) FormatAsMarkdown() string {
	// Summary is already in markdown format
	return fmt.Sprintf("# %s\n\n%s\n\nRead more [here](%s)", a.Title, a.Summary, a.Link)
}

func (a Article) FormatAsSlackMrkdwn() string {
	a.Summary = strings.ReplaceAll(a.Summary, "- ", "• ")
	a.Summary = strings.ReplaceAll(a.Summary, "**", "*")
	return fmt.Sprintf("*%s*\n\n%s\n\nRead more <%s|here>", a.Title, a.Summary, a.Link)
}

// CategoryRelevance represents the relevance of a category to an article.
// Relevance is a number between 0 (not relevant) to 10 (very relevant).
type CategoryRelevance struct {
	Category    string `json:"category"`
	Relevance   int    `json:"relevance"`
	Explanation string `json:"explanation"`
}
