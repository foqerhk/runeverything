package p2p

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProbeResult is a candidate after ping.
type ProbeResult struct {
	URL string
	RTT time.Duration
	Err error
}

// Crawl discovers relays starting from official seeds, then following /v1/peers
// (Bitcoin-style addr crawl). Returns unique relay URLs.
func Crawl(ctx context.Context, stickyPreferred string) ([]string, error) {
	seeds, err := SeedURLs(ctx)
	if err != nil {
		// still allow sticky-only
		if stickyPreferred == "" {
			return nil, err
		}
		seeds = nil
	}

	type item struct {
		url   string
		depth int
	}
	queue := make([]item, 0, 32)
	seen := map[string]struct{}{}
	add := func(raw string, depth int) {
		nu, err := NormalizeRelayURL(raw)
		if err != nil {
			return
		}
		if _, ok := seen[nu]; ok {
			return
		}
		if len(seen) >= MaxCrawlPeers {
			return
		}
		seen[nu] = struct{}{}
		queue = append(queue, item{url: nu, depth: depth})
	}

	if sticky := strings.TrimSpace(stickyPreferred); sticky != "" {
		add(sticky, 0)
	}
	for _, s := range seeds {
		add(s, 0)
	}

	for qi := 0; qi < len(queue); qi++ {
		it := queue[qi]
		if it.depth >= MaxCrawlDepth {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		resp, err := FetchPeers(pctx, it.url)
		cancel()
		if err != nil {
			continue
		}
		if resp.Self != "" {
			add(resp.Self, it.depth+1)
		}
		for _, p := range resp.Peers {
			add(p.URL, it.depth+1)
		}
	}

	out := make([]string, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil, errNoPeers
	}
	return out, nil
}

// SelectLowestPing probes candidates and returns the lowest-RTT healthy relay.
// Sticky is preferred when within 2x of the best RTT.
func SelectLowestPing(ctx context.Context, candidates []string, stickyPreferred string) (string, []ProbeResult, error) {
	if len(candidates) == 0 {
		return "", nil, errNoPeers
	}
	results := make([]ProbeResult, len(candidates))
	var wg sync.WaitGroup
	for i, u := range candidates {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			rtt, err := ProbeRelay(pctx, u)
			results[i] = ProbeResult{URL: u, RTT: rtt, Err: err}
		}(i, u)
	}
	wg.Wait()

	var ok []ProbeResult
	for _, r := range results {
		if r.Err == nil {
			ok = append(ok, r)
		}
	}
	if len(ok) == 0 {
		return "", results, errNoHealthy
	}
	sort.SliceStable(ok, func(i, j int) bool {
		return ok[i].RTT < ok[j].RTT
	})
	best := ok[0]
	if sticky := strings.TrimSpace(stickyPreferred); sticky != "" {
		if nu, err := NormalizeRelayURL(sticky); err == nil {
			for _, r := range ok {
				if r.URL == nu && r.RTT <= best.RTT*2+50*time.Millisecond {
					return nu, results, nil
				}
			}
		}
	}
	return best.URL, results, nil
}

// DiscoverBestPing is ping-only selection (used as fallback).
func DiscoverBestPing(ctx context.Context, stickyPreferred string) (string, []ProbeResult, error) {
	cands, err := Crawl(ctx, stickyPreferred)
	if err != nil {
		return "", nil, err
	}
	log.Printf("p2p crawl: %d candidates", len(cands))
	return SelectLowestPing(ctx, cands, stickyPreferred)
}
