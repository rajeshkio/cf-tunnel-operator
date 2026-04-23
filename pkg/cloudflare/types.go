package cloudflare

type TunnelRule struct {
	Hostname string `json:"hostname,omitempty"`
	Service  string `json:"service"`
}
type TunnelConfig struct {
	Rules []TunnelRule `json:"ingress"`
}
