package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

type conflictDriver struct {
	stubDriver
	date    string
	settled string
}

func (c *conflictDriver) ListSyncConflicts(_ context.Context, _ *authorization.Profile, date string, _ bool) ([]domain.SyncConflict, error) {
	c.date = date
	return []domain.SyncConflict{{ID: "c1", OperationID: "op1", RecordedPlanVersion: 3, CurrentPlanVersion: 4}}, nil
}

func (c *conflictDriver) SettleSyncConflict(_ context.Context, _ *authorization.Profile, id string) (domain.SyncConflict, error) {
	c.settled = id
	return domain.SyncConflict{ID: id}, nil
}

func TestSyncConflictRoutesAreDispatcherOnly(t *testing.T) {
	svc := &conflictDriver{}
	rec := httptest.NewRecorder()
	testRouter("usr-dispatcher", svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/sync-conflicts?date=2026-09-29", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"recordedPlanVersion":3`) || svc.date != "2026-09-29" {
		t.Fatalf("list %d %s date=%q", rec.Code, rec.Body.String(), svc.date)
	}
	rec = httptest.NewRecorder()
	testRouter("usr-dispatcher", svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/delivery/sync-conflicts/c1/settle", nil))
	if rec.Code != http.StatusOK || svc.settled != "c1" {
		t.Fatalf("settle %d", rec.Code)
	}
	for _, who := range []string{"usr-driver", "usr-loader", "usr-store-manager"} {
		rec = httptest.NewRecorder()
		testRouter(who, svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/delivery/sync-conflicts", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s list status=%d", who, rec.Code)
		}
	}
}
