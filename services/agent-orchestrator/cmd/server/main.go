package main

import (
	"log"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/bootstrap"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/approvals"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/credentials"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/handler"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/identity"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/llm"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/tools"
)

func main() {
	app, err := bootstrap.Setup("agent-orchestrator", "")
	if err != nil {
		log.Fatal(err)
	}
	creds := credentials.InboundForwarder{}
	var model llm.Client = llm.Disabled{}
	if strings.TrimSpace(os.Getenv("LLM_BASE_URL")) != "" && strings.TrimSpace(os.Getenv("LLM_MODEL")) != "" {
		model = llm.OpenAICompatible{BaseURL: os.Getenv("LLM_BASE_URL"), APIKey: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL")}
	}
	h := handler.Handler{
		Authn:     app.Authenticator,
		Profiles:  identity.Profiles{BaseURL: os.Getenv("SHARED_SERVICE_URL")},
		LLM:       model,
		Tools:     tools.Catalog(creds),
		Approvals: approvals.NewMemory(),
		Publisher: audit.LogPublisher{Logger: app.Logger, Source: "agent-orchestrator"},
		Creds:     creds,
	}
	if err := app.Run(func(r chi.Router) { h.Routes(r) }); err != nil {
		log.Fatal(err)
	}
}
