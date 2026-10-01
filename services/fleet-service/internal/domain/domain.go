package domain

import "encoding/json"

type VehicleIncident struct {
	ID            string   `json:"id"`
	VehicleID     string   `json:"vehicleId"`
	Date          string   `json:"date"`
	TripID        string   `json:"tripId,omitempty"`
	Type          string   `json:"type"`
	Description   string   `json:"description"`
	AffectedStops []string `json:"affectedStops"`
	Status        string   `json:"status"`
	ReportedBy    string   `json:"reportedBy"`
	ReportedAt    string   `json:"reportedAt"`
}

type Vehicle struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	Temp             string  `json:"temp"`
	WeightCapacityKg float64 `json:"weightCapacityKg"`
	VolumeCapacityM3 float64 `json:"volumeCapacityM3"`
	FuelType         string  `json:"fuelType"`
	KmPerL           float64 `json:"kmPerL"`
	WeeklyFuelQuotaL float64 `json:"weeklyFuelQuotaL"`
	HomeDepot        string  `json:"homeDepot"`
	Version          int     `json:"version"`
}

type Availability struct {
	Date      string `json:"date"`
	VehicleID string `json:"vehicleId"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

type FuelEntry struct {
	ID         string  `json:"id"`
	VehicleID  string  `json:"vehicleId"`
	Date       string  `json:"date"`
	Liters     float64 `json:"liters"`
	ReceiptRef string  `json:"receiptRef,omitempty"`
	Note       string  `json:"note,omitempty"`
	RecordedBy string  `json:"recordedBy"`
	CreatedAt  string  `json:"createdAt"`
}

type FuelLedgerItem struct {
	VehicleID     string  `json:"vehicleId"`
	WeeklyQuotaL  float64 `json:"weeklyQuotaL"`
	ActualLitersL float64 `json:"actualLitersL"`
}

type FuelLedger struct {
	WeekOf    string           `json:"weekOf"`
	WeekStart string           `json:"weekStart"`
	WeekEnd   string           `json:"weekEnd"`
	Items     []FuelLedgerItem `json:"items"`
	Entries   []FuelEntry      `json:"entries"`
}

type AuditOutbox struct {
	EventID string
	Payload json.RawMessage
}
