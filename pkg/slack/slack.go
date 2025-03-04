package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/alex-ilgayev/secfeed/pkg/constants"
	log "github.com/sirupsen/logrus"
)

type Slack struct {
	httpClient *http.Client
	endpoint   string
}

func New() (*Slack, error) {
	webhook := os.Getenv(constants.EnvSlackWebhookUrl)
	if webhook == "" {
		return nil, fmt.Errorf("%s environment variable is not set", constants.EnvSlackWebhookUrl)
	}

	return &Slack{
		httpClient: &http.Client{},
		endpoint:   webhook,
	}, nil
}

func (s *Slack) SendWebhook(ctx context.Context, data string) error {
	log.Debug("Sending webhook to Slack")

	payload := map[string]interface{}{
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": data,
				},
			},
		},
	}

	var jsonBody bytes.Buffer
	if err := json.NewEncoder(&jsonBody).Encode(payload); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, s.endpoint, &jsonBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed connection with status code %d", res.StatusCode)
	}

	return nil
}
