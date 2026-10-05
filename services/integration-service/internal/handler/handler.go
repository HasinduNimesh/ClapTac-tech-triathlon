package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/auth"
	apierrors "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/errors"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/httpx"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/integration-service/internal/notify"
	twilioClient "github.com/twilio/twilio-go/client"
)

type Handler struct {
	Authn             auth.Authenticator
	SMS               SMSProvider
	CallbackURL       string
	CallbackAuthToken string
	StatusSink        StatusSink
	// GatewayWebhookSecret signs the cellular gateway's webhooks (HMAC-SHA256). Empty switches the endpoint off.
	GatewayWebhookSecret string
}

type SMSProvider interface {
	Send(context.Context, notify.SMS) (string, error)
}
type StatusSink interface {
	UpdateStatus(context.Context, notify.StatusUpdate) error
}

func (h Handler) Routes(r chi.Router) {
	r.Route("/api/v1/integrations", func(r chi.Router) {
		r.Post("/routing/directions", h.withAuth(h.directions))
		r.Post("/storage/presign", h.withAuth(h.presign))
		r.Post("/notifications/send", h.withAuth(h.notify))
		r.Post("/notifications/status", h.statusCallback)
		r.Post("/notifications/gateway", h.gatewayWebhook)
	})
}

func (h Handler) statusCallback(w http.ResponseWriter, r *http.Request) {
	if h.CallbackURL == "" || h.CallbackAuthToken == "" || h.StatusSink == nil {
		apierrors.NotImplemented(w, "notification status callback is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		apierrors.RequestEntityTooLarge(w, "callback body exceeds limit")
		return
	}
	validator := twilioClient.NewRequestValidator(h.CallbackAuthToken)
	if !validator.ValidateBody(h.CallbackURL, body, r.Header.Get("X-Twilio-Signature")) {
		apierrors.Forbidden(w, "invalid Twilio callback signature")
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		apierrors.BadRequest(w, "invalid callback form")
		return
	}
	u := notify.StatusUpdate{MessageSID: form.Get("MessageSid"), Status: form.Get("MessageStatus"), ErrorCode: form.Get("ErrorCode")}
	if len(u.MessageSID) != 34 || len(u.ErrorCode) > 40 {
		apierrors.BadRequest(w, "invalid callback fields")
		return
	}
	if err = h.StatusSink.UpdateStatus(r.Context(), u); err != nil {
		apierrors.Internal(w, "notification status update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// gatewayWebhook receives the cellular gateway's signed events and records what happened to an outbound text.
// It is called by the gateway, not by a user, so the HMAC signature is its only credential. Events that say
// nothing about a notification (inbound SMS, calls, device presence) are acknowledged and ignored. A failure to
// record the status answers 500, so the gateway retries the delivery.
func (h Handler) gatewayWebhook(w http.ResponseWriter, r *http.Request) {
	if h.GatewayWebhookSecret == "" || h.StatusSink == nil {
		apierrors.NotImplemented(w, "gateway webhook is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	if err != nil {
		apierrors.RequestEntityTooLarge(w, "webhook body exceeds limit")
		return
	}
	if !notify.VerifyGatewaySignature(h.GatewayWebhookSecret, body, r.Header.Get("X-CG-Signature")) {
		apierrors.Forbidden(w, "invalid gateway webhook signature")
		return
	}
	event, err := notify.ParseGatewayEvent(body)
	if err != nil {
		apierrors.BadRequest(w, "invalid gateway event")
		return
	}
	update, relevant := event.StatusUpdate()
	if !relevant {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err = h.StatusSink.UpdateStatus(r.Context(), update); err != nil {
		apierrors.Internal(w, "notification status update failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.Authn.Authenticate(r)
		if err != nil {
			apierrors.Unauthorized(w, "valid bearer token required")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}
}

func (h Handler) directions(w http.ResponseWriter, _ *http.Request) {
	apierrors.NotImplemented(w, "routing provider is not configured")
}

func (h Handler) presign(w http.ResponseWriter, _ *http.Request) {
	apierrors.NotImplemented(w, "external storage signing adapter is not configured")
}

func (h Handler) notify(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	allowed := false
	if p != nil {
		for _, s := range p.Scopes {
			if s == "notifications:send" {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		apierrors.Forbidden(w, "service notification scope required")
		return
	}
	if h.SMS == nil {
		apierrors.NotImplemented(w, "notification provider is not configured")
		return
	}
	var input struct {
		To   string `json:"to"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		apierrors.BadRequest(w, "invalid notification JSON")
		return
	}
	sid, err := h.SMS.Send(r.Context(), notify.SMS{To: strings.TrimSpace(input.To), Body: strings.TrimSpace(input.Body)})
	if err != nil {
		if strings.Contains(err.Error(), "outcome is unknown") {
			httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "unknown"})
			return
		}
		apierrors.BadGateway(w, "notification provider rejected the request")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "providerMessageId": sid})
}
