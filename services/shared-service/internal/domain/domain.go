package domain

type Profile struct {
	Subject   string   `json:"subject"`
	Roles     []string `json:"roles"`
	OutletIDs []string `json:"outlet_ids,omitempty"`
	DriverID  string   `json:"driver_id,omitempty"`
}

type Notification struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	To      string `json:"to"`
	Body    string `json:"body"`
}
