package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverySuspensionRejectsHumanRoles(t *testing.T) {
	for _, role := range []string{"usr-dispatcher", "usr-store-manager", "usr-driver", "usr-loader"} {
		rec := httptest.NewRecorder()
		testRouter(role, &stubDriver{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/internal/recovery/suspend", strings.NewReader(`{"caseId":"case","stops":[]}`)))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s could suspend: %d", role, rec.Code)
		}
	}
}
