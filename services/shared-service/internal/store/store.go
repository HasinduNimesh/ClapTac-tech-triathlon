package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
)

type AuditFilter struct {
	Query, Action, ResourceType, ResourceID, ActorID string
	From, To                                         time.Time
	Limit, Offset                                    int
}

func (s Store) SearchAudit(ctx context.Context, f AuditFilter) ([]audit.Event, int, error) {
	args := []any{f.Query, f.Action, f.ResourceType, f.ResourceID, f.ActorID, nullableTime(f.From), nullableTime(f.To)}
	where := `WHERE ($1 = '' OR concat_ws(' ', actor_id, action, resource_type, resource_id, source, reason) ILIKE '%' || $1 || '%')
		AND ($2 = '' OR action = $2) AND ($3 = '' OR resource_type = $3) AND ($4 = '' OR resource_id = $4) AND ($5 = '' OR actor_id = $5)
		AND ($6::timestamptz IS NULL OR timestamp >= $6) AND ($7::timestamptz IS NULL OR timestamp <= $7)`
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit.events `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.Pool.Query(ctx, `SELECT event_id, COALESCE(correlation_id,''), COALESCE(actor_id,''), COALESCE(actor_type,''), action,
		COALESCE(resource_type,''), COALESCE(resource_id,''), previous_state, new_state, COALESCE(reason,''), timestamp, COALESCE(source,'')
		FROM audit.events `+where+` ORDER BY timestamp DESC, event_id DESC LIMIT $8 OFFSET $9`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []audit.Event{}
	for rows.Next() {
		var ev audit.Event
		var prev, next []byte
		if err := rows.Scan(&ev.EventID, &ev.CorrelationID, &ev.ActorID, &ev.ActorType, &ev.Action, &ev.ResourceType, &ev.ResourceID, &prev, &next, &ev.Reason, &ev.Timestamp, &ev.Source); err != nil {
			return nil, 0, err
		}
		if len(prev) > 0 {
			_ = json.Unmarshal(prev, &ev.PreviousState)
		}
		if len(next) > 0 {
			_ = json.Unmarshal(next, &ev.NewState)
		}
		items = append(items, ev)
	}
	return items, total, rows.Err()
}

func (s Store) AuditKPIs(ctx context.Context, from, to time.Time) (map[string]any, error) {
	type pair struct {
		Action string `json:"action"`
		Count  int    `json:"count"`
	}
	rows, err := s.Pool.Query(ctx, `SELECT action, count(*) FROM audit.events WHERE timestamp >= $1 AND timestamp <= $2 GROUP BY action ORDER BY action`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	byAction := []pair{}
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.Action, &p.Count); err != nil {
			return nil, err
		}
		counts[p.Action] = p.Count
		byAction = append(byAction, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var total int
	for _, n := range counts {
		total += n
	}
	return map[string]any{"from": from, "to": to, "totalEvents": total, "byAction": byAction,
		"plansGenerated": counts["PLAN_GENERATED"], "ordersDeferred": counts["ORDER_DEFERRED"],
		"tripsStarted": counts["DELIVERY_RUN_STARTED"], "tripsCompleted": counts["DELIVERY_RUN_COMPLETED"],
		"deliveryOutcomes": counts["DELIVERY_OUTCOME_RECORDED"], "syncConflicts": counts["DELIVERY_SYNC_CONFLICT"]}, nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

type Store struct {
	Pool *pgxpool.Pool
}

func (s Store) Resolve(ctx context.Context, subject string) (*authorization.Profile, error) {
	p, err := s.ProfileBySubject(ctx, subject)
	if err != nil {
		return &authorization.Profile{Subject: subject}, nil
	}
	return p, nil
}

func (s Store) ProfileBySubject(ctx context.Context, subject string) (*authorization.Profile, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.identity_subject, u.display_name, u.role, COALESCE(p.outlet_id, ''), COALESCE(l.depot, dp.depot, ''), COALESCE(d.vehicle_id, '')
		FROM users u
		LEFT JOIN store_manager_profiles p ON p.user_id = u.id
		LEFT JOIN loader_profiles l ON l.user_id = u.id
		LEFT JOIN dispatcher_profiles dp ON dp.user_id = u.id
		LEFT JOIN driver_profiles d ON d.user_id = u.id
		WHERE u.identity_subject = $1
	`, subject)
	var p authorization.Profile
	var role, outlet, depot, vehicle string
	if err := row.Scan(&p.UserID, &p.Subject, &p.DisplayName, &role, &outlet, &depot, &vehicle); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}
	p.Roles = []string{role}
	if outlet != "" {
		p.OutletIDs = []string{outlet}
	}
	p.Depot = depot
	p.VehicleID = vehicle
	return &p, nil
}

// SetDisplayName only changes the user represented by the authenticated token subject.
func (s Store) SetDisplayName(ctx context.Context, subject, name string) error {
	var id string
	err := s.Pool.QueryRow(ctx, `UPDATE shared.users SET display_name=$2, updated_at=now()
		WHERE identity_subject=$1 RETURNING id`, subject, name).Scan(&id)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("not found")
	}
	return err
}

type Outlet struct {
	ID                            string     `json:"id"`
	Brand                         string     `json:"brand"`
	Name                          string     `json:"name"`
	District                      string     `json:"district"`
	Depot                         string     `json:"depot"`
	DockType                      string     `json:"dockType"`
	ParkingConstraint             string     `json:"parkingConstraint"`
	MallWindow                    bool       `json:"mallWindow"`
	WindowOpenTime                string     `json:"windowOpenTime,omitempty"`
	WindowCloseTime               string     `json:"windowCloseTime,omitempty"`
	AccessInstructions            string     `json:"accessInstructions"`
	AccessInstructionsUpdatedBy   string     `json:"accessInstructionsUpdatedBy,omitempty"`
	AccessInstructionsUpdatedAt   *time.Time `json:"accessInstructionsUpdatedAt,omitempty"`
	AccessInstructionsConfirmedBy string     `json:"accessInstructionsConfirmedBy,omitempty"`
	AccessInstructionsConfirmedAt *time.Time `json:"accessInstructionsConfirmedAt,omitempty"`
	ChilledTemperatureMinC        *float64   `json:"chilledTemperatureMinC,omitempty"`
	ChilledTemperatureMaxC        *float64   `json:"chilledTemperatureMaxC,omitempty"`
	Version                       int        `json:"version"`
	// Latitude/Longitude are approximate (district centre + offset) unless set exactly.
	Latitude            *float64 `json:"latitude,omitempty"`
	Longitude           *float64 `json:"longitude,omitempty"`
	LocationApproximate bool     `json:"locationApproximate"`
}

