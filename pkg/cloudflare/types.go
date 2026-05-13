package cloudflare

type OriginRequest struct {
	NoTLSVerify bool `json:"noTLSVerify,omitempty"`
}

type TunnelRule struct {
	Hostname      string        `json:"hostname,omitempty"`
	Service       string        `json:"service"`
	OriginRequest OriginRequest `json:"originRequest,omitempty"`
}
type TunnelConfig struct {
	Rules []TunnelRule `json:"ingress"`
}
type DNSRecords struct {
	Name    string `json:"name"`
	Id      string `json:"id"`
	Type    string `json:"type"`
	TTL     int    `json:"ttl"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

type DNSRecordRequests struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	TTL     int    `json:"ttl"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}
