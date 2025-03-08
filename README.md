# SecFeed

![](./assets/secfeed-logo.png)

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://golang.org/doc/go1.24)

SecFeed is a powerful CLI tool that aggregates, analyzes, and filters security-related articles from multiple RSS feeds using large language models (LLMs). It automates the discovery of relevant security news based on user-defined categories, provides concise summaries, and can push notifications to Slack.

## Table of Contents

TBD

## Main Features

- **Intelligent filtering** of security articles using LLM-based categorization
- **Configurable categories** to focus on specific security domains
- Support for **multiple LLM backends** (OpenAI or Ollama)
- **Concise summaries** of relevant security articles
- **Extensible RSS feed system** with built-in content extraction
- **Slack integration** for real-time notifications

## Installation

### Prerequisites

- Go 1.24 or higher
- OpenAI API key or local Ollama setup

### From Source

```bash
# Clone the repository
git clone https://github.com/alex-ilgayev/secfeed.git
cd secfeed

# Build the binary
go build -o secfeed ./cmd/secfeed

# Optional: Install to your $GOPATH/bin
go install ./cmd/secfeed
```

### Using Docker

```bash
# Build the Docker image
docker build -t secfeed .

# Run with Docker
docker run -v $(pwd)/config.yml:/app/config.yml \
  -e OPENAI_API_KEY \
  -e SLACK_WEBHOOK_URL \
  secfeed
```

## Configuration

SecFeed uses a YAML configuration file to define categories for filtering articles and RSS feeds to monitor.

### Configuration File Structure

Basic configuration file together with all values is found [here](./config.yml).

```yaml
init_pull: 0
...

reporting:
  slack: false
  ...

llm:
  client: "openai"
  ...

categories:
  - name: Software Supply Chain
    description: Articles about software supply chain security, including best practices, tools, and case studies.
    ...

rss_feed:
  - url: https://feeds.feedburner.com/TheHackersNews
    name: The Hacker News
    ...
```

### Environment Variables

- `OPENAI_API_KEY`: Your OpenAI API key. Required when using OpenAI.
- `OLLAMA_BASE_URL`: Base URL for Ollama API. Required when using Ollama. defaults to http://localhost:11434.
- `SLACK_WEBHOOK_URL`: Webhook URL for Slack notifications

## Usage

```bash
secfeed --config config.yml
```

### Command Line Options

```
Usage:
  secfeed [flags]

Flags:
  -c, --config string   config file path (default "config.yml")
  -h, --help            help for secfeed
  -v, --verbose         verbose output
```

### Example Configs

**Running all articles from last 7 days**

```yaml
init_pull: 7
```

**Running on local Ollama model**

```yaml
llm:
  client: "ollama"
  classification:
    engine: "llm"
    model: "llama3.2"
    threshold: 7
  summary:
    model: "llama3.2"
```

**Running classification engine based on text embeddings**

Classifiaction based on text embeddings is still WIP, and needs to introduce more optimization ont he text so the results will be more reliabe.

```yaml
llm:
  client: "openai"
  classification:
    engine: "embeddings"
    model: "text-embedding-3-large"
    threshold: 4.2
  summary:
    model: "gpt-4o"
```

## Architecture

SecFeed is designed with modularity in mind, separating concerns into distinct packages:

### Flow Diagram

```

   ┌─────────────┐   ┌────────────────┐   ┌──────────┐
   │ RSS Sources ├──►│ Feed Fetcher   ├──►│ Enricher │
   └─────────────┘   └────────────────┘   └────┬─────┘
                                               │
                                               ▼
    ┌──────────┐       ┌────────────┐     ┌────────────────┐
    │ Slack    │◄──────┤ Summary    │◄────┤ Classification │
    └──────────┘       └──────┬─────┘     └───────┬────────┘
                              │                   │
                              ▼                   ▼
                       ┌───────────────────────────────────┐
                       │ LLM Client (OpenAI / Ollama)      │
                       └───────────────────────────────────┘
```

### Core Components

1. **Feed Fetcher** ([`feed`](./pkg/feed/))

   - Fetches articles from RSS feeds
   - Provides a stream of articles for processing
   - Can be extendable into fetching from additional sources, like LinkedIn and Twitter.

2. **Enricher** ([`feed.enrichArticleItem`](./pkg/feed/feed.go))

   - Usually RSS feeds coming without the content.
   - Smartly fetches the content from the blog.
   - Adding browesr-like headers to avoid being blocked.
   - Extracts the relevant information from the HTML.
   - Using [go-readability](https://github.com/go-shiori/go-readability) for the text cleaning task.

3. **Classification Engine** ([`classification`](./pkg/classification))

   - Analyzes articles for relevance using LLM or embeddings
   - Scores articles against user-defined categories
   - Filters out irrelevant content based on threshold

4. **LLM Client** ([`llm`](./pkg/llm/))

   - Abstracts interaction with language model providers
   - Supports OpenAI API and Ollama
   - Handles prompt engineering, result analysis, and input chunking.
   - Doing cost tracking only for OpenAI implementation.

5. **Summary** ([`llm.Summarize`](./pkg/llm/llm.go))

   - Summarizes the article to the concrete relevant action items.

6. **Slack** ([`slack`](./pkg/slack/))

   - Formats articles for Slack
   - Sends webhook notifications

### Classification Engine

Currently we provide two classification methods, that could be defined through `llm.classification.engine` config value.

1. **LLM Classification**

   - Uses direct LLM queries to evaluate article relevance. Giving the categories and their description to the prompt, and ask for relevance score from 0 to 10.
   - Provides detailed explanations for classifications
   - Higher accuracy but more token usage

2. **Embeddings Classification**
   - Uses vector embeddings to match articles to categories.
   - Pre-encode each category.
   - Encodes the article, and calculating relation to each category.
   - More efficient for token usage
   - Can't be trusted at the moment. Still WIP.

## Cost Management

16 hours
3 categories
TBD

## Contributing

Contributions are welcome! See the [TODOs](README.md#todos) in the README for planned features. Please feel free to submit pull requests or open issues for bugs and feature requests.

## TODOS:

### Important:

- Checking minimal article content that fits as an article
- Amortized cost
- write logs to some file, so could investigate classification issues
- some tests
- ci sanity
- remove debug printing to slack

### Nice to have:

- verify config when parsed, together with settings default values.
- extensive tests
- Reddit feed.
- Linkedin feed.
- Following article that just fowards to a URL (like in reddit?)
- Add option to choose between LLM filtering and embedding filtering
- Add tl;dr sec to feeds?
- Caching
- Ability to bypass Cloudflare when fetching URLs - needed for some feeds.
- Check more feeds:
  - security boulevard
  - bank info security
  - information week
  - infosecurity magazine
  - computerweekly
  - hackread
- Enforce JSON formatting in llm clients
- Embedding
  - generate similar words for improving the precision

## License

[Apache-2.0 License](./LICENSE)

---

Built with ❤️ for the security community