// NotificationPreferences keeps an outlet's SMS number private from ordinary
// outlet reads and records affirmative consent separately from alert choices.
type NotificationPreferences struct {
	OutletID           string     `json:"outletId"`
	PhoneE164          string     `json:"phoneE164"`
	ConsentEnabled     bool       `json:"consentEnabled"`
	DeferralsEnabled   bool       `json:"deferralsEnabled"`
	MajorDelaysEnabled bool       `json:"majorDelaysEnabled"`
	Locale             string     `json:"locale"`
	ConsentedAt        *time.Time `json:"consentedAt,omitempty"`
	Version            int        `json:"version"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type NotificationEvent struct {
	EventKey     string `json:"eventKey"`
	OutletID     string `json:"outletId"`
	Type         string `json:"type"`
	OrderRef     string `json:"orderRef"`
	Reason       string `json:"reason,omitempty"`
	DelayMinutes int    `json:"delayMinutes,omitempty"`

	NextRun      string `json:"nextRun,omitempty"`
	OldArrivalAt string `json:"oldArrivalAt,omitempty"`
	Goods        string `json:"goods,omitempty"`
	Units        int    `json:"units,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	FollowupDate string `json:"followupDate,omitempty"`
	NewArrivalAt string `json:"newArrivalAt,omitempty"`
}

type EnqueueResult struct {
	Status string `json:"status"`
	ID     int64  `json:"id,omitempty"`
}

func nextRunText(date string) string {
	if date == "" {
		return "to be confirmed"
	}
	return date
}

// EnqueueNotification records the notice for the outlet and decides, separately, whether it may also be sent
// by SMS. The in-app message is always stored: an outlet with no preference row, with consent withdrawn or with
// this alert switched off still reads what will arrive on its Notifications page. Only the outlet's SMS consent
// and alert choices decide the status: PENDING (eligible for the SMS worker) or IN_APP_ONLY (never claimed).
//
// Merge note: branch #35 made this return "suppressed" with no row when there is no preference row; that quick
// fix is superseded by this behaviour (the status for such an outlet is "in_app_only", with a row).
func (s Store) EnqueueNotification(ctx context.Context, e NotificationEvent) (EnqueueResult, error) {
	phone, locale, smsEligible := "", "en", false
	err := s.Pool.QueryRow(ctx, `SELECT phone_e164,locale,consent_enabled AND CASE $2 WHEN 'DEFERRAL' THEN deferrals_enabled WHEN 'LOAD_SHORTFALL' THEN deferrals_enabled WHEN 'MAJOR_DELAY' THEN major_delays_enabled WHEN 'ARRIVAL_CHANGE' THEN major_delays_enabled WHEN 'DELIVERY_REJECTED' THEN deferrals_enabled ELSE false END FROM shared.outlet_notification_preferences WHERE outlet_id=$1`, e.OutletID, e.Type).Scan(&phone, &locale, &smsEligible)
	if err == pgx.ErrNoRows {
		// No SMS contact on file: the in-app notice is still stored, in English, with no phone number.
		var known bool
		if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shared.outlets WHERE id=$1)`, e.OutletID).Scan(&known); err != nil {
			return EnqueueResult{}, err
		}
		if !known {
			return EnqueueResult{}, fmt.Errorf("outlet not found")
		}
		phone, locale, smsEligible = "", "en", false
	} else if err != nil {

		return EnqueueResult{}, err
	}
	var body string
	if e.Type == "LOAD_SHORTFALL" {
		body = ShortfallBody(locale, e)
	} else if e.Type == "DELIVERY_REJECTED" {
		action := "re-attempt requested for the next run"
		if e.Resolution == "REQUEST_DEFERRAL" {
			action = "dispatcher deferral requested"
		}
		body = fmt.Sprintf("Waypoint: order %s rejected; %d unit(s) of %s returned (%s). %s. Follow-up run: %s.",
			e.OrderRef, e.Units, e.Goods, e.Reason, action, e.FollowupDate)
	} else if e.Type == "ARRIVAL_CHANGE" {
		oldETA, oldErr := time.Parse(time.RFC3339Nano, e.OldArrivalAt)
		newETA, newErr := time.Parse(time.RFC3339Nano, e.NewArrivalAt)
		if oldErr != nil || newErr != nil {
			return EnqueueResult{}, fmt.Errorf("invalid arrival notification")
		}
		colombo := time.FixedZone("Sri Lanka", 5*60*60+30*60)
		oldText := oldETA.In(colombo).Format("02 Jan 15:04")
		newText := newETA.In(colombo).Format("02 Jan 15:04")
		switch locale {
		case "si":
			body = fmt.Sprintf("Waypoint: %s ඇණවුමේ පෙර පැමිණීම %s; නව පැමිණීම %s.", e.OrderRef, oldText, newText)
		case "ta":
			body = fmt.Sprintf("Waypoint: %s ஆர்டரின் முந்தைய வருகை %s; புதிய வருகை %s.", e.OrderRef, oldText, newText)
		default:
			body = fmt.Sprintf("Waypoint: order %s arrival changed from %s to %s (Sri Lanka time).", e.OrderRef, oldText, newText)
		}
	} else {
		reason := notificationReason(locale, e.Reason)
		switch locale {
		case "si":
			if e.Type == "DEFERRAL" {
				body = fmt.Sprintf("Waypoint: ඇණවුම %s කල් දමා ඇත. හේතුව: %s. ඊළඟ බෙදාහැරීම: %s. වැඩිදුර විස්තර සඳහා Dispatcher අමතන්න.", e.OrderRef, reason, nextRunText(e.NextRun))
			} else {
				body = fmt.Sprintf("Waypoint: ඇණවුම %s පැමිණීම විනාඩි %dකින් ප්‍රමාද වේ.", e.OrderRef, e.DelayMinutes)
			}
		case "ta":
			if e.Type == "DEFERRAL" {
				body = fmt.Sprintf("Waypoint: ஆர்டர் %s ஒத்திவைக்கப்பட்டது. காரணம்: %s. அடுத்த விநியோகம்: %s. மேலும் விவரங்களுக்கு Dispatcher-ஐ தொடர்புகொள்ளவும்.", e.OrderRef, reason, nextRunText(e.NextRun))
			} else {
				body = fmt.Sprintf("Waypoint: ஆர்டர் %s வருகை %d நிமிடங்கள் தாமதமாகும்.", e.OrderRef, e.DelayMinutes)
			}
		default:
			if e.Type == "DEFERRAL" {
				body = fmt.Sprintf("Waypoint: order %s was deferred. Reason: %s. Next delivery run: %s. Contact the dispatcher if you need more detail.", e.OrderRef, reason, nextRunText(e.NextRun))
			} else {
				body = fmt.Sprintf("Waypoint: order %s is expected to arrive %d minutes later than planned.", e.OrderRef, e.DelayMinutes)
			}

		}
	}
	status, errorCode, result := "PENDING", "", "enqueued"
	if !smsEligible {
		// The SMS number is not kept on a row that will never be sent.
		status, errorCode, result, phone = "IN_APP_ONLY", "CONSENT_OR_ALERT_DISABLED", "in_app_only", ""
	}
	var id int64
	err = s.Pool.QueryRow(ctx, `INSERT INTO shared.notification_outbox(event_key,outlet_id,event_type,phone_e164,body,status,error_code) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')) ON CONFLICT(event_key) DO NOTHING RETURNING id`, e.EventKey, e.OutletID, e.Type, phone, body, status, errorCode).Scan(&id)
	if err == pgx.ErrNoRows {
		err = s.Pool.QueryRow(ctx, `SELECT id FROM shared.notification_outbox WHERE event_key=$1`, e.EventKey).Scan(&id)
		return EnqueueResult{Status: "duplicate", ID: id}, err
	}
	if err != nil {
		return EnqueueResult{}, err
	}
	return EnqueueResult{Status: result, ID: id}, nil
}

