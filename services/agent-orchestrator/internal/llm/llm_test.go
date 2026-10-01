package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisabledProviderFailsClearly(t *testing.T) {
	if _, err := (Disabled{}).Complete(context.Background(), Prompt{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenAICompatibleParsesAllowlistedToolCall(t *testing.T) {
	var authHeader, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"GetOrders","arguments":"{\"status\":\"confirmed\"}"}}]}}]}`))
	}))
	defer srv.Close()
	c := OpenAICompatible{BaseURL: srv.URL, APIKey: "provider-secret", Model: "test-model"}
	out, err := c.Complete(context.Background(), Prompt{System: "system", User: "show confirmed orders"})
	if err != nil {
		t.Fatal(err)
	}
	if authHeader != "Bearer provider-secret" || out.ToolName != "GetOrders" || out.ToolArgs["status"] != "confirmed" {
		t.Fatalf("bad result: %+v auth=%q", out, authHeader)
	}
	if strings.Contains(body, "user-token") {
		t.Fatal("human token was sent to the LLM provider")
	}
}
