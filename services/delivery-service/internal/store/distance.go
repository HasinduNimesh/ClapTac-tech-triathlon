package store

import (
	"math"
	"time"
)

const (
	// A step shorter than this is GPS jitter at a standstill, not driving.
	minStepMetres = 15.0
	// A step implying more than this speed is a bad fix (a jump), not driving. 40 m/s is 144 km/h.
	maxSpeedMetresPerSecond = 40.0
	earthRadiusMetres       = 6371000.0
)

// haversine is the great-circle distance in metres between two points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusMetres * math.Asin(math.Min(1, math.Sqrt(a)))
}

// stepDistance is how many metres to add to a run's total when a new position follows the previous one. Jitter and
// impossible jumps add nothing, and a point that is not newer than the previous one adds nothing either.
func stepDistance(prevLat, prevLon float64, prevAt time.Time, lat, lon float64, at time.Time) float64 {
	seconds := at.Sub(prevAt).Seconds()
	if seconds <= 0 {
		return 0
	}
	metres := haversine(prevLat, prevLon, lat, lon)
	if metres < minStepMetres || metres/seconds > maxSpeedMetresPerSecond {
		return 0
	}
	return metres
}