func notificationReason(locale, code string) string {
	type phrase struct{ en, si, ta string }
	phrases := map[string]phrase{
		"NO_ELIGIBLE_VEHICLE":        {"vehicle availability", "වාහන ලබාගැනීමේ සීමා", "வாகனக் கிடைக்கும் வரம்புகள்"},
		"WEIGHT_CAPACITY_EXCEEDED":   {"vehicle capacity", "වාහන ධාරිතා සීමා", "வாகன கொள்ளளவு வரம்புகள்"},
		"VOLUME_CAPACITY_EXCEEDED":   {"vehicle capacity", "වාහන ධාරිතා සීමා", "வாகன கொள்ளளவு வரம்புகள்"},
		"REFRIGERATION_REQUIRED":     {"cooling requirements", "සිසිලන අවශ්‍යතා", "குளிரூட்டல் தேவைகள்"},
		"VAN_REQUIRED":               {"outlet access", "වෙළඳසැල් ප්‍රවේශ සීමා", "கடை அணுகல் வரம்புகள்"},
		"DEPOT_MISMATCH":             {"depot constraints", "ඩිපෝ සීමා", "கிடங்கு வரம்புகள்"},
		"DELIVERY_WINDOW_CONFLICT":   {"delivery window constraints", "බෙදාහැරීමේ කාල සීමා", "விநியோக நேர வரம்புகள்"},
		"FUEL_QUOTA_EXCEEDED":        {"fuel limits", "ඉන්ධන සීමා", "எரிபொருள் வரம்புகள்"},
		"TRIP_LIMIT_REACHED":         {"trip limits", "ගමන් සීමා", "பயண வரம்புகள்"},
		"VEHICLE_UNAVAILABLE":        {"vehicle availability", "වාහන ලබාගැනීමේ සීමා", "வாகனக் கிடைக்கும் வரம்புகள்"},
		"MANUAL_DISPATCHER_DEFERRAL": {"dispatch planning decision", "යැවීම් සැලසුම් තීරණය", "அனுப்பீட்டுத் திட்டமிடல் முடிவு"},
	}
	p, ok := phrases[code]
	if !ok {
		p = phrases["MANUAL_DISPATCHER_DEFERRAL"]
	}
	if locale == "si" {
		return p.si
	}
	if locale == "ta" {
		return p.ta
	}
	return p.en
}

type PendingNotification struct {
	ID        int64
	Phone     string
	Body      string
	EventType string
}

type NotificationOutboxStats struct {
	Pending               int64
	OldestAgeSeconds      float64
	AwaitingStatus        int64
	AwaitingOldestSeconds float64
}

func (s Store) NotificationOutboxStats(ctx context.Context) (NotificationOutboxStats, error) {
	var stats NotificationOutboxStats
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status IN ('PENDING','SENDING')),COALESCE(extract(epoch from(now()-min(created_at) FILTER(WHERE status='PENDING'))),0),count(*) FILTER(WHERE status IN ('QUEUED','SENT')),COALESCE(extract(epoch from(now()-min(updated_at) FILTER(WHERE status IN ('QUEUED','SENT')))),0) FROM shared.notification_outbox`).Scan(&stats.Pending, &stats.OldestAgeSeconds, &stats.AwaitingStatus, &stats.AwaitingOldestSeconds)
	return stats, err
}

// ClaimNotification atomically marks a row before the external request. Rows
// in SENDING are never retried automatically because provider acceptance may
// be ambiguous after a timeout or process crash.
func (s Store) ClaimNotification(ctx context.Context) (*PendingNotification, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var n PendingNotification
	var consent, enabled bool

	err = tx.QueryRow(ctx, `SELECT n.id,COALESCE(p.phone_e164,n.phone_e164),n.body,n.event_type,COALESCE(p.consent_enabled,false),COALESCE(CASE n.event_type WHEN 'DEFERRAL' THEN p.deferrals_enabled WHEN 'LOAD_SHORTFALL' THEN p.deferrals_enabled WHEN 'MAJOR_DELAY' THEN p.major_delays_enabled WHEN 'ARRIVAL_CHANGE' THEN p.major_delays_enabled WHEN 'DELIVERY_REJECTED' THEN p.deferrals_enabled END,false)

	FROM shared.notification_outbox n LEFT JOIN shared.outlet_notification_preferences p USING(outlet_id)
	WHERE n.status='PENDING' ORDER BY n.created_at,n.id LIMIT 1 FOR UPDATE OF n SKIP LOCKED`).Scan(&n.ID, &n.Phone, &n.Body, &n.EventType, &consent, &enabled)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !consent || !enabled {
		if _, err = tx.Exec(ctx, `UPDATE shared.notification_outbox SET status='IN_APP_ONLY',phone_e164='',error_code='CONSENT_OR_ALERT_DISABLED',updated_at=now(),payload_expires_at=now()+interval '7 days' WHERE id=$1`, n.ID); err != nil {
			return nil, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE shared.notification_outbox SET phone_e164=$2,status='SENDING',updated_at=now() WHERE id=$1`, n.ID, n.Phone); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &n, nil
}

