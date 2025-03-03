package feed

import (
	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
)

type Feed struct {
	rssFeeds []config.RSSFeed
	ch       chan types.Article
}

func New(cfg *config.Config) (*Feed, error) {
	f := &Feed{
		rssFeeds: cfg.RssFeed,
		ch:       make(chan types.Article),
	}

	return f, nil
}

func (f *Feed) Stream() chan types.Article {
	return f.ch
}

func (f *Feed) Start() {
	go f.fetchFeeds()
}

// TODO: get only the updates
// TODO: Mode for get all or only new?

func (f *Feed) fetchFeeds() {
	log.Info("Starting to fetch feeds")

	// var articles []types.Article
	parser := gofeed.NewParser()

	for _, url := range f.rssFeeds {
		log.WithFields(log.Fields{"name": url.Name, "url": url.Url}).Debug("Fetching feed")

		feed, err := parser.ParseURL(url.Url)
		if err != nil {
			log.Printf("Error fetching feed %s: %v", url, err)
			continue
		}

		for _, item := range feed.Items {
			a := types.Article{
				Title:      item.Title,
				Link:       item.Link,
				Content:    item.Content,
				Categories: item.Categories,
			}
			if item.PublishedParsed != nil {
				a.Published = *item.PublishedParsed
			} else {
				log.WithFields(log.Fields{"title": item.Title, "link": item.Link, "categories": item.Categories}).Warn("Published date is nil")
			}

			log.WithFields(log.Fields{"title": item.Title, "link": item.Link, "categories": item.Categories}).Debug("Fetched article")

			f.ch <- a

			// break
		}
	}
}

// // Model holds the TUI state.
// type Model struct {
// 	articles []Article
// 	loading  bool
// 	err      error
// 	selected int
// }

// // Message types for asynchronous commands.
// type articlesMsg struct {
// 	articles []Article
// 	err      error
// }

// type summaryMsg struct {
// 	index   int
// 	summary string
// 	err     error
// }
