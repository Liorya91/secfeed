package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/classification"
	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/feed"
	"github.com/alex-ilgayev/secfeed/pkg/llm"
	"github.com/alex-ilgayev/secfeed/pkg/signal"
	"github.com/alex-ilgayev/secfeed/pkg/slack"
	"github.com/alex-ilgayev/secfeed/pkg/types"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/charmbracelet/glamour"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	configFile    string
	verbose       bool
	logFileOutput string
)

var rootCmd = &cobra.Command{
	Use:   "secfeed",
	Short: "Security Feed CLI tool",
	Long:  `A CLI tool fetching security feeds and vulnerability information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := setupLog(); err != nil {
			return err
		}

		if err := start(); err != nil {
			return err
		}

		return nil
	},
}

func setupLog() error {
	output := io.Writer(os.Stdout)

	// If log file is specified, create a multi-writer
	if logFileOutput != "" {
		logFile, err := os.OpenFile(logFileOutput, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		// Use MultiWriter to write to both stdout and the file
		output = io.MultiWriter(os.Stdout, logFile)
	}

	log.SetOutput(output)

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.SetFormatter(&nested.Formatter{
		HideKeys:         false,
		TimestampFormat:  "2006-01-02 15:04:05",
		NoUppercaseLevel: true,
	})

	return nil
}

func start() error {
	banner()
	log.Info("Starting secfeed!")

	ctx, _ := signal.SetupHandler()

	cfg, err := config.New(configFile)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	feed, err := feed.New(cfg, time.Duration(cfg.InitPullInDays)*24*time.Hour)
	if err != nil {
		return fmt.Errorf("failed to create feed: %w", err)
	}

	llmClient, err := llm.NewClient(ctx, cfg.LLM)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	clsEngine, err := classification.New(ctx, cfg.LLM.Classification, llmClient, cfg.Categories)
	if err != nil {
		return fmt.Errorf("failed to create classification engine: %w", err)
	}

	var slackClient *slack.Slack
	if cfg.Reporting.Slack {
		slackClient, err = slack.New()
		if err != nil {
			return fmt.Errorf("failed to create slack client: %w", err)
		}
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

				var catMatches []types.CategoryRelevance
				catMatches, err = clsEngine.Classify(ctx, article)
				if err != nil {
					log.WithFields(articleLogFields).Errorf("failed to classify the article: %v", err)
					continue
				}
				article.CatRelevance = catMatches

				for _, catMatch := range catMatches {
					log.WithFields(articleLogFields).Infof("category %s is similar with relevance %.1f (Explanation: %s)", catMatch.Category, catMatch.Relevance, catMatch.Explanation)
				}
				if len(catMatches) > 0 {
					article.Summary, err = llmClient.Summarize(ctx, article)
					if err != nil {
						log.WithFields(articleLogFields).Errorf("failed to summarize article: %v", err)
						continue
					}

					if cfg.Reporting.Stdout {
						printSummaryToStdout(article)
					}

					if slackClient != nil {
						if err := slackClient.SendWebhook(ctx, article.FormatAsSlackMrkdwn()); err != nil {
							log.WithFields(articleLogFields).Errorf("failed to send slack webhook: %v", err)
						}
					}
				}
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
	rootCmd.PersistentFlags().StringVarP(&logFileOutput, "log-file", "l", "", "log file path")

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

func banner() {
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
