package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryWritePermissions(t *testing.T) {
	for _, path := range []string{"/api/v1/planning/recovery", "/api/v1/planning/recovery/case/approve"} {
		for _, actor := range []string{"usr-store-manager", "usr-driver", "usr-loader"} {
			rec := httptest.NewRecorder()
			testRouter(actor, &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s: %d", actor, path, rec.Code)
			}
		}
	}
}
