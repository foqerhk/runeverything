package p2p

import (
	"os"
	"strings"
)

// SharingEnabled reports whether this host should advertise itself via gossip.
// Default true unless RE_SHARE_RELAY=0/false/off or cfg says otherwise.
func SharingEnabled(cfgShare *bool) bool {
	if v := os.Getenv("RE_SHARE_RELAY"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	if cfgShare != nil {
		return *cfgShare
	}
	return true
}
