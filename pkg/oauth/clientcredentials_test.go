package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTokenRequestNamesTheResourceItsScopesBelongTo(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
	}))
	defer srv.Close()

	source := &TokenSource{TokenURL: srv.URL, ClientID: "waypoint-delivery-service", ClientSecret: "s", Scope: "loading:read-internal", Resource: "https://waypoint.claptac.dev/api/v1"}
	if _, err := source.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if form.Get("resource") != "https://waypoint.claptac.dev/api/v1" || form.Get("scope") != "loading:read-internal" || form.Get("grant_type") != "client_credentials" {
		t.Fatalf("token request = %v", form)
	}

	form = nil
	if _, err := (&TokenSource{TokenURL: srv.URL, ClientID: "c", ClientSecret: "s"}).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, sent := form["resource"]; sent {
		t.Fatalf("no resource configured means none is sent: %v", form)
	}
}

func TestRefusedTokenRequestIsAnErrorCarryingTheProvidersAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_scope"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := (&TokenSource{TokenURL: srv.URL, ClientID: "c", ClientSecret: "s", Scope: "x"}).Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid_scope") {
		t.Fatalf("err = %v", err)
	}
}

// A service that asks for scopes without naming the API gets HTTP 400 from a real identity provider and
// every call that needs its token fails. This is what took trips and loading down in production once.
func TestEveryServiceThatRequestsScopesAlsoNamesTheResource(t *testing.T) {
	mains, err := filepath.Glob("../../services/*/cmd/server/main.go")
	if err != nil || len(mains) == 0 {
		t.Fatalf("no service mains found: %v", err)
	}
	// One oauth.TokenSource literal, up to its closing brace (they never nest braces).
	literal := regexp.MustCompile(`oauth\.TokenSource\{[^{}]*\}`)
	checked := 0
	for _, path := range mains {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range literal.FindAllString(string(source), -1) {
			if !strings.Contains(match, "Scope:") {
				continue
			}
			checked++
			if !strings.Contains(match, "Resource:") {
				t.Errorf("%s requests scopes without a Resource:\n%s", path, match)
			}
		}
	}
	if checked < 7 {
		t.Fatalf("expected the seven services' token sources, found %d", checked)
	}
}
