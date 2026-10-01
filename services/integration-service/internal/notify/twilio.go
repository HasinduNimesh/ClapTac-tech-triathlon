package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const twilioAPI = "https://api.twilio.com/2010-04-01/Accounts/"

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

type TwilioConfig struct {
	AccountSID          string
	AuthToken           string
	From                string
	MessagingServiceSID string
	StatusCallbackURL   string
}

type SMS struct {
	To   string
	Body string
}

type Twilio struct {
	config   TwilioConfig
	client   *http.Client
	endpoint string
}

func NewTwilio(config TwilioConfig, client *http.Client) (*Twilio, error) {
	config.AccountSID = strings.TrimSpace(config.AccountSID)
	config.AuthToken = strings.TrimSpace(config.AuthToken)
	config.From = strings.TrimSpace(config.From)
	config.MessagingServiceSID = strings.TrimSpace(config.MessagingServiceSID)
	if len(config.AccountSID) != 34 || !strings.HasPrefix(config.AccountSID, "AC") || config.AuthToken == "" {
		return nil, fmt.Errorf("Twilio account SID and auth token are required")
	}
	if (config.From == "") == (config.MessagingServiceSID == "") {
		return nil, fmt.Errorf("configure exactly one Twilio sender: From or MessagingServiceSID")
	}
	if config.From != "" && !e164.MatchString(config.From) {
		return nil, fmt.Errorf("Twilio From must be an E.164 phone number")
	}
	if config.MessagingServiceSID != "" && (len(config.MessagingServiceSID) != 34 || !strings.HasPrefix(config.MessagingServiceSID, "MG")) {
		return nil, fmt.Errorf("Twilio messaging service SID is invalid")
	}
	if config.StatusCallbackURL != "" {
		callback, err := url.Parse(config.StatusCallbackURL)
		if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.User != nil {
			return nil, fmt.Errorf("Twilio status callback must be an absolute HTTPS URL")
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &Twilio{config: config, client: client, endpoint: twilioAPI + url.PathEscape(config.AccountSID) + "/Messages.json"}, nil
}

// Send creates one Twilio Message resource. It deliberately does not retry:
// the Messages create endpoint does not document an idempotency key, so a
// timeout after provider acceptance can have an ambiguous outcome.
func (t *Twilio) Send(ctx context.Context, sms SMS) (string, error) {
	sms.To = strings.TrimSpace(sms.To)
	sms.Body = strings.TrimSpace(sms.Body)
	if !e164.MatchString(sms.To) || sms.Body == "" || len([]rune(sms.Body)) > 1600 {
		return "", fmt.Errorf("SMS recipient or body is invalid")
	}
	form := url.Values{"To": {sms.To}, "Body": {sms.Body}}
	if t.config.From != "" {
		form.Set("From", t.config.From)
	} else {
		form.Set("MessagingServiceSid", t.config.MessagingServiceSID)
	}
	if t.config.StatusCallbackURL != "" {
		form.Set("StatusCallback", t.config.StatusCallbackURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(t.config.AccountSID, t.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("SMS provider request outcome is unknown: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("SMS provider rejected request with HTTP %d", resp.StatusCode)
	}
	var result struct {
		SID string `json:"sid"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.SID == "" {
		return "", fmt.Errorf("SMS provider accepted request without a usable message ID; outcome requires reconciliation")
	}
	return result.SID, nil
}
