package models

type Health struct {
    Status        string `json:"status"`
    Name          string `json:"name"`
    Updated       string `json:"updated"`
    Documentation string `json:"documentation"`
}
