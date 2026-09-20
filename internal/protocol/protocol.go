package protocol

import (
	"fmt"
	"net/url"
)

// Version is the shipping pairing / QR protocol version (RE2.1 + UDP desktop).
const Version = 3

// VersionRE2 is historical alias for RE2-era clients (v2); shipping is Version.
const VersionRE2 = 3

// Roles for WebSocket query ?role= on /re2.
const (
	RoleAgent  = "agent"
	RoleClient = "client"
)

// PairingPayload is encoded into the QR code / deep link (JSON).
type PairingPayload struct {
	V            int    `json:"v"`
	Relay        string `json:"relay"`
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
	Name         string `json:"name"`
	ExpiresAt    int64  `json:"expires_at"`
	NoisePub     string `json:"noise_pub,omitempty"`
	UDP          string `json:"udp,omitempty"` // host:port REUDP data plane
}

func (p PairingPayload) DeepLink() string {
	q := url.Values{}
	q.Set("v", fmt.Sprintf("%d", p.V))
	q.Set("relay", p.Relay)
	q.Set("device_id", p.DeviceID)
	q.Set("pairing_token", p.PairingToken)
	q.Set("name", p.Name)
	q.Set("expires_at", fmt.Sprintf("%d", p.ExpiresAt))
	q.Set("noise_pub", p.NoisePub)
	if p.UDP != "" {
		q.Set("udp", p.UDP)
	}
	return "koko://pair?" + q.Encode()
}
