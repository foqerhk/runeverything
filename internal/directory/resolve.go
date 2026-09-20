package directory

import (
	"context"

	"github.com/foqerhk/runeverything/internal/p2p"
)

// ResolveForAgent delegates to P2P seed crawl + lowest-ping selection.
func ResolveForAgent(ctx context.Context, currentRelay, currentPublic string, autoDiscover bool) (relayURL, publicURL string, discovered bool) {
	return p2p.ResolveForAgent(ctx, currentRelay, currentPublic, autoDiscover)
}

// SelectBest crawls from seeds (and optional sticky) then picks lowest ping.
func SelectBest(ctx context.Context, stickyPreferred string) (string, []p2p.ProbeResult, error) {
	return p2p.DiscoverBest(ctx, stickyPreferred)
}
