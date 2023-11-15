package models

type ResponsePayload struct {
    TotalItemCount int         `json:"total_item_count"`
    CurrentPage    int         `json:"current_page"`
    ItemLimit      int         `json:"item_limit"`
    TotalPages     int         `json:"total_pages"`
    Items          interface{} `json:"items"`
    NextPage       *string     `json:"next_page"`
    PrevPage       *string     `json:"prev_page"`
}

type Config struct {
    Database Database `yaml:"database" json:"database"`
    General  General  `yaml:"general" json:"general"`
}

type Database struct {
    File       string `yaml:"file" json:"file"`
    Connection string `yaml:"connection" json:"connection,omitempty"`
    Username   string `yaml:"username" json:"username,omitempty"`
    Password   string `yaml:"password" json:"password,omitempty"`
}

type General struct {
    Hostname   string   `yaml:"hostname" json:"hostname"`
	Schemes    []string `yaml:"schemes" json:"schemes"`
    ListenPort string   `yaml:"listen-port" json:"listen_port"`
    LogLevel   string   `yaml:"log-level" json:"log-level"`
}

type GoogleCreds struct {
    Web struct {
        ClientID                string   `yaml:"client_id" json:"client_id"`
        ProjectId               string   `yaml:"project_id" json:"project_id"`
        AuthUri                 string   `yaml:"auth_uri" json:"auth_uri"`
        TokenUri                string   `yaml:"token_uri" json:"token_uri"`
        AuthProviderX509CertUrl string   `yaml:"auth_provider_x509_cert_url" json:"auth_provider_x509_cert_url"`
        ClientSecret            string   `yaml:"client_secret" json:"client_secret"`
        RedirectUris            []string `yaml:"redirect_uris" json:"redirect_uris"`
        JavaScriptOrigins       []string `yaml:"javascript_origins" json:"javascript_origins"`
    } `yaml:"web" json:"web"`
}
