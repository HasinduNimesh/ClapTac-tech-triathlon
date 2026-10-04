package domain

import "time"

// Location is the latest point for one active run. No route or other stop data is included.
type Location struct {
    TripID string `json:"tripId"`
    VehicleID string `json:"vehicleId"`
    Latitude float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
    Timestamp time.Time `json:"timestamp"`
}
