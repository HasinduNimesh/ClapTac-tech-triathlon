package cutoff

import (
	"time"

	_ "time/tzdata"
)

const Zone = "Asia/Colombo"

type Day struct {
	Date        time.Time
	IsOperating bool
}

type Calendar struct {
	days []Day
	loc  *time.Location
}

func Load(days []Day) *Calendar {
	loc, err := time.LoadLocation(Zone)
	if err != nil {
		loc = time.UTC
	}
	return &Calendar{days: days, loc: loc}
}

func (c *Calendar) locOrUTC() *time.Location {
	if c == nil || c.loc == nil {
		loc, _ := time.LoadLocation(Zone)
		if loc == nil {
			return time.UTC
		}
		return loc
	}
	return c.loc
}

// Adjust moves a requested date to the next operating run when the 4 PM
// Colombo cutoff has passed, or when the requested date is not operating.
func (c *Calendar) Adjust(requested time.Time, now time.Time) time.Time {
	return c.AdjustWithCutoff(requested, now, "16:00")
}

func (c *Calendar) AdjustWithCutoff(requested time.Time, now time.Time, cutoffLocalTime string) time.Time {
	loc := c.locOrUTC()
	now = now.In(loc)
	requested = dateOnly(requested, loc)
	parsed, err := time.Parse("15:04", cutoffLocalTime)
	if err != nil {
		parsed = time.Date(0, 1, 1, 16, 0, 0, 0, time.UTC)
	}
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)

	start := requested
	if !now.Before(cutoff) && !requested.After(dateOnly(now, loc)) {
		start = dateOnly(now, loc).AddDate(0, 0, 1)
	}
	if c == nil {
		return start
	}
	for _, d := range c.days {
		day := dateOnly(d.Date, loc)
		if day.Before(start) {
			continue
		}
		if d.IsOperating {
			return day
		}
	}
	return start
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
