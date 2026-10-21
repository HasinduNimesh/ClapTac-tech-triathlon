package errors

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternalCauseAnswersGenericallyAndLogsTheRealError(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	rec := httptest.NewRecorder()
	InternalCause(rec, errors.New("ERROR: deadlock detected (SQLSTATE 40P01)"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Detail != "unexpected error" {
		t.Fatalf("the caller must see only a generic detail, got %+v err=%v", p, err)
	}
	if strings.Contains(rec.Body.String(), "deadlock") {
		t.Fatal("the real error must not reach the caller")
	}
	line := logs.String()
	if !strings.Contains(line, `"msg":"internal_error"`) || !strings.Contains(line, "deadlock detected") || !strings.Contains(line, `"level":"ERROR"`) {
		t.Fatalf("the real error must be logged: %s", line)
	}
}

func TestInternalCauseCutsAVeryLongError(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	InternalCause(httptest.NewRecorder(), errors.New(strings.Repeat("x", 5000)))
	if logs.Len() > 700 {
		t.Fatalf("log line not bounded: %d bytes", logs.Len())
	}
}
