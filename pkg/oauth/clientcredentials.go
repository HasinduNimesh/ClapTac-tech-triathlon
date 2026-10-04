package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type TokenSource struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scope        string
	// Resource is the API the token is for (RFC 8707), for example the API's identifier. A real
	// identity provider that defines scopes on a resource server refuses a scope request that does not
	// name it (HTTP 400), so every service that asks for scopes sets it.
	Resource string
	HTTP     *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.exp.Add(-30*time.Second)) {
		return s.token, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {s.ClientID},
		"client_secret": {s.ClientSecret},
	}
	if s.Scope != "" {
		form.Set("scope", s.Scope)
	}
	if s.Resource != "" {
		form.Set("resource", s.Resource)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(body))
		// Without this a refused token shows up only as a bare 500 from whatever needed the token.
		slog.Error("client_credentials_refused", "client_id", s.ClientID, "token_url", s.TokenURL, "scope", s.Scope, "resource", s.Resource, "status", resp.StatusCode, "response", detail)
		return "", fmt.Errorf("client_credentials: %s", detail)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	s.token = out.AccessToken
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 300
	}
	s.exp = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return s.token, nil
}
