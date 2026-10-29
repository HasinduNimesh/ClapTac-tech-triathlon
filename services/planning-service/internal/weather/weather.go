// Package weather reads rain forecasts for the delivery districts from Open-Meteo and turns heavy rain into
// suggested disruption risks. Suggestions are never stored: a dispatcher adds one as a risk (or ignores it),
// so a wrong forecast never changes a plan on its own.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// District centres used to place outlets on the maps; keep in step with shared.district_locations
// (database/migrations/0043_outlet_locations.sql).
var Districts = []District{
	{"Colombo", 6.9271, 79.8612},
	{"Gampaha", 7.0840, 79.9939},
	{"Kalutara", 6.5854, 79.9607},
	{"Galle", 6.0535, 80.2210},
	{"Matara", 5.9549, 80.5550},
	{"Kurunegala", 7.4863, 80.3647},
	{"Puttalam", 8.0362, 79.8283},
	{"Kandy", 7.2906, 80.6337},
	{"Matale", 7.4675, 80.6234},
	{"Nuwara Eliya", 6.9497, 80.7891},
	{"Badulla", 6.9934, 81.0550},
	{"Kegalle", 7.2513, 80.3464},
}

type District struct {
	Name      string
	Latitude  float64
	Longitude float64
}

// Rain thresholds for one day at a district centre. Sri Lanka's Department of Meteorology calls 50 mm
// "fairly heavy" and 100 mm "heavy"; an hour of 10–20 mm is enough to flood low roads in towns.
const (
	MediumDayMm    = 25.0
	MediumHourMm   = 10.0
	HighDayMm      = 50.0
	HighHourMm     = 20.0
	FloodingDayMm  = 100.0
	workStartHour  = 6
	workEndHour    = 20 // exclusive: rain from 06:00 to 19:59
	SourceName     = "Open-Meteo forecast"
	cacheFor       = 30 * time.Minute
	pastDaysAllow  = 7
	aheadDaysAllow = 15
)

type Suggestion struct {
	RiskType   string  `json:"riskType"`
	Severity   string  `json:"severity"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

type DistrictForecast struct {
	District  string  `json:"district"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// Nil when the forecast could not be read; the district is still listed so the maps can place risks.
	DayRainMm       *float64    `json:"dayRainMm,omitempty"`
	WorkingRainMm   *float64    `json:"workingRainMm,omitempty"`
	MaxHourlyRainMm *float64    `json:"maxHourlyRainMm,omitempty"`
	PeakHour        string      `json:"peakHour,omitempty"`
	RainProbability *int        `json:"rainProbability,omitempty"`
	MaxGustKmh      *float64    `json:"maxGustKmh,omitempty"`
	Suggestion      *Suggestion `json:"suggestion,omitempty"`
}

type Forecast struct {
	Date      string             `json:"date"`
	Source    string             `json:"source"`
	SourceURL string             `json:"sourceUrl"`
	FetchedAt *time.Time         `json:"fetchedAt,omitempty"`
	Error     string             `json:"error,omitempty"`
	Districts []DistrictForecast `json:"districts"`
}

// Client fetches and caches forecasts. A zero BaseURL turns the lookup off (districts are still returned).
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Now     func() time.Time

	mu    sync.Mutex
	cache map[string]Forecast
}

var colombo = time.FixedZone("Asia/Colombo", 5*3600+30*60)

// ForDate returns the rain forecast for every delivery district on date (YYYY-MM-DD, Sri Lanka time).
// A failed lookup is reported in Error with the districts listed and no figures, never as an error,
// so a weather outage never hides the maps.
func (c *Client) ForDate(ctx context.Context, date string) (Forecast, error) {
	day, err := time.ParseInLocation("2006-01-02", date, colombo)
	if err != nil {
		return Forecast{}, fmt.Errorf("invalid: date must be YYYY-MM-DD")
	}
	now := c.now().In(colombo)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, colombo)
	out := Forecast{Date: date, Source: SourceName, SourceURL: "https://open-meteo.com"}
	if day.Before(today.AddDate(0, 0, -pastDaysAllow)) || day.After(today.AddDate(0, 0, aheadDaysAllow)) {
		out.Error = fmt.Sprintf("weather is available from %d days ago to %d days ahead", pastDaysAllow, aheadDaysAllow)
		out.Districts = bare()
		return out, nil
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		out.Error = "weather lookup is turned off"
		out.Districts = bare()
		return out, nil
	}
	c.mu.Lock()
	cached, ok := c.cache[date]
	c.mu.Unlock()
	if ok && cached.FetchedAt != nil && now.Sub(*cached.FetchedAt) < cacheFor {
		return cached, nil
	}
	districts, err := c.fetch(ctx, date)
	if err != nil {
		out.Error = "weather forecast is unavailable right now"
		out.Districts = bare()
		return out, nil
	}
	fetched := now
	out.FetchedAt, out.Districts = &fetched, districts
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]Forecast{}
	}
	c.cache[date] = out
	c.mu.Unlock()
	return out, nil
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func bare() []DistrictForecast {
	out := make([]DistrictForecast, len(Districts))
	for i, d := range Districts {
		out[i] = DistrictForecast{District: d.Name, Latitude: d.Latitude, Longitude: d.Longitude}
	}
	return out
}

