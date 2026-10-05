package domain

import "time"

// Location is copied from the authorized order's delivery tracking response.
type Location struct {
	TripID    string    `json:"tripId"`
	VehicleID string    `json:"vehicleId"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
}
