package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/feed"
	"github.com/alex-ilgayev/secfeed/pkg/llm"
	"github.com/alex-ilgayev/secfeed/pkg/signal"
	"github.com/alex-ilgayev/secfeed/pkg/similarity"
	"github.com/alex-ilgayev/secfeed/pkg/types"
	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"

	nested "github.com/antonfisher/nested-logrus-formatter"
	log "github.com/sirupsen/logrus"
)

var (
	configFile      string
	verbose         bool
	intitPullInDays int
)

var rootCmd = &cobra.Command{
	Use:   "secfeed",
	Short: "Security Feed CLI tool",
	Long:  `A CLI tool fetching security feeds and vulnerability information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		setupLog()

		if err := start(); err != nil {
			return err
		}

		return nil
	},
}

func setupLog() {
	log.SetOutput(os.Stdout)

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.SetFormatter(&nested.Formatter{
		HideKeys:         true,
		TimestampFormat:  "2006-01-02 15:04:05",
		NoUppercaseLevel: true,
	})
}

func start() error {
	asciiArt()
	log.Info("Starting secfeed!")

	ctx, _ := signal.SetupHandler()

	cfg, err := config.New(configFile)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	feed, err := feed.New(cfg, time.Duration(intitPullInDays)*24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to create feed: %w", err)
	}

	llmClient, err := llm.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	similarity, err := similarity.New(ctx, llmClient, cfg.Categories, cfg.SimilarityThreshold)
	if err != nil {
		return fmt.Errorf("failed to create similarity engine: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer func() {
			if value := recover(); value != nil {
				log.Panicf("failed with panic: %s", debug.Stack())
			}
		}()

		for {
			select {
			case article := <-feed.Stream():
				articleLogFields := log.Fields{
					"title": article.Title,
					"link":  article.Link,
				}
				log.WithFields(articleLogFields).Info("Received article. Analyzing...")

				catMatches, err := similarity.CheckSimilarity(ctx, article)
				if err != nil {
					log.WithFields(articleLogFields).Errorf("failed to check category similarity: %v", err)
					continue
				}

				for _, catMatch := range catMatches {
					log.WithFields(articleLogFields).Infof("category %s is similar with relevance %d (Explanation: %s)", catMatch.Category, catMatch.Relevance, catMatch.Explanation)
				}
				if len(catMatches) > 0 {
					article.Summary, err = llmClient.Summarize(ctx, article)
					if err != nil {
						log.WithFields(articleLogFields).Errorf("failed to summarize article: %v", err)
						continue
					}
					printSummaryToStdout(article)
				}

				// fmt.Println("Article:", article.Link)
				// for _, catMatch := range catMatches {
				// 	fmt.Println(catMatch.Category, "->", catMatch.Relevance, "(Explanation:", catMatch.Explanation, ")")
				// }

				// categories, err := llmClient.ExtractCategories(ctx, article)
				// if err != nil {
				// 	log.WithFields(articleLogFields).Errorf("failed to extract categories: %v", err)
				// 	continue
				// }

				// if len(categories) == 0 {
				// 	log.WithFields(articleLogFields).Warn("no categories extracted, can't find similarity")
				// 	continue
				// }

				// sims, err := similarity.CheckSimilarity(ctx, categories)
				// if err != nil {
				// 	log.WithFields(articleLogFields).Errorf("failed to check similarity: %v", err)
				// 	continue
				// }

				// for cat, sim := range sims {
				// 	if sim >= cfg.SimilarityThreshold {
				// 		log.WithFields(articleLogFields).Debugf("category %s is similar with similarity %f", cat, sim)

				// 		summary, err := llmClient.Summarize(ctx, article)
				// 		if err != nil {
				// 			log.WithFields(articleLogFields).Errorf("failed to summarize article: %v", err)
				// 			continue
				// 		}

				// 		log.WithFields(articleLogFields).Info("Summarized article")
				// 		printSummaryToStdout(summary)
				// 	}
				// }
			case <-ctx.Done():
				wg.Done()
			}
		}
	}()

	feed.Start()
	wg.Wait()

	log.Info("Shutting down secfeed")

	return nil
}

func main() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yml", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().IntVarP(&intitPullInDays, "init-pull", "i", 0, "initial pull in days (default behavior is we analyze only new articles)")

	rootCmd.Flags().SortFlags = false

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func printSummaryToStdout(article types.Article) {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	out, _ := r.Render(article.FormatAsMarkdown())
	fmt.Print(out)
}

func asciiArt() {
	fmt.Println("                                                                          ")
	fmt.Println("  /$$$$$$                      /$$$$$$$$                         /$$ /$$")
	fmt.Println(" /$$__  $$                    | $$_____/                        | $$| $$")
	fmt.Println("| $$  \\__/  /$$$$$$   /$$$$$$$| $$      /$$$$$$   /$$$$$$   /$$$$$$$| $$")
	fmt.Println("|  $$$$$$  /$$__  $$ /$$_____/| $$$$$  /$$__  $$ /$$__  $$ /$$__  $$| $$")
	fmt.Println(" \\____  $$| $$$$$$$$| $$      | $$__/ | $$$$$$$$| $$$$$$$$| $$  | $$|__/")
	fmt.Println(" /$$  \\ $$| $$_____/| $$      | $$    | $$_____/| $$_____/| $$  | $$    ")
	fmt.Println("|  $$$$$$/|  $$$$$$$|  $$$$$$$| $$    |  $$$$$$$|  $$$$$$$|  $$$$$$$ /$$")
	fmt.Println(" \\______/  \\_______/ \\_______/|__/     \\_______/ \\_______/ \\_______/|__/")
	fmt.Println("                                                                          ")
}
