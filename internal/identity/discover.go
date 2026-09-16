package identity

import (
	"os"
	"strings"

	"github.com/foqerhk/runeverything/internal/netutil"
)

// RelayExplicitlySet is true when the user pinned a relay via env.
func RelayExplicitlySet() bool {
	return strings.TrimSpace(os.Getenv("RE_RELAY")) != ""
}

// ShouldAutoDiscover reports whether Agent should pick a volunteer relay.
func ShouldAutoDiscover(cfg *Config, relayFlag string) bool {
	if strings.TrimSpace(relayFlag) != "" {
		return false
	}
	if RelayExplicitlySet() {
		return false
	}
	if cfg != nil && cfg.RelayManual {
		return false
	}
	return true
}

// HostIsLoopbackRelay reports whether a relay URL points at loopback.
func HostIsLoopbackRelay(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	u := raw
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if j := strings.IndexAny(u, "/?"); j >= 0 {
		u = u[:j]
	}
	return netutil.IsLoopbackHost(u)
}