func (s Store) FinishNotification(ctx context.Context, id int64, status, messageID, errorCode string) error {
	if status != "QUEUED" && status != "FAILED" && status != "UNKNOWN" {
		return fmt.Errorf("invalid notification terminal status")
	}
	_, err := s.Pool.Exec(ctx, `UPDATE shared.notification_outbox SET status=$2,provider_message_id=NULLIF($3,''),error_code=NULLIF($4,''),updated_at=now(),payload_expires_at=now()+interval '7 days' WHERE id=$1 AND status='SENDING'`, id, status, messageID, errorCode)
	return err
}

func (s Store) RecoverSendingNotifications(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE shared.notification_outbox SET status='UNKNOWN',error_code='STALE_SEND_OUTCOME_UNKNOWN',updated_at=now(),payload_expires_at=now()+interval '7 days' WHERE status='SENDING' AND updated_at < now()-interval '2 minutes'`)
	return err
}

func (s Store) PurgeExpiredNotificationPayloads(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE shared.notification_outbox SET phone_e164='',body='' WHERE status IN ('DELIVERED','FAILED','UNKNOWN','SUPPRESSED','IN_APP_ONLY') AND payload_expires_at<now() AND (phone_e164<>'' OR body<>'')`)
	return err
}

// gatewayMessageID is what the integration service stores for a text sent through our own cellular gateway:
// "cg:" and the gateway's request id.
var gatewayMessageID = regexp.MustCompile(`^cg:[A-Za-z0-9_-]{4,72}$`)

// validProviderMessageID accepts a Twilio message SID (34 characters, "SM...") or a cellular gateway id ("cg:...").
func validProviderMessageID(id string) bool {
	return (len(id) == 34 && strings.HasPrefix(id, "SM")) || gatewayMessageID.MatchString(id)
}

func (s Store) UpdateNotificationStatus(ctx context.Context, messageSID, providerStatus, errorCode string) error {
	var status string
	switch strings.ToLower(providerStatus) {
	case "queued", "accepted", "scheduled", "sending":
		status = "QUEUED"
	case "sent":
		status = "SENT"
	case "delivered":
		status = "DELIVERED"
	case "failed", "undelivered", "canceled":
		status = "FAILED"
	default:
		return fmt.Errorf("unsupported provider status")
	}
	if !validProviderMessageID(messageSID) || len(errorCode) > 40 {
		return fmt.Errorf("invalid provider status fields")
	}
	command, err := s.Pool.Exec(ctx, `UPDATE shared.notification_outbox SET status=$2,error_code=NULLIF($3,''),updated_at=now(),payload_expires_at=now()+interval '7 days' WHERE provider_message_id=$1 AND ((status='QUEUED' AND $2 IN ('QUEUED','SENT','DELIVERED','FAILED')) OR (status='SENT' AND $2 IN ('SENT','DELIVERED','FAILED')))`, messageSID, status, errorCode)
	if err != nil {
		return err
	}
	if command.RowsAffected() > 0 {
		return nil
	}
	var current string
	err = s.Pool.QueryRow(ctx, `SELECT status FROM shared.notification_outbox WHERE provider_message_id=$1`, messageSID).Scan(&current)
	if err != nil {
		return err
	}
	if current == status || current == "DELIVERED" || current == "FAILED" {
		return nil
	}
	return fmt.Errorf("notification status transition conflict")
}

