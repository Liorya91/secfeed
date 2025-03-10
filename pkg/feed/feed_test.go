package feed

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock for the gofeed.Parser
type mockParser struct {
	mock.Mock
}

func (m *mockParser) ParseURL(url string) (*gofeed.Feed, error) {
	args := m.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gofeed.Feed), args.Error(1)
}

// Helper to create test published time
func createTime(daysAgo int) time.Time {
	return time.Now().Add(time.Duration(-daysAgo) * 24 * time.Hour)
}

func createPointer(t time.Time) *time.Time {
	return &t
}

func TestNew(t *testing.T) {
	testTime := time.Now()

	tests := []struct {
		name         string
		config       *config.Config
		initPull     time.Duration
		expectedURLs []string
	}{
		{
			name: "create feed with multiple RSS feeds",
			config: &config.Config{
				RssFeed: []config.RSSFeed{
					{Name: "Feed1", Url: "https://feed1.com/rss"},
					{Name: "Feed2", Url: "https://feed2.com/rss"},
					{Name: "Feed3", Url: "https://feed3.com/rss"},
				},
			},
			initPull:     24 * time.Hour,
			expectedURLs: []string{"https://feed1.com/rss", "https://feed2.com/rss", "https://feed3.com/rss"},
		},
		{
			name: "create feed with no RSS feeds",
			config: &config.Config{
				RssFeed: []config.RSSFeed{},
			},
			initPull:     12 * time.Hour,
			expectedURLs: []string{},
		},
		{
			name: "create feed with single RSS feed",
			config: &config.Config{
				RssFeed: []config.RSSFeed{
					{Name: "Feed1", Url: "https://feed1.com/rss"},
				},
			},
			initPull:     1 * time.Hour,
			expectedURLs: []string{"https://feed1.com/rss"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function under test
			feed, err := New(tt.config, tt.initPull)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, feed)

			// Check that the channel is initialized
			require.NotNil(t, feed.ch)

			// Check that the feedParser is initialized
			require.NotNil(t, feed.feedParser)

			// Check that all feed URLs are properly stored
			assert.Equal(t, len(tt.expectedURLs), len(feed.rssFeeds))
			for i, url := range tt.expectedURLs {
				assert.Equal(t, url, feed.rssFeeds[i].Url)
			}

			// Check that lastItemPerFeed contains entries for all RSS feeds
			assert.Equal(t, len(tt.expectedURLs), len(feed.lastItemPerFeed))

			// Check that the timestamps are approximately correct (within 1 second of expected)
			expectedTime := testTime.Add(-tt.initPull)
			for _, url := range tt.expectedURLs {
				lastPullTime, exists := feed.lastItemPerFeed[url]
				assert.True(t, exists)

				// Check that the time difference is small (should be almost the same as expectedTime)
				timeDiff := lastPullTime.Sub(expectedTime)
				require.LessOrEqual(t, timeDiff.Abs(), time.Second)
			}
		})
	}
}

