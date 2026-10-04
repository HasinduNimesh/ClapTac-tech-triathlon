package automations

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Service Service
	Authn   auth.Authenticator
}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/shared/automations", func(r chi.Router) {
		r.Use(h.auth)
		r.Get("/", h.list)
		r.Post("/", h.activate)
		r.Post("/preview", h.preview)
		r.Post("/{id}/status", h.status)
		r.Get("/habits", h.habits)
		r.Post("/habits/{id}/respond", h.respond)
		r.Put("/preferences", h.preferences)
		r.Get("/inbox", h.inbox)
		r.Get("/runs", h.runs)
		r.Get("/priorities", h.priorities)
		r.Post("/priorities", h.priority)
	})
}
func (h Handler) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := h.Authn.Authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		p, err := h.Service.Profiles.Resolve(auth.WithBearer(r.Context(), r.Header.Get("Authorization")), principal.Subject)
		if err != nil || !allowed(p) {
			http.Error(w, "store manager or dispatcher profile required", 403)
			return
		}
		next.ServeHTTP(w, r.WithContext(authorization.WithProfile(r.Context(), p)))
	})
}
func profile(r *http.Request) *authorization.Profile {
	p, _ := authorization.ProfileFrom(r.Context())
	return p
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		http.Error(w, "invalid request", 400)
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "one JSON object required", 400)
		return false
	}
	return true
}
func fail(w http.ResponseWriter, err error) {
	http.Error(w, "Automation data is unavailable. Please retry.", 503)
}
func (h Handler) habits(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.habits(r.Context(), profile(r))
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": items})
}
func (h Handler) respond(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Response string `json:"response"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := h.Service.respond(r.Context(), profile(r), chi.URLParam(r, "id"), body.Response)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	httpx.WriteJSON(w, 200, out)
}
func (h Handler) preferences(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	p := profile(r)
	tx, err := h.Service.Pool.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO shared.automation_preferences(owner_id,enabled) VALUES($1,$2) ON CONFLICT(owner_id) DO UPDATE SET enabled=excluded.enabled`, p.UserID, body.Enabled)
	if err == nil {
		err = auditTx(r.Context(), tx, p.UserID, "HABIT_PREFERENCE_CHANGED", p.UserID, body)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, body)
}
func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	p := profile(r)
	rows, err := h.Service.Pool.Query(r.Context(), `SELECT id::text,definition,status,next_run_at FROM shared.automations WHERE owner_id=$1 AND status<>'deleted' ORDER BY created_at DESC`, p.UserID)
	if err != nil {
		fail(w, err)
		return
	}
	items := []any{}
	for rows.Next() {
		var id, status string
		var raw json.RawMessage
		var next time.Time
		if err = rows.Scan(&id, &raw, &status, &next); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		var d automation.Definition
		if json.Unmarshal(raw, &d) != nil || validate(d, p) != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "definition": raw, "status": status, "nextRunAt": next})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(w, err)
		return
	}
	var enabled bool
	if err = h.Service.Pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT enabled FROM shared.automation_preferences WHERE owner_id=$1),true)`, p.UserID).Scan(&enabled); err != nil {
		fail(w, err)
		return
	}
	var demo bool
	if err = h.Service.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM audit.events WHERE actor_id=$1 AND source='automation-demo')`, p.UserID).Scan(&demo); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": items, "habitEnabled": enabled, "demoHistory": demo})
}
func (h Handler) preview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Definition automation.Definition `json:"definition"`
		At         string                `json:"at"`
	}
	if !decode(w, r, &body) {
		return
	}
	p := profile(r)
	if err := validate(body.Definition, p); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	at := h.Service.now()
	if body.At == "previous" {
		at = body.Definition.Previous(at)
	} else if body.At != "now" {
		http.Error(w, "choose now or previous", 400)
		return
	}
	result, err := h.Service.evaluate(r.Context(), p, body.Definition, at)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	var id string
	err = h.Service.Pool.QueryRow(r.Context(), `INSERT INTO shared.automation_previews(owner_id,definition,result) VALUES($1,$2,$3) RETURNING id::text`, p.UserID, marshal(body.Definition), marshal(result)).Scan(&id)
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"id": id, "result": result})
}
func (h Handler) activate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PreviewID string `json:"previewId"`
	}
	if !decode(w, r, &body) {
		return
	}
	p := profile(r)
	ctx := r.Context()
	tx, err := h.Service.Pool.Begin(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, p.UserID)
	if err != nil {
		fail(w, err)
		return
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT definition FROM shared.automation_previews WHERE id::text=$1 AND owner_id=$2 AND created_at>now()-interval '15 minutes'`, body.PreviewID, p.UserID).Scan(&raw)
	if err != nil {
		http.Error(w, "Preview missing or expired; test the workflow again.", 409)
		return
	}
	var d automation.Definition
	if json.Unmarshal(raw, &d) != nil || validate(d, p) != nil {
		http.Error(w, "workflow is no longer authorized", 403)
		return
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM shared.automations WHERE owner_id=$1 AND status<>'deleted'`, p.UserID).Scan(&count); err != nil {
		fail(w, err)
		return
	}
	if count >= 100 {
		http.Error(w, "automation limit reached", 409)
		return
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO shared.automations(owner_id,definition,status,next_run_at,activation_key) VALUES($1,$2,'active',$3,$4::uuid) ON CONFLICT(owner_id,activation_key) DO UPDATE SET activation_key=excluded.activation_key RETURNING id::text`, p.UserID, raw, d.Next(h.Service.now()), body.PreviewID).Scan(&id)
	if err == nil {
		err = auditTx(ctx, tx, p.UserID, "AUTOMATION_ACTIVATED", id, d)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"id": id})
}
func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Status != "active" && body.Status != "paused" && body.Status != "deleted" {
		http.Error(w, "invalid status", 400)
		return
	}
	p := profile(r)
	ctx := r.Context()
	tx, err := h.Service.Pool.Begin(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var raw []byte
	id := chi.URLParam(r, "id")
	err = tx.QueryRow(ctx, `SELECT definition FROM shared.automations WHERE id::text=$1 AND owner_id=$2 AND status<>'deleted' FOR UPDATE`, id, p.UserID).Scan(&raw)
	if err != nil {
		http.Error(w, "automation not found", 404)
		return
	}
	var d automation.Definition
	if json.Unmarshal(raw, &d) != nil {
		fail(w, errors.New("invalid definition"))
		return
	}
	if body.Status == "active" {
		if err = validate(d, p); err != nil {
			http.Error(w, err.Error(), 403)
			return
		}
	}
	_, err = tx.Exec(ctx, `UPDATE shared.automations SET status=$2,next_run_at=$3 WHERE id::text=$1`, id, body.Status, d.Next(h.Service.now()))
	if err == nil {
		err = auditTx(ctx, tx, p.UserID, "AUTOMATION_STATUS_CHANGED", id, body)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, body)
}
func (h Handler) inbox(w http.ResponseWriter, r *http.Request) {
	h.records(w, r, `SELECT id::text,title,payload,created_at FROM shared.automation_inbox WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 50`)
}
func (h Handler) runs(w http.ResponseWriter, r *http.Request) {
	h.records(w, r, `SELECT id::text,status,result,created_at FROM shared.automation_runs WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 50`)
}
func (h Handler) records(w http.ResponseWriter, r *http.Request, query string) {
	p := profile(r)
	rows, err := h.Service.Pool.Query(r.Context(), query, p.UserID)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, title string
		var raw []byte
		var at time.Time
		if err = rows.Scan(&id, &title, &raw, &at); err != nil {
			fail(w, err)
			return
		}
		var result Result
		if err = json.Unmarshal(raw, &result); err != nil {
			fail(w, err)
			return
		}
		// Reapply current scope to persisted results after outlet membership changes.
		filtered := []automation.Deferred{}
		for _, d := range result.Items {
			if owns(p, d.OutletID) {
				filtered = append(filtered, d)
			}
		}
		result.Items = filtered
		if result.Prefill != nil && !owns(p, result.Prefill.OutletID) {
			continue
		}
		result.Message = "Workflow result; visibility follows your current access."
		items = append(items, map[string]any{"id": id, "title": title, "result": result, "createdAt": at})
	}
	if err = rows.Err(); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": items})
}
func (h Handler) priorities(w http.ResponseWriter, r *http.Request) {
	p := profile(r)
	if role(p) != "DISPATCHER" {
		http.Error(w, "dispatcher required", 403)
		return
	}
	snap, err := h.Service.snapshot(r.Context(), p, h.Service.now())
	if err != nil {
		fail(w, err)
		return
	}
	rows, err := h.Service.Pool.Query(r.Context(), `SELECT outlet_id FROM shared.priority_reviews WHERE owner_id=$1 AND expires_at>$2`, p.UserID, h.Service.now())
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	marked := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			fail(w, err)
			return
		}
		marked = append(marked, id)
	}
	if err = rows.Err(); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": snap.Items, "marked": marked})
}
func (h Handler) priority(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutletID string `json:"outletId"`
	}
	if !decode(w, r, &body) {
		return
	}
	p := profile(r)
	if role(p) != "DISPATCHER" {
		http.Error(w, "dispatcher required", 403)
		return
	}
	snap, err := h.Service.snapshot(r.Context(), p, h.Service.now())
	if err != nil {
		fail(w, err)
		return
	}
	valid := false
	for _, d := range snap.Items {
		if d.OutletID == body.OutletID && d.Count >= 2 {
			valid = true
		}
	}
	if !valid {
		http.Error(w, "outlet needs at least two recorded deferral dates", 409)
		return
	}
	tx, err := h.Service.Pool.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	err = priorityTx(r.Context(), tx, p.UserID, body.OutletID, h.Service.now())
	if err == nil {
		err = auditTx(r.Context(), tx, p.UserID, "PRIORITY_REVIEW_MARKED", body.OutletID, body)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, 200, body)
}
