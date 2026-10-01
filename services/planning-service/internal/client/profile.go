package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Profiles struct {
	Shared Peers
}

func (p Profiles) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Shared.SharedURL+"/api/v1/shared/profiles/me", nil)
	if err != nil {
		return nil, err
	}
	if b := auth.BearerFrom(ctx); b != "" {
		req.Header.Set("Authorization", b)
	}
	resp, err := p.Shared.http().Do(req)
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
