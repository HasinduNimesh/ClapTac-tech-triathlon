package domain

import "time"

// Location is the latest point for one active run. No route or other stop data is included.
type Location struct {
	TripID    string    `json:"tripId"`
	VehicleID string    `json:"vehicleId"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
	// DistanceM is how far the vehicle has driven on this run, in metres, from the phone's position reports.
	DistanceM float64 `json:"distanceM"`
	// Fixes is how many position reports were accepted.
	Fixes int `json:"fixes"`
}

// RunDistance is the distance driven on one run, kept after the run ends.
type RunDistance struct {
	TripID         string    `json:"tripId"`
	VehicleID      string    `json:"vehicleId"`
	DistanceM      float64   `json:"distanceM"`
	Fixes          int       `json:"fixes"`
	LastRecordedAt time.Time `json:"lastRecordedAt"`
	RunStatus      string    `json:"runStatus"`
}
