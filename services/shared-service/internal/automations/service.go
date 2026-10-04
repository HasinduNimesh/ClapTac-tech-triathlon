// Package automations owns deterministic habit detection and workflow execution.
// AI never has database access and cannot execute the workflows itself.
package automations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Pool        *pgxpool.Pool
	Profiles    authorization.ProfileResolver
	PlanningURL string
	Tokens      interface {
		Token(context.Context) (string, error)
	}
	HTTP     *http.Client
	Now      func() time.Time
	Snapshot func(context.Context, time.Time) (automation.Snapshot, error) // deterministic test boundary
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
func role(p *authorization.Profile) string {
	if p == nil || len(p.Roles) != 1 {
		return ""
	}
	return p.Roles[0]
}
func allowed(p *authorization.Profile) bool {
	return p != nil && p.UserID != "" && (role(p) == "STORE_MANAGER" || role(p) == "DISPATCHER")
}
func owns(p *authorization.Profile, outlet string) bool {
	if role(p) == "DISPATCHER" {
		return true
	}
	for _, id := range p.OutletIDs {
		if id == outlet {
			return true
		}
	}
	return false
}
func validate(d automation.Definition, p *authorization.Profile) error {
	if err := d.Validate(role(p)); err != nil {
		return err
	}
	if d.Prefill != nil && !owns(p, d.Prefill.OutletID) {
		return errors.New("template outlet is no longer authorized")
	}
	return nil
}
func marshal(v any) []byte { b, _ := json.Marshal(v); return b }
func key(v any) string     { h := sha256.Sum256(marshal(v)); return hex.EncodeToString(h[:]) }
func auditTx(ctx context.Context, tx pgx.Tx, owner, action, resource string, data any) error {
	actorType := "human"
	if action == "AUTOMATION_RUN" || action == "AUTOMATION_PAUSED_PERMISSION" {
		actorType = "workflow"
	}
	_, err := tx.Exec(ctx, `INSERT INTO audit.events(event_id,actor_id,actor_type,action,resource_type,resource_id,new_state,timestamp,source)
 VALUES(gen_random_uuid()::text,$1,$5,$2,'AUTOMATION',$3,$4,now(),'shared-service')`, owner, action, resource, marshal(data), actorType)
	return err
}
func (s Service) snapshot(ctx context.Context, p *authorization.Profile, at time.Time) (automation.Snapshot, error) {
	if !allowed(p) {
		return automation.Snapshot{}, errors.New("owner no longer authorized")
	}
	var out automation.Snapshot
	if s.Snapshot != nil {
		var err error
		out, err = s.Snapshot(ctx, at)
		if err != nil {
			return out, err
		}
	} else {
		token, err := s.Tokens.Token(ctx)
		if err != nil {
			return out, err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", s.PlanningURL+"/api/v1/planning/internal/automation-snapshot?at="+url.QueryEscape(at.Format(time.RFC3339)), nil)
		if err != nil {
			return out, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		client := s.HTTP
		if client == nil {
			client = &http.Client{Timeout: 8 * time.Second}
		}
		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return out, errors.New("planning history unavailable")
		}
		if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out); err != nil {
			return out, err
		}
	}
	filtered := []automation.Deferred{}
	for _, d := range out.Items {
		if owns(p, d.OutletID) {
			filtered = append(filtered, d)
		}
	}
	out.Items = filtered
	if at.Before(out.HistorySince) {
		return out, fmt.Errorf("history starts %s; choose a later test time", out.HistorySince.Format(time.RFC3339))
	}
	return out, nil
}

type Result struct {
	At      time.Time             `json:"at"`
	Items   []automation.Deferred `json:"items"`
	Prefill *automation.Prefill   `json:"prefill,omitempty"`
	Message string                `json:"message"`
}

