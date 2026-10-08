package protocol

import (
	"fmt"
	"net/url"
	"strings"
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
	V            int      `json:"v"`
	Relay        string   `json:"relay"`
	DeviceID     string   `json:"device_id"`
	PairingToken string   `json:"pairing_token"`
	Name         string   `json:"name"`
	ExpiresAt    int64    `json:"expires_at"`
	NoisePub     string   `json:"noise_pub,omitempty"`
	UDP          string   `json:"udp,omitempty"` // host:port REUDP data plane (usually relay)
	LAN          []string `json:"lan,omitempty"` // host:port on local LAN for same-subnet P2P
	// Relays is an optional failover list (primary first). Same pairing token is
	// offered on each reachable node; App tries in order when one is full/down.
	Relays []RelayCandidate `json:"relays,omitempty"`
}

// RelayCandidate is an alternate public relay + UDP endpoint for pairing failover.
type RelayCandidate struct {
	Relay string `json:"relay"`
	UDP   string `json:"udp,omitempty"`
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
	if len(p.LAN) > 0 {
		q.Set("lan", strings.Join(p.LAN, ","))
	}
	if len(p.Relays) > 0 {
		parts := make([]string, 0, len(p.Relays))
		for _, r := range p.Relays {
			if r.Relay == "" {
				continue
			}
			if r.UDP != "" {
				parts = append(parts, r.Relay+"|"+r.UDP)
			} else {
				parts = append(parts, r.Relay)
			}
		}
		if len(parts) > 0 {
			q.Set("relays", strings.Join(parts, ","))
		}
	}
	return "koko://pair?" + q.Encode()
}

