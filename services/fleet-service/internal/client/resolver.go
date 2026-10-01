package client

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Profiles struct {
	SharedURL string
	HTTP      *http.Client
}

func (p Profiles) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	base := p.SharedURL
	if base == "" {
		base = os.Getenv("SHARED_SERVICE_URL")
	}
	if base == "" {
		base = "http://shared-service:8080"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/shared/profiles/me", nil)
	if err != nil {
		return nil, err
	}
	if b := auth.BearerFrom(ctx); b != "" {
		req.Header.Set("Authorization", b)
	}
	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &authorization.Profile{Subject: subject}, nil
	}
	var out struct {
		Profile authorization.Profile `json:"profile"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Profile.Subject == "" {
		out.Profile.Subject = subject
	}
	return &out.Profile, nil
}