func (s Service) evaluate(ctx context.Context, p *authorization.Profile, d automation.Definition, at time.Time) (Result, error) {
	out := Result{At: at, Items: []automation.Deferred{}}
	if err := validate(d, p); err != nil {
		return out, err
	}
	if d.Action == "prefill_order" {
		out.Prefill = d.Prefill
		out.Message = "Prepare an order draft for review; nothing is submitted."
		return out, nil
	}
	snap, err := s.snapshot(ctx, p, at)
	if err != nil {
		return out, err
	}
	for _, item := range snap.Items {
		if d.Action != "priority_review" || item.Count >= 2 {
			out.Items = append(out.Items, item)
		}
	}
	out.Message = fmt.Sprintf("%d deferred orders match. No orders or plans are changed.", len(out.Items))
	return out, nil
}

type Habit struct {
	ID       string              `json:"id"`
	Pattern  string              `json:"pattern"`
	Action   string              `json:"action"`
	Evidence string              `json:"evidence"`
	Prefill  *automation.Prefill `json:"prefill,omitempty"`
	OutletID string              `json:"outletId,omitempty"`
	YesCount int                 `json:"yesCount"`
	NoCount  int                 `json:"noCount"`
}

func (s Service) habits(ctx context.Context, p *authorization.Profile) ([]Habit, error) {
	out := []Habit{}
	var enabled bool
	if err := s.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM shared.automation_preferences WHERE owner_id=$1),true)`, p.UserID).Scan(&enabled); err != nil {
		return out, err
	}
	if !enabled {
		return out, nil
	}
	now := s.now()
	loc, _ := time.LoadLocation("Asia/Colombo")
	date := now.In(loc).Format("2006-01-02")
	candidates := []Habit{}
	if role(p) == "STORE_MANAGER" {
		rows, err := s.Pool.Query(ctx, `SELECT new_state,timestamp FROM audit.events WHERE actor_id=$1 AND action='order.created' AND timestamp>=$2 AND timestamp<=$3 ORDER BY timestamp`, p.UserID, now.AddDate(0, 0, -28), now)
		if err != nil {
			return out, err
		}
		type group struct {
			p    automation.Prefill
			days map[string]bool
		}
		groups := map[string]*group{}
		for rows.Next() {
			var raw []byte
			var at time.Time
			if err = rows.Scan(&raw, &at); err != nil {
				rows.Close()
				return out, err
			}
			var pre automation.Prefill
			if json.Unmarshal(raw, &pre) != nil || !pre.Valid() || !owns(p, pre.OutletID) || at.In(loc).Weekday() != now.In(loc).Weekday() {
				continue
			}
			k := key(pre)
			if groups[k] == nil {
				groups[k] = &group{pre, map[string]bool{}}
			}
			groups[k].days[at.In(loc).Format("2006-01-02")] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		for k, g := range groups {
			if len(g.days) >= 3 && !g.days[date] {
				pre := g.p
				candidates = append(candidates, Habit{Pattern: "order:" + k, Action: "prefill_order", Prefill: &pre, Evidence: fmt.Sprintf("You placed this %d-unit %s order on %d %ss in the last 4 weeks. Fill the form for review?", pre.Units, pre.Temperature, len(g.days), now.In(loc).Weekday())})
			}
		}
	} else {
		var n int
		err := s.Pool.QueryRow(ctx, `SELECT count(DISTINCT timestamp::date) FROM audit.events WHERE actor_id=$1 AND action='PRIORITY_REVIEW_MARKED' AND timestamp>=$2 AND timestamp<=$3`, p.UserID, now.AddDate(0, 0, -14), now).Scan(&n)
		if err != nil {
			return out, err
		}
		if n >= 3 {
			snap, err := s.snapshot(ctx, p, now)
			if err != nil {
				return out, err
			}
			seen := map[string]bool{}
			for _, d := range snap.Items {
				if d.Count < 2 || seen[d.OutletID] {
					continue
				}
				seen[d.OutletID] = true
				candidates = append(candidates, Habit{Pattern: "repeat-deferral-review", Action: "priority_review", OutletID: d.OutletID, Evidence: fmt.Sprintf("You flagged repeat-deferred outlets for review on %d days in 2 weeks. Flag %s for the next planning review?", n, d.OutletID)})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Pattern+candidates[i].OutletID < candidates[j].Pattern+candidates[j].OutletID
	})
	for _, c := range candidates {
		var yes, no int
		var suppressed bool
		err := s.Pool.QueryRow(ctx, `SELECT yes_count,no_count,suppressed FROM shared.habit_feedback WHERE owner_id=$1 AND pattern_key=$2`, p.UserID, c.Pattern).Scan(&yes, &no, &suppressed)
		if err != nil && err != pgx.ErrNoRows {
			return out, err
		}
		if suppressed || no >= 2 {
			continue
		}
		c.YesCount = yes
		c.NoCount = no
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return out, err
		}
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO shared.habit_suggestions(owner_id,pattern_key,occurrence_key,payload) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id::text`, p.UserID, c.Pattern, date+":"+c.OutletID, marshal(c)).Scan(&id)
		if err == nil {
			err = auditTx(ctx, tx, p.UserID, "HABIT_SUGGESTED", id, c)
		} else if err == pgx.ErrNoRows {
			err = nil
		}
		if err != nil {
			tx.Rollback(ctx)
			return out, err
		}
		if err = tx.Commit(ctx); err != nil {
			return out, err
		}
		var response *string
		err = s.Pool.QueryRow(ctx, `SELECT id::text,response FROM shared.habit_suggestions WHERE owner_id=$1 AND pattern_key=$2 AND occurrence_key=$3`, p.UserID, c.Pattern, date+":"+c.OutletID).Scan(&c.ID, &response)
		if err != nil {
			return out, err
		}
		if response == nil {
			out = append(out, c)
		}
		if len(out) == 3 {
			break
		}
	}
	return out, nil
}

