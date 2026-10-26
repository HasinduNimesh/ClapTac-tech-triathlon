// Package handovers validates reviewed handover snapshots. A snapshot is an
// author's report, not an authoritative mutation of the underlying operations.
package handovers

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type Task struct {
	ID        string `json:"id"`
	Priority  string `json:"priority"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
	Reference string `json:"reference"`
	Action    string `json:"action"`
	To        string `json:"to"`
	DueAt     string `json:"dueAt,omitempty"`
}
type Snapshot struct {
	Title string    `json:"title"`
	Notes string    `json:"notes"`
	AsOf  time.Time `json:"asOf"`
	Tasks []Task    `json:"tasks"`
}
type Record struct {
	ID               string     `json:"id"`
	Role             string     `json:"role"`
	Scope            string     `json:"scope"`
	AuthorID         string     `json:"authorId"`
	AuthorName       string     `json:"authorName"`
	Snapshot         Snapshot   `json:"snapshot"`
	PublishedAt      time.Time  `json:"publishedAt"`
	AcknowledgedBy   string     `json:"acknowledgedBy,omitempty"`
	AcknowledgedName string     `json:"acknowledgedName,omitempty"`
	AcknowledgedAt   *time.Time `json:"acknowledgedAt,omitempty"`
}

var UUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var dispatcherLink = regexp.MustCompile(`^/dispatcher/(recovery|live|planning|notifications)(\?date=\d{4}-\d{2}-\d{2})?$`)
var storeLink = regexp.MustCompile(`^/store-manager/(recovery\?date=\d{4}-\d{2}-\d{2}|receipts\?order=[A-Za-z0-9%._~-]+|orders/[A-Za-z0-9%._~-]+/(track|timeline))$`)

func Authorized(p *authorization.Profile, role, scope string) bool {
	if p == nil || p.UserID == "" {
		return false
	}
	for _, r := range p.Roles {
		if role == "dispatcher" && scope == "fleet" && r == "DISPATCHER" {
			return true
		}
		if role == "store" && r == "STORE_MANAGER" && scope != "fleet" {
			for _, id := range p.OutletIDs {
				if id == scope && scope != "" {
					return true
				}
			}
		}
	}
	return false
}
func Validate(s Snapshot, role string) error {
	if strings.TrimSpace(s.Title) == "" || utf8.RuneCountInString(s.Title) > 100 || utf8.RuneCountInString(s.Notes) > 4000 {
		return fmt.Errorf("title is required (100 characters); notes allow 4000 characters")
	}
	if s.AsOf.IsZero() || s.Tasks == nil || len(s.Tasks) > 500 {
		return fmt.Errorf("snapshot time and up to 500 tasks are required")
	}
	seen := map[string]bool{}
	for _, t := range s.Tasks {
		if t.ID == "" || len(t.ID) > 200 || seen[t.ID] {
			return fmt.Errorf("task IDs must be unique and nonempty")
		}
		seen[t.ID] = true
		if t.Priority != "Critical" && t.Priority != "High" && t.Priority != "Normal" {
			return fmt.Errorf("invalid task priority")
		}
		if t.Title == "" || len(t.Title) > 600 || len(t.Reason) > 3000 || len(t.Reference) > 600 || t.Action == "" || len(t.Action) > 300 {
			return fmt.Errorf("invalid task text")
		}
		valid := role == "dispatcher" && dispatcherLink.MatchString(t.To) || role == "store" && storeLink.MatchString(t.To)
		if !valid {
			return fmt.Errorf("invalid task action for this role")
		}
		if t.DueAt != "" {
			if _, err := time.Parse(time.RFC3339, t.DueAt); err != nil {
				return fmt.Errorf("invalid task time")
			}
		}
	}
	return nil
}
func Fresh(asOf, now time.Time) bool {
	return !asOf.Before(now.Add(-5*time.Minute)) && !asOf.After(now.Add(30*time.Second))
}
