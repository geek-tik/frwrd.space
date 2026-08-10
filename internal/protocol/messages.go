package protocol

type ControlMessage struct {
	Type       string `json:"type"`
	LocalPort  int    `json:"local_port,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	TunnelID   string `json:"tunnel_id,omitempty"`
	Subdomain  string `json:"subdomain,omitempty"`
	PublicURL  string `json:"public_url,omitempty"`
	Message    string `json:"message,omitempty"`
}

const (
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypeError      = "error"
)
