package p2p

import (
	"context"
	"log"
	"net/url"
	"strings"

	"github.com/foqerhk/runeverything/internal/netutil"
)

// ResolveForAgent picks relay URLs for a home/NAT agent using P2P crawl.
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
			log.Printf("p2p: keep sticky relay %s (%v)", sticky, err)
			return sticky, sticky, false
		}
		log.Printf("p2p: discover failed (%v); keeping %s", err, relayURL)
		return relayURL, publicURL, false
	}
	healthy := 0
	for _, r := range results {
		if r.Err == nil {
			healthy++
		}
	}
	log.Printf("p2p: selected %s (%d/%d healthy)", best, healthy, len(results))
	return best, best, true
}

func urlIsLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	return netutil.IsLoopbackHost(u.Hostname())
}
