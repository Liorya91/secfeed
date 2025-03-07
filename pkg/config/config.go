package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Categories   []Category `yaml:"categories"`
	ClsThreshold float32    `yaml:"classification_threshold"`
	RssFeed      []RSSFeed  `yaml:"rss_feed"`
}

type Category struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type RSSFeed struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
}

func New(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &config, nil
}