// OutletNotification is one message the system queued for an outlet, as the outlet itself may read
// it back. It never carries the phone number.
type OutletNotification struct {
	ID        int64     `json:"id"`
	EventType string    `json:"eventType"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

// OutletNotifications returns the newest messages stored for an outlet, whether they are waiting for SMS or
// kept in-app only (IN_APP_ONLY, never sent). Rows whose text has been purged have nothing to show and are left out.
func (s Store) OutletNotifications(ctx context.Context, outletID string, limit int) ([]OutletNotification, error) {
	var known bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shared.outlets WHERE id=$1)`, outletID).Scan(&known); err != nil {
		return nil, err
	}
	if !known {
		return nil, fmt.Errorf("outlet not found")
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,event_type,body,status,created_at FROM shared.notification_outbox WHERE outlet_id=$1 AND body<>'' ORDER BY created_at DESC,id DESC LIMIT $2`, outletID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []OutletNotification{}
	for rows.Next() {
		var n OutletNotification
		if err := rows.Scan(&n.ID, &n.EventType, &n.Body, &n.Status, &n.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (s Store) NotificationPreferences(ctx context.Context, outletID string) (NotificationPreferences, error) {
	var p NotificationPreferences
	err := s.Pool.QueryRow(ctx, `SELECT outlet_id,phone_e164,consent_enabled,deferrals_enabled,major_delays_enabled,locale,consented_at,version,updated_at FROM shared.outlet_notification_preferences WHERE outlet_id=$1`, outletID).
		Scan(&p.OutletID, &p.PhoneE164, &p.ConsentEnabled, &p.DeferralsEnabled, &p.MajorDelaysEnabled, &p.Locale, &p.ConsentedAt, &p.Version, &p.UpdatedAt)
	return p, err
}

func (s Store) SaveNotificationPreferences(ctx context.Context, p NotificationPreferences, expected int, actor string, ev audit.Event) (NotificationPreferences, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	// Lock the parent outlet as well so two first-time preference submissions
	// cannot both observe version zero and silently overwrite each other.
	var outletID string
	if err = tx.QueryRow(ctx, `SELECT id FROM shared.outlets WHERE id=$1 FOR UPDATE`, p.OutletID).Scan(&outletID); err != nil {
		return p, err
	}
	var current int
	var priorPhone string
	var priorConsent *time.Time
	err = tx.QueryRow(ctx, `SELECT version,phone_e164,consented_at FROM shared.outlet_notification_preferences WHERE outlet_id=$1 FOR UPDATE`, p.OutletID).Scan(&current, &priorPhone, &priorConsent)
	if err != nil && err != pgx.ErrNoRows {
		return p, err
	}
	if current != expected {
		return p, fmt.Errorf("conflict: notification preferences changed")
	}
	// Consent timestamps are server-owned. A changed destination requires a
	// fresh affirmative setting instead of inheriting the old number's consent.
	p.ConsentedAt = nil
	if p.ConsentEnabled && current > 0 && priorPhone == p.PhoneE164 && priorConsent != nil {
		p.ConsentedAt = priorConsent
	}
	if p.ConsentEnabled && p.ConsentedAt == nil {
		now := time.Now().UTC()
		p.ConsentedAt = &now
	}
	if !p.ConsentEnabled {
		p.ConsentedAt = nil
	}
	p.Version = current + 1
	p.UpdatedAt = time.Now().UTC()
	err = tx.QueryRow(ctx, `INSERT INTO shared.outlet_notification_preferences(outlet_id,phone_e164,consent_enabled,deferrals_enabled,major_delays_enabled,locale,consented_at,version,updated_by,updated_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(outlet_id) DO UPDATE SET phone_e164=EXCLUDED.phone_e164,consent_enabled=EXCLUDED.consent_enabled,deferrals_enabled=EXCLUDED.deferrals_enabled,major_delays_enabled=EXCLUDED.major_delays_enabled,locale=EXCLUDED.locale,consented_at=EXCLUDED.consented_at,version=EXCLUDED.version,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at
	RETURNING updated_at`, p.OutletID, p.PhoneE164, p.ConsentEnabled, p.DeferralsEnabled, p.MajorDelaysEnabled, p.Locale, p.ConsentedAt, p.Version, actor, p.UpdatedAt).Scan(&p.UpdatedAt)
	if err != nil {
		return p, err
	}
	ev.ActorID = actor
	ev.ActorType = "human"
	ev.ResourceType = "OUTLET_NOTIFICATION_PREFERENCES"
	ev.ResourceID = p.OutletID
	ev.Source = "shared-service"
	ev.NewState = map[string]any{"consentEnabled": p.ConsentEnabled, "deferralsEnabled": p.DeferralsEnabled, "majorDelaysEnabled": p.MajorDelaysEnabled, "locale": p.Locale, "version": p.Version}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	return p, nil
}

type CalendarDay struct {
	Date        string `json:"date"`
	IsOperating bool   `json:"isOperating"`
	Version     int    `json:"version"`
}

type PlanningPolicy struct {
	Version              int       `json:"version"`
	CutoffLocalTime      string    `json:"cutoffLocalTime"`
	DeferralWeightPoints int       `json:"deferralWeightPoints"`
	MaxDeferralCount     int       `json:"maxDeferralCount"`
	MaxUnservedDays      int       `json:"maxUnservedDays"`
	MaxTripsPerVehicle   int       `json:"maxTripsPerVehicle"`
	CreatedBy            string    `json:"createdBy"`
	CreatedAt            time.Time `json:"createdAt"`
}

func (s Store) CurrentPolicy(ctx context.Context) (PlanningPolicy, error) {
	var p PlanningPolicy
	err := s.Pool.QueryRow(ctx, `SELECT p.version,p.cutoff_local_time::text,p.deferral_weight_points,p.max_deferral_count,p.max_unserved_days,p.max_trips_per_vehicle,p.created_by,p.created_at
	FROM shared.planning_policy_current c JOIN shared.planning_policy_versions p USING(version) WHERE c.singleton=true`).Scan(&p.Version, &p.CutoffLocalTime, &p.DeferralWeightPoints, &p.MaxDeferralCount, &p.MaxUnservedDays, &p.MaxTripsPerVehicle, &p.CreatedBy, &p.CreatedAt)
	return p, err
}

func (s Store) CreatePolicy(ctx context.Context, p PlanningPolicy, expectedVersion int, ev audit.Event) (PlanningPolicy, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PlanningPolicy{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7182001)`); err != nil {
		return PlanningPolicy{}, err
	}
	var current PlanningPolicy
	err = tx.QueryRow(ctx, `SELECT p.version,p.cutoff_local_time::text,p.deferral_weight_points,p.max_deferral_count,p.max_unserved_days,p.max_trips_per_vehicle,p.created_by,p.created_at FROM shared.planning_policy_current c JOIN shared.planning_policy_versions p USING(version) WHERE c.singleton=true FOR UPDATE OF c`).Scan(&current.Version, &current.CutoffLocalTime, &current.DeferralWeightPoints, &current.MaxDeferralCount, &current.MaxUnservedDays, &current.MaxTripsPerVehicle, &current.CreatedBy, &current.CreatedAt)
	if err != nil {
		return PlanningPolicy{}, err
	}
	if current.Version != expectedVersion {
		return PlanningPolicy{}, fmt.Errorf("conflict: planning policy changed since version %d", expectedVersion)
	}
	p.Version = current.Version + 1
	err = tx.QueryRow(ctx, `INSERT INTO shared.planning_policy_versions(version,cutoff_local_time,deferral_weight_points,max_deferral_count,max_unserved_days,max_trips_per_vehicle,created_by)
	VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`, p.Version, p.CutoffLocalTime, p.DeferralWeightPoints, p.MaxDeferralCount, p.MaxUnservedDays, p.MaxTripsPerVehicle, p.CreatedBy).Scan(&p.CreatedAt)
	if err != nil {
		return PlanningPolicy{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE shared.planning_policy_current SET version=$1 WHERE singleton=true`, p.Version); err != nil {
		return PlanningPolicy{}, err
	}
	ev.PreviousState = map[string]any{"policy": current}
	ev.NewState = map[string]any{"policy": p}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return PlanningPolicy{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PlanningPolicy{}, err
	}
	return p, nil
}

// LocationChange says what an outlet update does to the outlet's exact position. The zero value
// leaves it alone; Set stores Latitude/Longitude as the verified position; Clear removes it, so the
// outlet goes back to the approximate district position.
type LocationChange struct {
	Set       bool
	Clear     bool
	Latitude  float64
	Longitude float64
}

// Sri Lanka's extent with a margin. A position outside it is almost always swapped or mistyped.
const (
	minLatitude, maxLatitude   = 5.5, 10.0
	minLongitude, maxLongitude = 79.3, 82.2
)

// ValidateLocation rejects a position that cannot be a place in Sri Lanka.
func ValidateLocation(latitude, longitude float64) error {
	if latitude != latitude || longitude != longitude || latitude < minLatitude || latitude > maxLatitude || longitude < minLongitude || longitude > maxLongitude {
		return fmt.Errorf("location must be a latitude between %.1f and %.1f and a longitude between %.1f and %.1f (Sri Lanka); check the two are not swapped", minLatitude, maxLatitude, minLongitude, maxLongitude)
	}
	return nil
}

func (s Store) UpdateOutlet(ctx context.Context, o Outlet, expected int, ev audit.Event) (Outlet, error) {
	return s.UpdateOutletLocation(ctx, o, expected, LocationChange{}, ev)
}

// UpdateOutletLocation is UpdateOutlet that can also set or clear the outlet's exact position in the
// same versioned, audited change.
func (s Store) UpdateOutletLocation(ctx context.Context, o Outlet, expected int, loc LocationChange, ev audit.Event) (Outlet, error) {
	if loc.Set {
		if err := ValidateLocation(loc.Latitude, loc.Longitude); err != nil {
			return Outlet{}, fmt.Errorf("invalid: %w", err)
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Outlet{}, err
	}
	defer tx.Rollback(ctx)
	var before Outlet
	err = tx.QueryRow(ctx, outletSelect+` WHERE id=$1 FOR UPDATE`, o.ID).Scan(&before.ID, &before.Brand, &before.Name, &before.District, &before.Depot, &before.DockType, &before.ParkingConstraint, &before.MallWindow, &before.WindowOpenTime, &before.WindowCloseTime, &before.AccessInstructions, &before.AccessInstructionsUpdatedBy, &before.AccessInstructionsUpdatedAt, &before.AccessInstructionsConfirmedBy, &before.AccessInstructionsConfirmedAt, &before.ChilledTemperatureMinC, &before.ChilledTemperatureMaxC, &before.Version)
	if err != nil {
		return Outlet{}, err
	}
	if before.Version != expected {
		return Outlet{}, fmt.Errorf("conflict: outlet version changed")
	}
	var beforeLat, beforeLng *float64
	if err = tx.QueryRow(ctx, `SELECT latitude, longitude FROM outlets WHERE id=$1`, o.ID).Scan(&beforeLat, &beforeLng); err != nil {
		return Outlet{}, err
	}
	var updated Outlet
	err = tx.QueryRow(ctx, `UPDATE shared.outlets SET brand=$2,name=$3,district=$4,depot=$5,dock_type=$6,parking_constraint=$7,mall_window=$8,
	window_open_time=NULLIF($9,'')::time,window_close_time=NULLIF($10,'')::time,
	access_instructions=$11,
	access_instructions_updated_by=CASE WHEN access_instructions IS DISTINCT FROM $11 THEN $12 ELSE access_instructions_updated_by END,
	access_instructions_updated_at=CASE WHEN access_instructions IS DISTINCT FROM $11 THEN now() ELSE access_instructions_updated_at END,
	access_instructions_confirmed_by=CASE WHEN access_instructions IS DISTINCT FROM $11 THEN '' ELSE access_instructions_confirmed_by END,
	access_instructions_confirmed_at=CASE WHEN access_instructions IS DISTINCT FROM $11 THEN NULL ELSE access_instructions_confirmed_at END,
	chilled_temperature_min_c=$13,chilled_temperature_max_c=$14,
	latitude=CASE WHEN $15 THEN $16::double precision WHEN $18 THEN NULL ELSE latitude END,
	longitude=CASE WHEN $15 THEN $17::double precision WHEN $18 THEN NULL ELSE longitude END,
	version=version+1
	WHERE id=$1 RETURNING id,brand,name,COALESCE(district,''),COALESCE(depot,''),COALESCE(dock_type,''),COALESCE(parking_constraint,''),mall_window,
	COALESCE(window_open_time::text,''),COALESCE(window_close_time::text,''),COALESCE(access_instructions,''),COALESCE(access_instructions_updated_by,''),access_instructions_updated_at,COALESCE(access_instructions_confirmed_by,''),access_instructions_confirmed_at,chilled_temperature_min_c,chilled_temperature_max_c,version`, o.ID, o.Brand, o.Name, o.District, o.Depot, o.DockType, o.ParkingConstraint, o.MallWindow, o.WindowOpenTime, o.WindowCloseTime, o.AccessInstructions, ev.ActorID, o.ChilledTemperatureMinC, o.ChilledTemperatureMaxC, loc.Set, loc.Latitude, loc.Longitude, loc.Clear).
		Scan(&updated.ID, &updated.Brand, &updated.Name, &updated.District, &updated.Depot, &updated.DockType, &updated.ParkingConstraint, &updated.MallWindow, &updated.WindowOpenTime, &updated.WindowCloseTime, &updated.AccessInstructions, &updated.AccessInstructionsUpdatedBy, &updated.AccessInstructionsUpdatedAt, &updated.AccessInstructionsConfirmedBy, &updated.AccessInstructionsConfirmedAt, &updated.ChilledTemperatureMinC, &updated.ChilledTemperatureMaxC, &updated.Version)
	if err != nil {
		return Outlet{}, err
	}
	// Answer with the outlet as every read shows it, including its position, so a client that replaces
	// its copy with this response does not lose the map position.
	if updated, err = scanOutlet(tx.QueryRow(ctx, outletReadSelect+` WHERE id=$1`, o.ID)); err != nil {
		return Outlet{}, err
	}
	prev, _ := json.Marshal(before)
	next, _ := json.Marshal(updated)
	ev.PreviousState = map[string]any{"outlet": json.RawMessage(prev), "exactLocation": exactLocation(beforeLat, beforeLng)}
	ev.NewState = map[string]any{"outlet": json.RawMessage(next), "exactLocation": exactLocation(ifExact(updated.Latitude, updated.LocationApproximate), ifExact(updated.Longitude, updated.LocationApproximate))}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return Outlet{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Outlet{}, err
	}
	return updated, nil
}

// exactLocation is how audit history records a verified position: the pair, or nil when the outlet
// only has the approximate district position.
func exactLocation(latitude, longitude *float64) any {
	if latitude == nil || longitude == nil {
		return nil
	}
	return map[string]float64{"latitude": *latitude, "longitude": *longitude}
}

func ifExact(value *float64, approximate bool) *float64 {
	if approximate {
		return nil
	}
	return value
}

func (s Store) ConfirmOutletAccessInstructions(ctx context.Context, outletID string, expected int, ev audit.Event) (Outlet, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Outlet{}, err
	}
	defer tx.Rollback(ctx)
	var before Outlet
	err = tx.QueryRow(ctx, outletSelect+` WHERE id=$1 FOR UPDATE`, outletID).Scan(&before.ID, &before.Brand, &before.Name, &before.District, &before.Depot, &before.DockType, &before.ParkingConstraint, &before.MallWindow, &before.WindowOpenTime, &before.WindowCloseTime, &before.AccessInstructions, &before.AccessInstructionsUpdatedBy, &before.AccessInstructionsUpdatedAt, &before.AccessInstructionsConfirmedBy, &before.AccessInstructionsConfirmedAt, &before.ChilledTemperatureMinC, &before.ChilledTemperatureMaxC, &before.Version)
	if err != nil {
		return Outlet{}, err
	}
	if before.Version != expected {
		return Outlet{}, fmt.Errorf("conflict: outlet version changed")
	}
	if strings.TrimSpace(before.AccessInstructions) == "" {
		return Outlet{}, fmt.Errorf("invalid: access instructions are empty")
	}
	var confirmed Outlet
	err = tx.QueryRow(ctx, `UPDATE shared.outlets SET access_instructions_confirmed_by=$2,access_instructions_confirmed_at=now(),version=version+1
		WHERE id=$1 RETURNING id,brand,name,COALESCE(district,''),COALESCE(depot,''),COALESCE(dock_type,''),COALESCE(parking_constraint,''),mall_window,
		COALESCE(window_open_time::text,''),COALESCE(window_close_time::text,''),COALESCE(access_instructions,''),COALESCE(access_instructions_updated_by,''),access_instructions_updated_at,COALESCE(access_instructions_confirmed_by,''),access_instructions_confirmed_at,chilled_temperature_min_c,chilled_temperature_max_c,version`, outletID, ev.ActorID).
		Scan(&confirmed.ID, &confirmed.Brand, &confirmed.Name, &confirmed.District, &confirmed.Depot, &confirmed.DockType, &confirmed.ParkingConstraint, &confirmed.MallWindow, &confirmed.WindowOpenTime, &confirmed.WindowCloseTime, &confirmed.AccessInstructions, &confirmed.AccessInstructionsUpdatedBy, &confirmed.AccessInstructionsUpdatedAt, &confirmed.AccessInstructionsConfirmedBy, &confirmed.AccessInstructionsConfirmedAt, &confirmed.ChilledTemperatureMinC, &confirmed.ChilledTemperatureMaxC, &confirmed.Version)
	if err != nil {
		return Outlet{}, err
	}
	prev, _ := json.Marshal(before)
	next, _ := json.Marshal(confirmed)
	ev.Action = "OUTLET_ACCESS_INSTRUCTIONS_CONFIRMED"
	ev.ResourceType = "OUTLET"
	ev.ResourceID = outletID
	ev.PreviousState = map[string]any{"outlet": json.RawMessage(prev)}
	ev.NewState = map[string]any{"outlet": json.RawMessage(next)}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return Outlet{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Outlet{}, err
	}
	return confirmed, nil
}

func (s Store) Calendar(ctx context.Context, from, to string) ([]CalendarDay, error) {
	rows, err := s.Pool.Query(ctx, `SELECT date::text,is_operating,version FROM shared.operating_calendar WHERE date BETWEEN $1 AND $2 ORDER BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CalendarDay{}
	for rows.Next() {
		var d CalendarDay
		if err := rows.Scan(&d.Date, &d.IsOperating, &d.Version); err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

func (s Store) UpdateCalendar(ctx context.Context, day CalendarDay, expected int, ev audit.Event) (CalendarDay, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return CalendarDay{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, day.Date); err != nil {
		return CalendarDay{}, err
	}
	var before CalendarDay
	err = tx.QueryRow(ctx, `SELECT date::text,is_operating,version FROM shared.operating_calendar WHERE date=$1 FOR UPDATE`, day.Date).Scan(&before.Date, &before.IsOperating, &before.Version)
	if err == pgx.ErrNoRows {
		if expected != 0 {
			return CalendarDay{}, fmt.Errorf("conflict: calendar version changed")
		}
	} else if err != nil {
		return CalendarDay{}, err
	} else if before.Version != expected {
		return CalendarDay{}, fmt.Errorf("conflict: calendar version changed")
	}
	err = tx.QueryRow(ctx, `INSERT INTO shared.operating_calendar(date,is_operating,version) VALUES($1,$2,1) ON CONFLICT(date) DO UPDATE SET is_operating=EXCLUDED.is_operating,version=shared.operating_calendar.version+1 RETURNING date::text,is_operating,version`, day.Date, day.IsOperating).Scan(&day.Date, &day.IsOperating, &day.Version)
	if err != nil {
		return CalendarDay{}, err
	}
	ev.PreviousState = map[string]any{"calendar": before}
	ev.NewState = map[string]any{"calendar": day}
	if err = s.insertAuditTx(ctx, tx, ev); err != nil {
		return CalendarDay{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CalendarDay{}, err
	}
	return day, nil
}

func (s Store) Outlet(ctx context.Context, id string) (Outlet, error) {
	o, err := scanOutlet(s.Pool.QueryRow(ctx, outletReadSelect+" WHERE id = $1", id))
	if err == pgx.ErrNoRows {
		return Outlet{}, fmt.Errorf("not found")
	}
	return o, err
}

func (s Store) ListOutlets(ctx context.Context) ([]Outlet, error) {
	rows, err := s.Pool.Query(ctx, outletReadSelect+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Outlet
	for rows.Next() {
		o, err := scanOutlet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if out == nil {
		out = []Outlet{}
	}
	return out, rows.Err()
}

func (s Store) TravelRows(ctx context.Context) ([][4]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT from_district, to_district, km::text, minutes::text FROM district_travel`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][4]string
	for rows.Next() {
		var r [4]string
		if err := rows.Scan(&r[0], &r[1], &r[2], &r[3]); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s Store) ServiceMinutes(ctx context.Context, kind string) int {
	var m int
	err := s.Pool.QueryRow(ctx, `SELECT minutes FROM service_allowance WHERE stop_kind = $1`, kind).Scan(&m)
	if err != nil {
		_ = s.Pool.QueryRow(ctx, `SELECT minutes FROM service_allowance WHERE stop_kind = 'default'`).Scan(&m)
	}
	if m == 0 {
		return 15
	}
	return m
}

const outletSelect = `
	SELECT id, brand, name, COALESCE(district,''), COALESCE(depot,''), COALESCE(dock_type,'normal'),
		COALESCE(parking_constraint,'normal'), mall_window,
		COALESCE(window_open_time::text,''), COALESCE(window_close_time::text,''),
		COALESCE(access_instructions,''), COALESCE(access_instructions_updated_by,''), access_instructions_updated_at,
		COALESCE(access_instructions_confirmed_by,''), access_instructions_confirmed_at, chilled_temperature_min_c, chilled_temperature_max_c, version
FROM outlets`

// outletReadSelect adds the map position to outlet reads. Updates keep using
// outletSelect, which their positional scans depend on.
var outletReadSelect = strings.Replace(outletSelect, "\nFROM outlets", `,
		COALESCE(latitude, (SELECT dl.latitude FROM district_locations dl WHERE dl.district = outlets.district)
			+ ((('x' || substr(md5(id), 1, 4))::bit(16)::int / 65535.0) - 0.5) * 0.04),
		COALESCE(longitude, (SELECT dl.longitude FROM district_locations dl WHERE dl.district = outlets.district)
			+ ((('x' || substr(md5(id), 5, 4))::bit(16)::int / 65535.0) - 0.5) * 0.04),
		latitude IS NULL OR longitude IS NULL
FROM outlets`, 1)

type scanner interface {
	Scan(dest ...any) error
}

func scanOutlet(row scanner) (Outlet, error) {
	var o Outlet
	err := row.Scan(&o.ID, &o.Brand, &o.Name, &o.District, &o.Depot, &o.DockType, &o.ParkingConstraint, &o.MallWindow, &o.WindowOpenTime, &o.WindowCloseTime, &o.AccessInstructions, &o.AccessInstructionsUpdatedBy, &o.AccessInstructionsUpdatedAt, &o.AccessInstructionsConfirmedBy, &o.AccessInstructionsConfirmedAt, &o.ChilledTemperatureMinC, &o.ChilledTemperatureMaxC, &o.Version, &o.Latitude, &o.Longitude, &o.LocationApproximate)
	return o, err
}

func (s Store) InsertAudit(ctx context.Context, ev audit.Event) error {
	if ev.EventID == "" {
		ev.EventID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	prev, _ := json.Marshal(ev.PreviousState)
	next, _ := json.Marshal(ev.NewState)
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO audit.events (
			event_id, correlation_id, actor_id, actor_type, action,
			resource_type, resource_id, previous_state, new_state, reason, timestamp, source
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (event_id) DO NOTHING
	`, ev.EventID, ev.CorrelationID, ev.ActorID, ev.ActorType, ev.Action,
		ev.ResourceType, ev.ResourceID, prev, next, ev.Reason, ev.Timestamp, ev.Source)
	return err
}

func (s Store) insertAuditTx(ctx context.Context, tx pgx.Tx, ev audit.Event) error {
	if ev.EventID == "" {
		ev.EventID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	prev, _ := json.Marshal(ev.PreviousState)
	next, _ := json.Marshal(ev.NewState)
	_, err := tx.Exec(ctx, `INSERT INTO audit.events(event_id,correlation_id,actor_id,actor_type,action,resource_type,resource_id,previous_state,new_state,reason,timestamp,source)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, ev.EventID, ev.CorrelationID, ev.ActorID, ev.ActorType, ev.Action, ev.ResourceType, ev.ResourceID, prev, next, ev.Reason, ev.Timestamp, ev.Source)
	return err
}

// ShortfallBody is the plain-language store notice for a loader shortfall
// after the dispatcher has decided what will arrive.
func ShortfallBody(locale string, e NotificationEvent) string {
	type phrase struct{ en, si, ta string }
	outcomes := map[string]phrase{
		"PARTIAL_LOAD":     {"the rest will arrive on the next delivery", "ඉතිරිය ඊළඟ බෙදාහැරීමේදී පැමිණේ", "மீதமுள்ளவை அடுத்த விநியோகத்தில் வரும்"},
		"HOLD":             {"delivery is on hold until the goods are ready", "භාණ්ඩ සූදානම් වන තුරු බෙදාහැරීම රඳවා ඇත", "பொருட்கள் தயாராகும் வரை விநியோகம் நிறுத்தப்பட்டுள்ளது"},
		"MOVE_TO_NEXT_RUN": {"this order moves to the next delivery run", "මෙම ඇණවුම ඊළඟ බෙදාහැරීමට මාරු කර ඇත", "இந்த ஆர்டர் அடுத்த விநியோகத்துக்கு மாற்றப்பட்டது"},
	}
	o, ok := outcomes[e.Reason]
	if !ok {
		o = outcomes["PARTIAL_LOAD"]
	}
	switch locale {
	case "si":
		return fmt.Sprintf("Waypoint: ඇණවුම %s සඳහා ඒකක %d ක් අඩුය; %s.", e.OrderRef, e.Units, o.si)
	case "ta":
		return fmt.Sprintf("Waypoint: ஆர்டர் %s இல் %d அலகுகள் குறைவு; %s.", e.OrderRef, e.Units, o.ta)
	}
	return fmt.Sprintf("Waypoint: order %s is %d units short; %s.", e.OrderRef, e.Units, o.en)
}
