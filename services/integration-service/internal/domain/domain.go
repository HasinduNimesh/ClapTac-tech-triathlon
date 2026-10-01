package domain

type DirectionsRequest struct {
	Origin      string   `json:"origin"`
	Destination string   `json:"destination"`
	Stops       []string `json:"stops"`
}

type PresignRequest struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type NotificationAdapter interface {
	Send(channel, to, body string) error
}

type ObjectStore interface {
	PresignPut(bucket, key string) (string, error)
}

type Router interface {
	Directions(req DirectionsRequest) (map[string]any, error)
}
