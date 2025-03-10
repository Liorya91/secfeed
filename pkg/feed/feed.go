package feed

import (
	"fmt"
	"io"
	"net/http"
	nurl "net/url"
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	"github.com/go-shiori/go-readability"
	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
)

var pullInterval = 5 * time.Minute

// Interface for feed parser to enable mocking
type FeedParser interface {
	ParseURL(url string) (*gofeed.Feed, error)
}

type Feed struct {
	rssFeeds []config.RSSFeed
	ch       chan types.Article

	feedParser FeedParser

	// For each feed, we storing the time from which we should have new articles.
	// This is usually set to time of the last article we pulled.
	//
	// Timestamp checking is not enough, because probably publishers can
	// retroacitvely publish the article, so we missing the articles.
	//
	// Also, we do not want to miss any articles during the time we iterate on the feeds.
	lastItemPerFeed map[string]time.Time
}

func New(cfg *config.Config, initPull time.Duration) (*Feed, error) {
	f := &Feed{
		rssFeeds:        cfg.RssFeed,
		ch:              make(chan types.Article),
		feedParser:      gofeed.NewParser(),
		lastItemPerFeed: make(map[string]time.Time),
	}

	initFrom := time.Now().Add(-initPull)

	for _, feed := range f.rssFeeds {
		f.lastItemPerFeed[feed.Url] = initFrom
	}

	return f, nil
}

func (f *Feed) Stream() chan types.Article {
	return f.ch
}

func (f *Feed) Start() {
	go f.fetchFeeds()
}

func (f *Feed) collect() ([]types.Article, error) {
	var articles []types.Article

	for _, feed := range f.rssFeeds {
		feedArticles, err := f.collectFeed(feed)
		if err != nil {
			return nil, fmt.Errorf("failed to collect feed: %w", err)
		}

		articles = append(articles, feedArticles...)
	}

	return articles, nil
}

func (f *Feed) collectFeed(feed config.RSSFeed) ([]types.Article, error) {
	feedFields := log.Fields{"name": feed.Name, "url": feed.Url}
	log.WithFields(feedFields).Debug("Fetching feed. Previous pull time: ", f.lastItemPerFeed[feed.Url].Format("2006-01-02 15:04:05"))

	parsedFeed, err := f.feedParser.ParseURL(feed.Url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed: %w", err)
	}

	log.WithFields(feedFields).Debugf("Fetched %d items", len(parsedFeed.Items))

	var articles []types.Article
	for _, item := range parsedFeed.Items {
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

		if a.Published.After(f.lastItemPerFeed[feed.Url]) {
			// Fetching article content.
			a, err = enrichArticleItem(a)
			if err != nil {
				log.WithFields(logFields).Warnf("Failed to enrich article: %v", err)

				// We can leave without the content for now.
				// the analysis will be less efficient though.
			}

			articles = append(articles, a)
		}
	}

	if len(articles) > 0 {
		// Updating the last timestamp with the last element.
		// Assuming RSS feed is sorted by date.
		f.lastItemPerFeed[feed.Url] = articles[0].Published

		log.WithFields(feedFields).Debugf("Found %d new articles, updated last pull time to %v", len(articles), f.lastItemPerFeed[feed.Url])
	} else {
		log.WithFields(feedFields).Debug("No new articles")
	}

	return articles, nil
}

func (f *Feed) fetchFeeds() {
	log.Info("Starting to fetch feeds")

	articles, err := f.collect()
	if err != nil {
		log.Errorf("Failed to collect articles: %v", err)
		return
	}
	log.Infof("Collected %d articles", len(articles))

	for _, a := range articles {
		f.ch <- a
	}

	log.Infof("Sleeping for %v", pullInterval)
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		articles, err := f.collect()
		if err != nil {
			log.Errorf("Failed to collect articles: %v", err)
			continue
		}
		log.Infof("Collected %d articles", len(articles))

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
	htmlReader, err := fetchURL(a.Link)
	if err != nil {
		return a, nil
	}
	defer htmlReader.Close()

	// Using readability library to extract the interesting content.
	// Without that it will blow the number of tokens (by magnitude)
	parsedURL, err := nurl.ParseRequestURI(a.Link)
	if err != nil {
		return a, fmt.Errorf("failed to parse URL: %w", err)
	}

	content, err := readability.FromReader(htmlReader, parsedURL)
	if err != nil {
		return a, fmt.Errorf("failed to extract text content from html: %w", err)
	}

	// We have two type of contents,
	// 1. TextContent - only the text and white spaces.
	// 2. Content - the entire content with html tags.
	if len(content.Content) == 0 {
		// This can happned for few feeds, need to fix it.
		return a, fmt.Errorf("failed to extract text content from html (zero content)")
	}

	a.Content = content.TextContent

	return a, nil
}

// Fetches the article content so LLM can be smarter.
// Usually RSS feed don't have the entire content.
// Reader needs to be closed after usage.
func fetchURL(url string) (io.ReadCloser, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the page: %w", err)
	}

	// Set headers to mimic a browser.
	// This is essential for some websites.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) "+
		"Chrome/90.0.4430.93 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the page: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch with status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}
