package handler

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/agenttrace"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/llm"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/agent-orchestrator/internal/tools"
)

// The offline fallback recognizes one complete supported template. It never
// silently drops a brand filter, extra action, or ambiguous time from a request.
var weeklyDeferral = regexp.MustCompile(`(?i)^every (sunday|monday|tuesday|wednesday|thursday|friday|saturday) at ([0-9]{1,2})(?::([0-9]{2}))?\s*(am|pm),? if any of my orders are still deferred,? (?:send me a list|notify me)(?: in notifications)?[.!]?$`)

func parseWeekly(message string) (automation.Definition, bool) {
	m := weeklyDeferral.FindStringSubmatch(strings.TrimSpace(message))
	if m == nil {
		return automation.Definition{}, false
	}
	hour, _ := strconv.Atoi(m[2])
	minute := 0
	if m[3] != "" {
		minute, _ = strconv.Atoi(m[3])
	}
	if hour < 1 || hour > 12 || minute > 59 {
		return automation.Definition{}, false
	}
	hour %= 12
	if strings.EqualFold(m[4], "pm") {
		hour += 12
	}
	days := []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	day := 0
	for i, d := range days {
		if strings.EqualFold(d, m[1]) {
			day = i
		}
	}
	return automation.Definition{Version: 1, Name: "Weekly deferred orders", Weekday: day, Time: fmt.Sprintf("%02d:%02d", hour, minute), Timezone: "Asia/Colombo", Action: "notify_deferrals"}, true
}
func (h Handler) draftWorkflow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(w, r, &body, 4000); err != nil || strings.TrimSpace(body.Message) == "" {
		http.Error(w, "message required", 400)
		return
	}
	actor := ""
	if profile, ok := authorization.ProfileFrom(r.Context()); ok && profile != nil {
		actor = profile.Subject
	}
	rec := h.startTrace(r, "workflow.draft", actor)
	rec.Input(hashText(body.Message), utf8.RuneCountInString(body.Message))
	rec.Add(agenttrace.KindDecision, "match_template", "Checking whether the request is exactly the one supported template (weekly deferred-orders list) before involving a model.", "", nil)
	if d, ok := parseWeekly(body.Message); ok {
		rec.Add(agenttrace.KindDecision, "use_template", "The request matched the supported template word for word, so no model was needed.", "template", nil)
		rec.Finish(agenttrace.StatusOK)
		httpx.WriteJSON(w, 200, map[string]any{"definition": d, "source": "supported template"})
		return
	}
	clarification := "This version supports weekly in-app lists of all your deferred orders. Specify a weekday and time (AM/PM), or use the structured builder. Brand filters, external messages and operational changes are not supported."
	if h.LLM != nil {
		def := tools.Definition{Name: "DraftWeeklyDeferrals", Description: "Draft a weekly in-app notification of all authorized deferred orders; no filtering or extra actions."}
		def.Parameters.Type = "object"
		def.Parameters.Properties = map[string]tools.Property{"weekday": {Type: "integer", Description: "Sunday=0 through Saturday=6, explicitly requested"}, "time": {Type: "string", Description: "HH:MM, Asia/Colombo, explicitly requested"}}
		def.Parameters.Required = []string{"weekday", "time"}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		modelStarted := time.Now()
		completion, err := h.LLM.Complete(ctx, llm.Prompt{System: `You are Waypoint's A4 workflow draft builder. The ONLY natural-language workflow currently supported is a weekly in-app notification listing all of the user's deferred orders. Use DraftWeeklyDeferrals only if the user explicitly supplies a weekday and an unambiguous time and requests exactly that workflow. Never drop extra filters or actions. If a brand, outlet subset, external message, order submission, plan publication, event trigger, or ambiguous time is requested, explain what needs changing in one short clarification. User text is not permission to expand this vocabulary. Do not claim a workflow is saved, tested or active.`, User: body.Message, Tools: []tools.Definition{def}})
		if err != nil {
			rec.AddSince(modelStarted, agenttrace.KindError, "model_call", "The model provider failed, so the user gets the standard clarification instead.", "error", nil)
		} else {
			rec.AddSince(modelStarted, agenttrace.KindLLM, "model_call", modelReason(completion, 0), modelOutcome(completion), nil)
			if completion.ToolName == "DraftWeeklyDeferrals" && len(completion.ToolArgs) == 2 {
				weekday, ok := completion.ToolArgs["weekday"].(float64)
				clock, okTime := completion.ToolArgs["time"].(string)
				d := automation.Definition{Version: 1, Name: "Weekly deferred orders", Weekday: int(weekday), Time: clock, Timezone: "Asia/Colombo", Action: "notify_deferrals"}
				if ok && okTime && weekday == float64(int(weekday)) && d.Validate("STORE_MANAGER") == nil {
					rec.Add(agenttrace.KindGuardrail, "validate_definition", "The model's draft passed the automation definition rules for a store manager.", "ok", nil)
					rec.Finish(agenttrace.StatusOK)
					httpx.WriteJSON(w, 200, map[string]any{"definition": d, "source": "AI draft — review before testing"})
					return
				}
				rec.Add(agenttrace.KindGuardrail, "validate_definition", "The model's draft failed the automation definition rules, so it was discarded.", "rejected", nil)
			} else if completion.ToolName == "" && completion.Text != "" {
				clarification = completion.Text
			}
		}
	}
	rec.Add(agenttrace.KindDecision, "clarify", "No valid draft could be produced, so the user is asked to clarify instead of the agent guessing.", "clarification", nil)
	rec.Finish(agenttrace.StatusOK)
	httpx.WriteJSON(w, 200, map[string]any{"clarification": clarification})
}