func TestCollectFeed(t *testing.T) {
	tests := []struct {
		name             string
		feed             config.RSSFeed
		lastPullTime     time.Time
		parsedFeed       *gofeed.Feed
		parseError       error
		expectedArticles int
		expectedLastPull time.Time
		shouldUpdateTime bool
	}{
		{
			name: "successful parsing with new articles",
			feed: config.RSSFeed{
				Name: "Test Feed",
				Url:  "https://test.com/feed",
			},
			lastPullTime: createTime(5),
			parsedFeed: &gofeed.Feed{
				Items: []*gofeed.Item{
					{
						Title:           "New Article 1",
						Description:     "Description 1",
						Link:            "https://test.com/article1",
						Content:         "Content 1",
						Categories:      []string{"security", "test"},
						PublishedParsed: createPointer(createTime(1)),
					},
					{
						Title:           "New Article 2",
						Description:     "Description 2",
						Link:            "https://test.com/article2",
						Content:         "Content 2",
						Categories:      []string{"security", "test"},
						PublishedParsed: createPointer(createTime(2)),
					},
				},
			},
			parseError:       nil,
			expectedArticles: 2,
			expectedLastPull: createTime(1),
			shouldUpdateTime: true,
		},
		{
			name: "no new articles",
			feed: config.RSSFeed{
				Name: "Test Feed",
				Url:  "https://test.com/feed",
			},
			lastPullTime: createTime(1),
			parsedFeed: &gofeed.Feed{
				Items: []*gofeed.Item{
					{
						Title:           "Old Article",
						Description:     "Old Description",
						Link:            "https://test.com/old-article",
						Content:         "Old Content",
						Categories:      []string{"security", "test"},
						PublishedParsed: createPointer(createTime(3)),
					},
				},
			},
			parseError:       nil,
			expectedArticles: 0,
			shouldUpdateTime: false,
		},
		{
			name: "error parsing feed",
			feed: config.RSSFeed{
				Name: "Test Feed",
				Url:  "https://test.com/feed",
			},
			lastPullTime:     createTime(5),
			parsedFeed:       nil,
			parseError:       errors.New("parse error"),
			expectedArticles: 0,
			shouldUpdateTime: false,
		},
		{
			name: "special date format parsing",
			feed: config.RSSFeed{
				Name: "CrowdStrike Feed",
				Url:  "https://crowdstrike.com/feed",
			},
			lastPullTime: createTime(5),
			parsedFeed: &gofeed.Feed{
				Items: []*gofeed.Item{
					{
						Title:           "CrowdStrike Article",
						Description:     "CrowdStrike Description",
						Link:            "https://crowdstrike.com/article",
						Content:         "CrowdStrike Content",
						Categories:      []string{"security", "threat"},
						PublishedParsed: nil,
						Published:       time.Now().Format("Jan 2, 2006 15:04:05-0700"),
					},
				},
			},
			parseError:       nil,
			expectedArticles: 1,
			shouldUpdateTime: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock parser
			mockParser := new(mockParser)
			mockParser.On("ParseURL", tt.feed.Url).Return(tt.parsedFeed, tt.parseError)

			// Create a feed with the mock parser
			f := &Feed{
				feedParser:      mockParser,
				lastItemPerFeed: make(map[string]time.Time),
			}
			f.lastItemPerFeed[tt.feed.Url] = tt.lastPullTime

			// Call the function under test
			articles, err := f.collectFeed(tt.feed)

			// Assertions
			if tt.parseError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedArticles, len(articles))

				if tt.shouldUpdateTime && len(articles) > 0 {
					assert.Equal(t, articles[0].Published, f.lastItemPerFeed[tt.feed.Url])
				} else {
					assert.Equal(t, tt.lastPullTime, f.lastItemPerFeed[tt.feed.Url])
				}
			}

			mockParser.AssertExpectations(t)
		})
	}
}

func TestCollectFeedNoMock(t *testing.T) {
	f := &Feed{
		feedParser:      gofeed.NewParser(),
		lastItemPerFeed: make(map[string]time.Time),
	}

	feed := config.RSSFeed{
		Name: "Test Feed",
		Url:  "https://www.bleepingcomputer.com/feed/",
	}

	// Pull everything
	f.lastItemPerFeed[feed.Url] = time.Unix(0, 0)

	articles, err := f.collectFeed(feed)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(articles), 1)
}

func TestFetchURLNoMock(t *testing.T) {
	url := "https://techcrunch.com/2025/03/07/japanese-telco-giant-ntt-com-says-hackers-accessed-details-of-almost-18000-organizations/"

	reader, err := fetchURL(url)
	require.NoError(t, err)
	require.NotNil(t, reader)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Greater(t, len(data), 0)
}

func TestEnrichArticleItemNoMock(t *testing.T) {
	var err error

	url := "https://techcrunch.com/2025/03/07/japanese-telco-giant-ntt-com-says-hackers-accessed-details-of-almost-18000-organizations/"

	article := types.Article{
		Title:   "Japanese telco giant NTT Com says hackers accessed details of almost 18,000 organizations",
		Link:    url,
		Content: "",
	}

	article, err = enrichArticleItem(article)
	require.NoError(t, err)

	// Checking that we removed all non-relevant HTML content.
	require.LessOrEqual(t, len(article.Content), 5000)

	// Checking that the article content contains the title.
	require.Contains(t, article.Content, "Japanese telecom giant")

	// Checking that the article content contains the title within the first 100 characters.
	require.LessOrEqual(t, strings.Index(article.Content, "Japanese telecom giant"), 100)
}
