package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackWebhook posts events to a Slack incoming webhook. Incoming webhooks
// take a plain JSON payload, so this needs no Slack SDK — consistent with
// the stdlib-only core (ADR-001). The interactive Slack app (ack/resolve
// buttons, channel per incident) is a separate Phase 2 work item.
type SlackWebhook struct {
	URL    string
	Client *http.Client
}

func NewSlackWebhook(url string) *SlackWebhook {
	return &SlackWebhook{URL: url, Client: &http.Client{Timeout: 5 * time.Second}}
}

func (s *SlackWebhook) Send(ctx context.Context, ev Event) error {
	inc := ev.Incident

	var text string
	switch ev.Kind {
	case KindOpened:
		text = fmt.Sprintf(":rotating_light: *%s* [%s] opened — %s", inc.ID, inc.Severity, inc.Title)
	case KindResolved:
		text = fmt.Sprintf(":white_check_mark: *%s* resolved — %s", inc.ID, inc.Title)
	default:
		text = fmt.Sprintf("*%s* %s — %s", inc.ID, ev.Kind, inc.Title)
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %s", resp.Status)
	}
	return nil
}
