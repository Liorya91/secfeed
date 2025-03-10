package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type LLMClientType string

const (
	LLMClientTypeOpenAI LLMClientType = "openai"
	LLMClientTypeOllama LLMClientType = "ollama"
)

type ClassificationEngineType string

const (
	ClassificationEngineTypeLLM        ClassificationEngineType = "llm"
	ClassificationEngineTypeEmbeddings ClassificationEngineType = "embeddings"
)

type Config struct {
	InitPullInDays int        `yaml:"init_pull"`
	Reporting      Reporting  `yaml:"reporting"`
	LLM            LLM        `yaml:"llm"`
	Categories     []Category `yaml:"categories"`
	RssFeed        []RSSFeed  `yaml:"rss_feed"`
}

type Reporting struct {
	Slack  bool `yaml:"slack"`
	Stdout bool `yaml:"stdout"`
}

type LLM struct {
	Client         LLMClientType  `yaml:"client"`
	Classification Classification `yaml:"classification"`
	Summary        Summary        `yaml:"summary"`
}

type Classification struct {
	Engine    ClassificationEngineType `yaml:"engine"`
	Model     string                   `yaml:"model"`
	Threshold float32                  `yaml:"threshold"`
}

type Summary struct {
	Model string `yaml:"model"`
}

type Category struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type RSSFeed struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
}

// func (c *Config) SetDefaults() error {
// 	// if c.InitPullInDays == 0 {
// 	// 	c.InitPullInDays = 7
// 	// }

// 	// if c.Reporting.Slack == false {
// 	// 	c.Reporting.Slack = true
// 	// }

// 	// if c.Reporting.Stdout == false {
// 	// 	c.Reporting.Stdout = true
// 	// }

// 	// if c.LLM.Client == "" {
// 	// 	c.LLM.Client = OpenAI
// 	// }

// 	// if c.LLM.Classification.Engine == "" {
// 	// 	c.LLM.Classification.Engine = LLM
// 	// }

// 	// if c.LLM.Classification.Model == "" {
// 	// 	c.LLM.Classification.Model = "gpt-3.5-turbo"
// 	// }

// 	// if c.LLM.Classification.Threshold == 0 {
// 	// 	c.LLM.Classification.Threshold = 0.5
// 	// }

// 	// if c.LLM.Summary.Model == "" {
// 	// 	c.LLM.Summary.Model = "gpt-3.5-turbo"
// 	// }
// }

func New(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	// config.SetDefaults()

	return &config, nil
}
