package p2p

import (
	"context"
	"net/url"
	"strings"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/netutil"
)

// ResolveForAgent picks relay URLs for a behind-NAT agent using P2P crawl.
// Callers should skip this when netutil.DirectPublicIP reports a non-NAT host.
func ResolveForAgent(ctx context.Context, currentRelay, currentPublic string, autoDiscover bool) (relayURL, publicURL string, discovered bool) {
	relayURL = strings.TrimSpace(currentRelay)
	publicURL = strings.TrimSpace(currentPublic)
	if publicURL == "" {
		publicURL = relayURL
	}
	if !autoDiscover {
		return relayURL, publicURL, false
	}

	sticky := ""
	if relayURL != "" && !urlIsLoopback(relayURL) {
		sticky = relayURL
	} else if publicURL != "" && !urlIsLoopback(publicURL) {
		sticky = publicURL
	}

	best, results, err := DiscoverBest(ctx, sticky)
	if err != nil {
		if sticky != "" {
			i18n.Log("log.p2p_keep_sticky", sticky, err)
			return sticky, sticky, false
		}
		i18n.Log("log.p2p_discover_failed", err, relayURL)
		return relayURL, publicURL, false
	}
	healthy := 0
	for _, r := range results {
		if r.Err == nil {
			healthy++
		}
	}
	i18n.Log("log.p2p_selected", best, healthy, len(results))
	return best, best, true
}

func urlIsLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	return netutil.IsLoopbackHost(u.Hostname())
}
