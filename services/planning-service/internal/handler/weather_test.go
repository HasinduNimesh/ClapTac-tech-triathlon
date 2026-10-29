package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWeatherForecastIsDispatcherReadableAndListsDistrictsWithoutALookup(t *testing.T) {
	today := time.Now().In(time.FixedZone("Asia/Colombo", 5*3600+30*60)).Format("2006-01-02")
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/planning/disruption-risks/weather?date="+today, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"district":"Kandy"`) || !strings.Contains(rec.Body.String(), `"error":"weather lookup is turned off"`) {
		t.Fatalf("dispatcher read returned %d %s", rec.Code, rec.Body.String())
	}

	bad := httptest.NewRecorder()
	testRouter("usr-dispatcher", &stubPlanner{}).ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/v1/planning/disruption-risks/weather?date=soon", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid date returned %d", bad.Code)
	}

	denied := httptest.NewRecorder()
	testRouter("usr-store-manager", &stubPlanner{}).ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/api/v1/planning/disruption-risks/weather?date="+today, nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("store manager read returned %d", denied.Code)
	}
}