type hourly struct {
	Time          []string   `json:"time"`
	Precipitation []*float64 `json:"precipitation"`
	Probability   []*int     `json:"precipitation_probability"`
	Gusts         []*float64 `json:"wind_gusts_10m"`
}

type location struct {
	Hourly hourly `json:"hourly"`
}

func (c *Client) fetch(ctx context.Context, date string) ([]DistrictForecast, error) {
	lats, lngs := make([]string, len(Districts)), make([]string, len(Districts))
	for i, d := range Districts {
		lats[i], lngs[i] = fmt.Sprintf("%.4f", d.Latitude), fmt.Sprintf("%.4f", d.Longitude)
	}
	q := url.Values{
		"latitude": {strings.Join(lats, ",")}, "longitude": {strings.Join(lngs, ",")},
		"hourly":   {"precipitation,precipitation_probability,wind_gusts_10m"},
		"timezone": {"Asia/Colombo"}, "start_date": {date}, "end_date": {date},
	}
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "?")+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned %d", res.StatusCode)
	}
	var raw json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, err
	}
	var locations []location
	// One location comes back as an object, several as an array in request order.
	if err := json.Unmarshal(raw, &locations); err != nil {
		var one location
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		locations = []location{one}
	}
	if len(locations) != len(Districts) {
		return nil, fmt.Errorf("open-meteo returned %d locations for %d districts", len(locations), len(Districts))
	}
	out := bare()
	for i := range out {
		summarise(&out[i], locations[i].Hourly)
	}
	return out, nil
}

// summarise fills a district's figures from its hourly forecast and decides whether to suggest a risk.
func summarise(d *DistrictForecast, h hourly) {
	var day, working, maxHour, maxGust float64
	maxProb, peak, seen := -1, -1, false
	for i, at := range h.Time {
		hour := -1
		if len(at) >= 13 {
			fmt.Sscanf(at[11:13], "%d", &hour)
		}
		if i < len(h.Precipitation) && h.Precipitation[i] != nil {
			mm := *h.Precipitation[i]
			seen = true
			day += mm
			if hour >= workStartHour && hour < workEndHour {
				working += mm
			}
			if mm > maxHour {
				maxHour, peak = mm, hour
			}
		}
		if i < len(h.Probability) && h.Probability[i] != nil && *h.Probability[i] > maxProb {
			maxProb = *h.Probability[i]
		}
		if i < len(h.Gusts) && h.Gusts[i] != nil && *h.Gusts[i] > maxGust {
			maxGust = *h.Gusts[i]
		}
	}
	if !seen {
		return
	}
	day, working, maxHour = round1(day), round1(working), round1(maxHour)
	d.DayRainMm, d.WorkingRainMm, d.MaxHourlyRainMm = &day, &working, &maxHour
	if peak >= 0 && maxHour > 0 {
		d.PeakHour = fmt.Sprintf("%02d:00", peak)
	}
	if maxProb >= 0 {
		d.RainProbability = &maxProb
	}
	if len(h.Gusts) > 0 {
		gust := round1(maxGust)
		d.MaxGustKmh = &gust
	}
	d.Suggestion = suggest(d.District, day, maxHour, d.PeakHour, maxProb)
}

func suggest(district string, day, maxHour float64, peak string, prob int) *Suggestion {
	var riskType, severity string
	switch {
	case day >= FloodingDayMm:
		riskType, severity = "FLOODING", "HIGH"
	case day >= HighDayMm || maxHour >= HighHourMm:
		riskType, severity = "HEAVY_RAIN", "HIGH"
	case day >= MediumDayMm || maxHour >= MediumHourMm:
		riskType, severity = "HEAVY_RAIN", "MEDIUM"
	default:
		return nil
	}
	summary := fmt.Sprintf("%s: %.0f mm of rain forecast for the day", district, day)
	if peak != "" {
		summary += fmt.Sprintf(", heaviest around %s (%.1f mm in the hour)", peak, maxHour)
	}
	if riskType == "FLOODING" {
		summary += "; low roads may flood"
	}
	confidence := 0.5
	if prob >= 0 {
		confidence = math.Round(float64(prob)) / 100
	}
	return &Suggestion{RiskType: riskType, Severity: severity, Summary: summary + ".", Confidence: confidence}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
