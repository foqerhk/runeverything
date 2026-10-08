package p2p

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/netutil"
)

// ResolveForAgent picks relay URLs for a behind-NAT agent using P2P crawl.
// Callers should skip this when netutil.DirectPublicIP reports a non-NAT host.
func ResolveForAgent(ctx context.Context, currentRelay, currentPublic string, autoDiscover bool) (relayURL, publicURL string, discovered bool) {
	primary, _, alts, ok := ResolveForAgentMulti(ctx, currentRelay, currentPublic, autoDiscover, 2)
	_ = alts
	return primary, primary, ok
}

// ResolveForAgentMulti returns primary relay plus up to maxN-1 alternates (best-first).
// Alternates may also come from RE_RELAY_SECONDARY / config secondary when set.
func ResolveForAgentMulti(ctx context.Context, currentRelay, currentPublic string, autoDiscover bool, maxN int) (primary, public string, alts []string, discovered bool) {
	if maxN < 1 {
		maxN = 1
	}
	relayURL := strings.TrimSpace(currentRelay)
	publicURL := strings.TrimSpace(currentPublic)
	if publicURL == "" {
		publicURL = relayURL
	}
	manualSecondary := strings.TrimSpace(os.Getenv("RE_RELAY_SECONDARY"))

	if !autoDiscover {
		alts = dedupeRelays(nil, manualSecondary, relayURL)
		return relayURL, publicURL, alts, false
	}

	sticky := ""
	if relayURL != "" && !urlIsLoopback(relayURL) {
		sticky = relayURL
	} else if publicURL != "" && !urlIsLoopback(publicURL) {
		sticky = publicURL
	}

	top, results, err := DiscoverTop(ctx, sticky, maxN)
	if err != nil || len(top) == 0 {
		if sticky != "" {
			i18n.Log("log.p2p_keep_sticky", sticky, err)
			alts = dedupeRelays(nil, manualSecondary, sticky)
			return sticky, sticky, alts, false
		}
		i18n.Log("log.p2p_discover_failed", err, relayURL)
		alts = dedupeRelays(nil, manualSecondary, relayURL)
		return relayURL, publicURL, alts, false
	}
	healthy := 0
	for _, r := range results {
		if r.Err == nil {
			healthy++
		}
	}
	primary = top[0]
	i18n.Log("log.p2p_selected", primary, healthy, len(results))
	alts = dedupeRelays(top[1:], manualSecondary, primary)
	return primary, primary, alts, true
}

func dedupeRelays(extra []string, manual, primary string) []string {
	seen := map[string]bool{}
	if primary != "" {
		seen[primary] = true
	}
	var out []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	add(manual)
	for _, u := range extra {
		add(u)
	}
	return out
}

func urlIsLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	return netutil.IsLoopbackHost(u.Hostname())
}
