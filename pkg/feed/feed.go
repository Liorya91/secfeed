package feed

import (
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/types"
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
				log.WithFields(logFields).Warn("Published date is nil")
				continue
			}

			// Doing some alignment between different feeds.
			a = cleanArticleItem(a)

			if a.Published.After(from) {
				articles = append(articles, a)
			}
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

func cleanArticleItem(a types.Article) types.Article {
	// Content is actually a description
	if a.Content != "" && a.Content == a.Description {
		a.Content = ""
	}

	// Content is actually a description no. 2
	if a.Description == "" && len(a.Content) < 200 {
		a.Description = a.Content
		a.Content = ""
	}

	// This isn't really content. Better without it, and fetch the URL manually.
	if len(a.Content) < 300 {
		a.Content = ""
	}

	return a
}
