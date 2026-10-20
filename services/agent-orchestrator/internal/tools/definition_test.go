package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/credentials"
)

// A model provider rejects a tool whose schema has "properties": null, and then every question fails.
func TestEveryToolSendsAnObjectForItsProperties(t *testing.T) {
	for _, tool := range Catalog(credentials.InboundForwarder{}) {
		raw, err := json.Marshal(tool.Definition())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"properties":null`) {
			t.Fatalf("%s sends null properties: %s", tool.Name(), raw)
		}
		var parsed struct {
			Parameters struct {
				Type       string         `json:"type"`
				Properties map[string]any `json:"properties"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Parameters.Type != "object" || parsed.Parameters.Properties == nil {
			t.Fatalf("%s: parameters must be an object schema with an object for properties: %s (%v)", tool.Name(), raw, err)
		}
	}
}
