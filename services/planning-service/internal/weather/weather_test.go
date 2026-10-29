package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 10, 29, 9, 0, 0, 0, colombo) }

// hours builds one location's hourly forecast with rain mm at the given hours and 0 elsewhere.
func hours(date string, rain map[int]float64, prob int) map[string]any {
	times, mm, p, g := []string{}, []float64{}, []int{}, []float64{}
	for h := 0; h < 24; h++ {
		times = append(times, fmt.Sprintf("%sT%02d:00", date, h))
		mm = append(mm, rain[h])
		p = append(p, prob)
		g = append(g, 20)
	}
	return map[string]any{"hourly": map[string]any{"time": times, "precipitation": mm, "precipitation_probability": p, "wind_gusts_10m": g}}
}

func server(t *testing.T, calls *int, body func(date string) any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		q := r.URL.Query()
		if got := len(strings.Split(q.Get("latitude"), ",")); got != len(Districts) {
			t.Errorf("latitudes=%d, want %d", got, len(Districts))
		}
		if q.Get("timezone") != "Asia/Colombo" || q.Get("start_date") != q.Get("end_date") {
			t.Errorf("unexpected query %v", q)
		}
		_ = json.NewEncoder(w).Encode(body(q.Get("start_date")))
	}))
}

func TestForDateSuggestsRisksFromRain(t *testing.T) {
	calls := 0
	srv := server(t, &calls, func(date string) any {
		locs := make([]any, len(Districts))
		for i := range locs {
			locs[i] = hours(date, nil, 10)
		}
		locs[0] = hours(date, map[int]float64{14: 12, 15: 8}, 80)         // Colombo: 20 mm, 12 mm peak hour
		locs[7] = hours(date, map[int]float64{3: 30, 13: 25, 16: 10}, 90) // Kandy: 65 mm
		locs[9] = hours(date, map[int]float64{1: 40, 2: 40, 9: 30}, 95)   // Nuwara Eliya: 110 mm
		locs[3] = hours(date, map[int]float64{10: 5, 11: 4}, 60)          // Galle: light rain
		return locs
	})
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Now: fixedNow}
	f, err := c.ForDate(context.Background(), "2026-10-30")
	if err != nil || f.Error != "" || f.FetchedAt == nil {
		t.Fatalf("forecast=%+v err=%v", f, err)
	}
	by := map[string]DistrictForecast{}
	for _, d := range f.Districts {
		by[d.District] = d
	}
	want := map[string][2]string{"Colombo": {"HEAVY_RAIN", "MEDIUM"}, "Kandy": {"HEAVY_RAIN", "HIGH"}, "Nuwara Eliya": {"FLOODING", "HIGH"}}
	for name, d := range by {
		w, ok := want[name]
		if !ok {
			if d.Suggestion != nil {
				t.Errorf("%s: unexpected suggestion %+v", name, d.Suggestion)
			}
			continue
		}
		if d.Suggestion == nil || d.Suggestion.RiskType != w[0] || d.Suggestion.Severity != w[1] {
			t.Errorf("%s: suggestion=%+v, want %v", name, d.Suggestion, w)
		}
	}
	colomboDay := by["Colombo"]
	if *colomboDay.DayRainMm != 20 || *colomboDay.WorkingRainMm != 20 || colomboDay.PeakHour != "14:00" || *colomboDay.RainProbability != 80 {
		t.Errorf("colombo figures=%+v", colomboDay)
	}
	if by["Colombo"].Suggestion.Confidence != 0.8 || !strings.Contains(by["Colombo"].Suggestion.Summary, "around 14:00") {
		t.Errorf("colombo suggestion=%+v", by["Colombo"].Suggestion)
	}
	if kandy := by["Kandy"]; *kandy.WorkingRainMm != 35 || *kandy.DayRainMm != 65 {
		t.Errorf("kandy working=%v day=%v, want 35 and 65 (03:00 rain is outside working hours)", *kandy.WorkingRainMm, *kandy.DayRainMm)
	}
	if _, err := c.ForDate(context.Background(), "2026-10-30"); err != nil || calls != 1 {
		t.Errorf("second read should come from the cache: calls=%d err=%v", calls, err)
	}
}

func TestForDateKeepsDistrictsWhenTheLookupFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "busy", 503) }))
	defer srv.Close()
	for name, c := range map[string]*Client{"down": {BaseURL: srv.URL, Now: fixedNow}, "off": {Now: fixedNow}} {
		f, err := (c).ForDate(context.Background(), "2026-10-29")
		if err != nil || f.Error == "" || len(f.Districts) != len(Districts) || f.Districts[0].DayRainMm != nil {
			t.Errorf("%s: forecast=%+v err=%v", name, f, err)
		}
	}
}

func TestForDateRejectsBadDatesAndListsDistrictsOutsideTheForecast(t *testing.T) {
	c := &Client{Now: fixedNow}
	for _, date := range []string{"2026-02-30", "tomorrow"} {
		if _, err := c.ForDate(context.Background(), date); err == nil || !strings.HasPrefix(err.Error(), "invalid") {
			t.Errorf("%s: err=%v, want invalid", date, err)
		}
	}
	// Outside the forecast window the districts are still listed, so older plans can place their risks.
	for date, wantErr := range map[string]string{"2026-10-21": "available from", "2026-11-14": "available from", "2026-10-22": "turned off", "2026-11-13": "turned off"} {
		f, err := c.ForDate(context.Background(), date)
		if err != nil || !strings.Contains(f.Error, wantErr) || len(f.Districts) != len(Districts) {
			t.Errorf("%s: forecast=%+v err=%v, want error containing %q", date, f, err, wantErr)
		}
	}
}
