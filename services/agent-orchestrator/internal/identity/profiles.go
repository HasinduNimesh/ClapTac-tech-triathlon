package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Profiles struct {
	BaseURL string
	HTTP    *http.Client
}

func (p Profiles) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, errors.New("shared service URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/shared/profiles/me", nil)
	if err != nil {
		return nil, err
	}
	if token := auth.BearerFrom(ctx); token != "" {
		req.Header.Set("Authorization", token)
	}
	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("profile lookup failed")
	}
	var out struct {
		Profile authorization.Profile `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Profile.Subject != "" && out.Profile.Subject != subject {
		return nil, errors.New("profile subject mismatch")
	}
	if out.Profile.Subject == "" {
		out.Profile.Subject = subject
	}
	return &out.Profile, nil
}
