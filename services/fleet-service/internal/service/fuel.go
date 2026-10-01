package service

import (
	"fmt"
	"time"
)

var colombo = time.FixedZone("Asia/Colombo", 5*60*60+30*60)

// ISOWeekRange returns the Monday-Sunday week containing an ISO local date.
func ISOWeekRange(weekOf string) (string, string, error) {
	day, err := time.ParseInLocation(time.DateOnly, weekOf, colombo)
	if err != nil {
		return "", "", fmt.Errorf("invalid weekOf date")
	}
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := day.AddDate(0, 0, -(weekday - 1))
	end := start.AddDate(0, 0, 6)
	return start.Format(time.DateOnly), end.Format(time.DateOnly), nil
}
