package errors

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	// Code is a stable machine-readable reason a client can branch on.
	Code string `json:"code,omitempty"`
}

func Write(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

// WriteCode is Write plus a stable machine-readable code.
func WriteCode(w http.ResponseWriter, status int, title, detail, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Type: "about:blank", Title: title, Status: status, Detail: detail, Code: code})
}

func Unauthorized(w http.ResponseWriter, detail string) {
	Write(w, http.StatusUnauthorized, "Unauthorized", detail)
}

func Forbidden(w http.ResponseWriter, detail string) {
	Write(w, http.StatusForbidden, "Forbidden", detail)
}

func NotImplemented(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotImplemented, "Not Implemented", detail)
}

func BadRequest(w http.ResponseWriter, detail string) {
	Write(w, http.StatusBadRequest, "Bad Request", detail)
}

func NotFound(w http.ResponseWriter, detail string) {
	Write(w, http.StatusNotFound, "Not Found", detail)
}

func Internal(w http.ResponseWriter, detail string) {
	Write(w, http.StatusInternalServerError, "Internal Server Error", detail)
}

// InternalCause answers 500 and records why. The caller sees only "unexpected error"; the real error is written to
// the service log, because a 500 whose cause is not logged cannot be diagnosed afterwards.
func InternalCause(w http.ResponseWriter, err error) {
	msg := err.Error()
	if len(msg) > 400 {
		msg = msg[:400]
	}
	slog.Error("internal_error", "error", msg)
	Internal(w, "unexpected error")
}

func Conflict(w http.ResponseWriter, detail string) {
	Write(w, http.StatusConflict, "Conflict", detail)
}

func BadGateway(w http.ResponseWriter, detail string) {
	Write(w, http.StatusBadGateway, "Bad Gateway", detail)
}

func RequestEntityTooLarge(w http.ResponseWriter, detail string) {
	Write(w, http.StatusRequestEntityTooLarge, "Payload Too Large", detail)
}