func (s Service) respond(ctx context.Context, p *authorization.Profile, id, response string) (Habit, error) {
	var h Habit
	if response != "yes" && response != "no" && response != "never" {
		return h, errors.New("choose yes, no or never")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return h, err
	}
	defer tx.Rollback(ctx)
	// Serialize feedback across suggestions belonging to the same person.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.UserID); err != nil {
		return h, err
	}
	var raw []byte
	var existing *string
	var created time.Time
	if err = tx.QueryRow(ctx, `SELECT payload,response,created_at FROM shared.habit_suggestions WHERE id::text=$1 AND owner_id=$2 FOR UPDATE`, id, p.UserID).Scan(&raw, &existing, &created); err != nil {
		return h, errors.New("suggestion not found")
	}
	if err = json.Unmarshal(raw, &h); err != nil {
		return h, err
	}
	h.ID = id
	if existing != nil {
		return h, errors.New("suggestion already answered")
	}
	if s.now().Sub(created) > 24*time.Hour {
		return h, errors.New("suggestion expired")
	}
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT enabled FROM shared.automation_preferences WHERE owner_id=$1),true)`, p.UserID).Scan(&enabled); err != nil {
		return h, err
	}
	if !enabled {
		return h, errors.New("habit helper is disabled")
	}
	var suppressed bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT suppressed OR no_count>=2 FROM shared.habit_feedback WHERE owner_id=$1 AND pattern_key=$2),false)`, p.UserID, h.Pattern).Scan(&suppressed); err != nil {
		return h, err
	}
	if suppressed {
		return h, errors.New("pattern is suppressed")
	}
	if response == "yes" {
		if h.Action == "prefill_order" {
			if h.Prefill == nil || !h.Prefill.Valid() || !owns(p, h.Prefill.OutletID) {
				return h, errors.New("template is no longer authorized")
			}
		} else {
			if role(p) != "DISPATCHER" {
				return h, errors.New("dispatcher required")
			}
			snap, e := s.snapshot(ctx, p, s.now())
			if e != nil {
				return h, e
			}
			valid := false
			for _, d := range snap.Items {
				if d.OutletID == h.OutletID && d.Count >= 2 {
					valid = true
				}
			}
			if !valid {
				return h, errors.New("outlet no longer needs repeat-deferral review")
			}
			if err = priorityTx(ctx, tx, p.UserID, h.OutletID, s.now()); err != nil {
				return h, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO shared.habit_feedback(owner_id,pattern_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.UserID, h.Pattern); err != nil {
		return h, err
	}
	if err = tx.QueryRow(ctx, `UPDATE shared.habit_feedback SET yes_count=yes_count+CASE WHEN $3='yes' THEN 1 ELSE 0 END,no_count=no_count+CASE WHEN $3='no' THEN 1 ELSE 0 END,suppressed=suppressed OR $3='never' OR ($3='no' AND no_count>=1),last_response=$4 WHERE owner_id=$1 AND pattern_key=$2 RETURNING yes_count,no_count`, p.UserID, h.Pattern, response, s.now()).Scan(&h.YesCount, &h.NoCount); err != nil {
		return h, err
	}
	if _, err = tx.Exec(ctx, `UPDATE shared.habit_suggestions SET response=$2 WHERE id::text=$1`, id, response); err != nil {
		return h, err
	}
	if err = auditTx(ctx, tx, p.UserID, "HABIT_ANSWERED", id, map[string]any{"response": response, "pattern": h.Pattern}); err != nil {
		return h, err
	}
	return h, tx.Commit(ctx)
}
func priorityTx(ctx context.Context, tx pgx.Tx, owner, outlet string, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO shared.priority_reviews(owner_id,outlet_id,expires_at) VALUES($1,$2,$3) ON CONFLICT(owner_id,outlet_id) DO UPDATE SET expires_at=excluded.expires_at`, owner, outlet, now.Add(7*24*time.Hour))
	return err
}

// RunDue executes each due workflow under a row lock. Run, inbox, audit and next
// due date commit together; retries cannot duplicate a committed notification.
func (s Service) RunDue(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, `SELECT id::text FROM shared.automations WHERE status='active' AND next_run_at<=$1 ORDER BY next_run_at LIMIT 100`, s.now())
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var first error
	for _, id := range ids {
		if err = s.runOne(ctx, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}
func (s Service) runOne(ctx context.Context, id string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owner, subject string
	var raw []byte
	var scheduled time.Time
	err = tx.QueryRow(ctx, `SELECT a.owner_id,u.identity_subject,a.definition,a.next_run_at FROM shared.automations a JOIN shared.users u ON u.id=a.owner_id WHERE a.id::text=$1 AND a.status='active' AND a.next_run_at<=$2 FOR UPDATE OF a SKIP LOCKED`, id, s.now()).Scan(&owner, &subject, &raw, &scheduled)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	p, err := s.Profiles.Resolve(ctx, subject)
	if err != nil {
		return err
	}
	var d automation.Definition
	if err = json.Unmarshal(raw, &d); err != nil {
		return err
	}
	if !allowed(p) || p.UserID != owner || validate(d, p) != nil {
		if _, err = tx.Exec(ctx, `UPDATE shared.automations SET status='paused' WHERE id::text=$1`, id); err != nil {
			return err
		}
		if err = auditTx(ctx, tx, owner, "AUTOMATION_PAUSED_PERMISSION", id, nil); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	// Check current data on catch-up rather than delivering a stale operational list.
	result, err := s.evaluate(ctx, p, d, s.now())
	if err != nil {
		return err
	}
	status := "completed"
	if len(result.Items) == 0 && result.Prefill == nil {
		status = "no_matches"
	}
	tag, err := tx.Exec(ctx, `INSERT INTO shared.automation_runs(automation_id,owner_id,scheduled_at,status,result) VALUES($1::uuid,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, id, owner, scheduled, status, marshal(result))
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 && status == "completed" {
		if _, err = tx.Exec(ctx, `INSERT INTO shared.automation_inbox(owner_id,title,payload) VALUES($1,$2,$3)`, owner, d.Name, marshal(result)); err != nil {
			return err
		}
		if d.Action == "priority_review" {
			for _, item := range result.Items {
				if err = priorityTx(ctx, tx, owner, item.OutletID, s.now()); err != nil {
					return err
				}
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE shared.automations SET next_run_at=$2 WHERE id::text=$1`, id, d.Next(s.now())); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, owner, "AUTOMATION_RUN", id, map[string]any{"status": status, "scheduledAt": scheduled, "result": result}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
