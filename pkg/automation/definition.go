// Package automation defines the closed, versioned vocabulary accepted by A4.
// No workflow may contain executable code, URLs or arbitrary service calls.
package automation

import (
	"errors"
	"fmt"
	"math"
	"time"
	_ "time/tzdata"
)

type Prefill struct {
	OutletID    string  `json:"outletId"`
	Units       int     `json:"orderUnits"`
	Weight      float64 `json:"orderWeightKg"`
	Volume      float64 `json:"orderVolumeM3"`
	Temperature string  `json:"temperatureRequirement"`
}

func (p Prefill) Valid() bool {
	return p.OutletID != "" && p.Units > 0 && p.Units <= 1000000 && p.Weight > 0 && p.Weight <= 1000000 && p.Volume > 0 && p.Volume <= 1000000 && !math.IsNaN(p.Weight) && !math.IsNaN(p.Volume) && (p.Temperature == "ambient" || p.Temperature == "chilled")
}

type Definition struct {
	Version  int      `json:"version"`
	Name     string   `json:"name"`
	Weekday  int      `json:"weekday"` // Sunday=0
	Time     string   `json:"time"`
	Timezone string   `json:"timezone"`
	Action   string   `json:"action"` // notify_deferrals, prefill_order, priority_review
	Prefill  *Prefill `json:"prefill,omitempty"`
}

func (d Definition) Validate(role string) error {
	if d.Version != 1 || len(d.Name) < 1 || len(d.Name) > 120 || d.Weekday < 0 || d.Weekday > 6 || d.Timezone != "Asia/Colombo" {
		return errors.New("invalid workflow name, version or weekly schedule")
	}
	if _, err := time.Parse("15:04", d.Time); err != nil {
		return errors.New("time must be HH:MM")
	}
	if role != "STORE_MANAGER" && role != "DISPATCHER" {
		return errors.New("automations are available to store managers and dispatchers")
	}
	switch d.Action {
	case "notify_deferrals":
		if d.Prefill != nil {
			return errors.New("unexpected prefill")
		}
	case "prefill_order":
		if role != "STORE_MANAGER" || d.Prefill == nil || !d.Prefill.Valid() {
			return errors.New("a valid store order template is required")
		}
	case "priority_review":
		if role != "DISPATCHER" || d.Prefill != nil {
			return errors.New("priority review is for dispatchers")
		}
	default:
		return errors.New("unsupported action; orders and plans cannot be submitted by a workflow")
	}
	return nil
}
func (d Definition) Next(after time.Time) time.Time {
	loc, _ := time.LoadLocation("Asia/Colombo")
	local := after.In(loc)
	var hour, minute int
	fmt.Sscanf(d.Time, "%d:%d", &hour, &minute)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	for candidate.Weekday() != time.Weekday(d.Weekday) || !candidate.After(after) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate.UTC()
}
func (d Definition) Previous(before time.Time) time.Time { return d.Next(before).AddDate(0, 0, -7) }

type Deferred struct {
	OrderID  string `json:"orderId"`
	OutletID string `json:"outletId"`
	Date     string `json:"date"`
	Count    int    `json:"count"`
}
type Snapshot struct {
	AsOf         time.Time  `json:"asOf"`
	HistorySince time.Time  `json:"historySince"`
	Items        []Deferred `json:"items"`
}
