package guardrails

import (
	"errors"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/tools"
)

var ErrRequiresApproval = errors.New("sensitive agent action requires human approval")

func Check(tool tools.Tool, approved bool) error {
	if tool.Sensitive() && !approved {
		return ErrRequiresApproval
	}
	return nil
}
