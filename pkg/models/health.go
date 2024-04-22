package models

// Health represents the health status of a component.
type Health struct {
	Status        string `json:"status"`
	Name          string `json:"name"`
	Updated       string `json:"updated"`
	Documentation string `json:"documentation"`
}
