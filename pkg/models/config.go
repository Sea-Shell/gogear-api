package models

type ResponsePayload struct {
	TotalItemCount int `json:"total_item_count"`
	CurrentPage int `json:"current_page"`
	ItemLimit int `json:"item_limit"`
	TotalPages int `json:"total_pages"`
	Items interface{} `json:"items"`
	NextPage *string `json:"next_page"`
	PrevPage *string `json:"prev_page"`
}

type Config struct {
	Database Database `yaml:"database" json:"database"`
	General General `yaml:"general" json:"general"`
}

type Database struct {
	File string `yaml:"file" json:"file"`
	Connection string `yaml:"connection" json:"connection,omitempty"`
	Username string `yaml:"username" json:"username,omitempty"`
	Password string `yaml:"password" json:"password,omitempty"`
}

type General struct {
	ListenPort 	string 	`yaml:"listen-port" json:"listen_port"`
	LogLevel 	string 	`yaml:"log-level" json:"log-level"`
}