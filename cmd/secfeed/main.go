package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/alex-ilgayev/secfeed/pkg/classification"
	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/constants"
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
	configFile      string
	verbose         bool
	intitPullInDays int
	slackEnabled    bool
	modelClsLLM     string
	modelClsEmb     string
	modelSummary    string
	llmClient       llm.LLMClientType                       = llm.OpenAI
	clsEngineType   classification.ClassificationEngineType = classification.LLM
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
		HideKeys:         false,
		TimestampFormat:  "2006-01-02 15:04:05",
		NoUppercaseLevel: true,
	})
}

func start() error {
	banner()
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

	llmClient, err := llm.NewClient(ctx, llmClient, modelClsLLM, modelClsEmb, modelSummary)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	clsEngine, err := classification.New(ctx, clsEngineType, llmClient, cfg.Categories, cfg.ClsThreshold)
	if err != nil {
		return fmt.Errorf("failed to create classification engine: %w", err)
	}

	var slackClient *slack.Slack
	if slackEnabled {
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
					printSummaryToStdout(article)

					if slackClient != nil {
						if err := slackClient.SendWebhook(ctx, article.FormatAsSlackMrkdwn()); err != nil {
							log.WithFields(articleLogFields).Errorf("failed to send slack webhook: %v", err)
						}
					}
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
	rootCmd.PersistentFlags().BoolVarP(&slackEnabled, "slack", "s", false, fmt.Sprintf("send notifications to slack (requires %s env variable)", constants.EnvSlackWebhookUrl))
	rootCmd.PersistentFlags().StringVar(&modelClsLLM, "model-cls-llm", "gpt-4o-mini", "model name that will be used for initial classification if LLM engine was choosed (preferably a smaller model)")
	rootCmd.PersistentFlags().StringVar(&modelClsEmb, "model-cls-emb", "text-embedding-3-large", "model name that will be used for initial classification if embeddings engine was choosed (preferably a smaller model)")
	rootCmd.PersistentFlags().StringVar(&modelSummary, "model-summary", "gpt-4o", "model name that will be used for summarization")
	rootCmd.PersistentFlags().VarP(&llmClient, "llm", "l", "LLM client to use (openai - default, or ollama)")
	rootCmd.PersistentFlags().Var(&clsEngineType, "cls-type", "classification engine to use (llm - default, or embeddings)")
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
