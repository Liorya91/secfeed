package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/alex-ilgayev/secfeed/pkg/config"
	"github.com/alex-ilgayev/secfeed/pkg/feed"
	"github.com/alex-ilgayev/secfeed/pkg/llm"
	"github.com/alex-ilgayev/secfeed/pkg/signal"
	"github.com/alex-ilgayev/secfeed/pkg/similarity"
	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"

	nested "github.com/antonfisher/nested-logrus-formatter"
	log "github.com/sirupsen/logrus"
)

var (
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "secbulletin",
	Short: "Security Bulletin CLI tool",
	Long: `A CLI tool getting for managing security bulletins 
and vulnerability information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initVars(cmd)

		if err := start(); err != nil {
			return err
		}

		return nil
	},
}

func initVars(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&configFile, "config", "config.yml", "config file path")

	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
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

	feed, err := feed.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create feed: %w", err)
	}

	llmClient, err := llm.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	similarity, err := similarity.New(ctx, llmClient, cfg.Categories)
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

				categories, err := llmClient.ExtractCategories(ctx, article)
				if err != nil {
					log.WithFields(articleLogFields).Errorf("failed to extract categories: %v", err)
					continue
				}

				sims, err := similarity.CheckSimilarity(ctx, categories)
				if err != nil {
					log.WithFields(articleLogFields).Errorf("failed to check similarity: %v", err)
					continue
				}

				for cat, sim := range sims {
					if sim >= cfg.SimilarityThreshold {
						log.WithFields(articleLogFields).Debugf("category %s is similar with similarity %f", cat, sim)

						summary, err := llmClient.Summarize(ctx, article)
						if err != nil {
							log.WithFields(articleLogFields).Errorf("failed to summarize article: %v", err)
							continue
						}

						r, _ := glamour.NewTermRenderer(
							glamour.WithAutoStyle(),
							glamour.WithWordWrap(80),
						)

						out, _ := r.Render(summary)
						fmt.Print(out)
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
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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
