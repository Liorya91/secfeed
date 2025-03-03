package types

import (
	"fmt"
	"time"
)

type Article struct {
	Title      string
	Link       string
	Content    string
	Categories []string
	Published  time.Time
}

func (a Article) String() string {
	// return fmt.Sprintf("Article: %s\nLink: %s\nPublished: %s\nCategories: %v\n\n", a.Title, a.Link, a.Published, a.Categories)
	return fmt.Sprintf("Article: %s\nLink: %s\nPublished: %s\nCategories: %v\n\n", a.Title, a.Link, a.Published, a.Categories)
}
