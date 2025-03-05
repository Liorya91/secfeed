package feed

import (
	"fmt"
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	"github.com/go-shiori/go-readability"
	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
)

var pullInterval = 5 * time.Minute

type Feed struct {
	rssFeeds []config.RSSFeed
	ch       chan types.Article
	initFrom time.Time
	lastPull time.Time
}

func New(cfg *config.Config, initPull time.Duration) (*Feed, error) {
	f := &Feed{
		rssFeeds: cfg.RssFeed,
		ch:       make(chan types.Article),
		initFrom: time.Now().Add(-initPull),
	}

	return f, nil
}

func (f *Feed) Stream() chan types.Article {
	return f.ch
}

func (f *Feed) Start() {
	go f.fetchFeeds()
}

func (f *Feed) collect(from time.Time) ([]types.Article, error) {
	var articles []types.Article

	parser := gofeed.NewParser()

	for _, url := range f.rssFeeds {
		feedFields := log.Fields{"name": url.Name, "url": url.Url}
		log.WithFields(feedFields).Debug("Fetching feed")

		feed, err := parser.ParseURL(url.Url)
		if err != nil {
			log.WithFields(feedFields).Warn("Error fetching feed: %w", err)
			continue
		}

		for _, item := range feed.Items {
			logFields := log.Fields{"title": item.Title, "link": item.Link}

			a := types.Article{
				Title:       item.Title,
				Description: item.Description,
				Link:        item.Link,
				Content:     item.Content,
				Categories:  item.Categories,
			}
			if item.PublishedParsed != nil {
				a.Published = *item.PublishedParsed
			} else {
				// Special case with CrowdStrike feed.
				t, err := time.Parse("Jan 2, 2006 15:04:05-0700", item.Published)
				if err != nil {
					log.WithFields(logFields).Warn("Published date is nil")
					continue
				} else {
					a.Published = t
				}
			}

			// Fetching article content.
			a, err = enrichArticleItem(a)
			if err != nil {
				log.WithFields(logFields).Warnf("Failed to enrich article: %v", err)

				// We can leave without the content for now.
				// the analysis will be less efficient though.
			}

			if a.Published.After(from) {
				articles = append(articles, a)
			}

			break
		}
	}

	return articles, nil
}

func (f *Feed) fetchFeeds() {
	log.Info("Starting to fetch feeds")
	log.Infof("Starting with initial pull from %v", f.initFrom.Format("2006-01-02 15:04:05"))

	articles, err := f.collect(f.initFrom)
	if err != nil {
		log.Errorf("Failed to collect articles: %v", err)
		return
	}
	log.Infof("Collected %d articles", len(articles))

	f.lastPull = time.Now()

	for _, a := range articles {
		f.ch <- a
	}

	log.Infof("Sleeping for %v", pullInterval)
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		articles, err := f.collect(f.lastPull)
		if err != nil {
			log.Errorf("Failed to collect articles: %v", err)
			continue
		}
		log.Infof("Collected %d articles", len(articles))

		f.lastPull = time.Now()

		for _, a := range articles {
			f.ch <- a
		}

		log.Infof("Sleeping for %v", pullInterval)
	}
}

func enrichArticleItem(a types.Article) (types.Article, error) {
	// Sometimes the content is actually a description
	if a.Description == "" && a.Content != "" {
		a.Description = a.Content
	}

	// a description that is too long, isn't a description, and won't help us.
	if len(a.Description) > 1000 {
		a.Description = ""
	}

	// We don't trust the content field, so we fetching the article manually.
	content, err := readability.FromURL(a.Link, 5*time.Second)
	if err != nil {
		return a, fmt.Errorf("failed to extract text content from html: %w", err)
	}

	if len(content.Content) == 0 {
		// This can happned for few feeds, need to fix it.
		return a, fmt.Errorf("failed to extract text content from html (zero content)")
	}

	a.Content = content.TextContent
	fmt.Println("fetched text content len: ", len(content.TextContent))

	return a, nil
}
