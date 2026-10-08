package p2p

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProbeCapacity fetches /v1/capacity busy score (0..100). Missing endpoint → busy=0.
func ProbeCapacity(ctx context.Context, relayURL string) (busy int, maxSessions, active int, err error) {
	u, err := url.Parse(relayURL)
	if err != nil {
		return 0, 0, 0, err
	}
	u.Path = "/v1/capacity"
	u.RawQuery = ""
	u.Scheme = "http"
	if strings.HasPrefix(relayURL, "wss") {
		u.Scheme = "https"
	} else if strings.HasPrefix(relayURL, "ws") {
		u.Scheme = "http"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, 0, 0, err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return 0, 0, 0, nil
	}
	var body struct {
		Busy        int `json:"busy"`
		MaxSessions int `json:"max_sessions"`
		Active      int `json:"active"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	return body.Busy, body.MaxSessions, body.Active, nil
}

// ScoredRelay is a candidate with latency + load.
type ScoredRelay struct {
	URL   string
	RTT   time.Duration
	Busy  int // 0..100
	Score float64
}

// score = RTT_ms + busy*2 (busy heavily weighted so overloaded nodes lose).
func scoreRelay(rtt time.Duration, busy int) float64 {
	ms := float64(rtt.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	if busy < 0 {
		busy = 0
	}
	if busy > 100 {
		busy = 100
	}
	return ms + float64(busy)*2.5
}

// SelectBestLatencyAndLoad picks relay minimizing latency+busy.
func SelectBestLatencyAndLoad(ctx context.Context, candidates []string, stickyPreferred string) (string, []ScoredRelay, error) {
	if len(candidates) == 0 {
		return "", nil, errNoPeers
	}
	scored := make([]ScoredRelay, len(candidates))
	var wg sync.WaitGroup
	for i, u := range candidates {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			rtt, err := ProbeRelay(pctx, u)
			if err != nil {
				scored[i] = ScoredRelay{URL: u, Score: 1e12}
				return
			}
			busy, _, _, _ := ProbeCapacity(pctx, u)
			scored[i] = ScoredRelay{URL: u, RTT: rtt, Busy: busy, Score: scoreRelay(rtt, busy)}
		}(i, u)
	}
	wg.Wait()

	var ok []ScoredRelay
	for _, s := range scored {
		if s.Score < 1e11 {
			ok = append(ok, s)
		}
	}
	if len(ok) == 0 {
		return "", scored, errNoHealthy
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].Score < ok[j].Score })
	best := ok[0]
	if sticky := strings.TrimSpace(stickyPreferred); sticky != "" {
		if nu, err := NormalizeRelayURL(sticky); err == nil {
			for _, s := range ok {
				if s.URL == nu && s.Score <= best.Score*1.8 {
					return nu, scored, nil
				}
			}
		}
	}
	return best.URL, scored, nil
}

// DiscoverBest is seeds → crawl → latency+busy scoring.
func DiscoverBest(ctx context.Context, stickyPreferred string) (string, []ProbeResult, error) {
	urls, results, err := DiscoverTop(ctx, stickyPreferred, 1)
	if err != nil {
		return "", results, err
	}
	if len(urls) == 0 {
		return "", results, errNoHealthy
	}
	return urls[0], results, nil
}

// DiscoverTop returns up to n healthy relays sorted by latency+busy (best first).
func DiscoverTop(ctx context.Context, stickyPreferred string, n int) ([]string, []ProbeResult, error) {
	if n < 1 {
		n = 1
	}
	cands, err := Crawl(ctx, stickyPreferred)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("p2p crawl: %d candidates", len(cands))
	best, scored, err := SelectBestLatencyAndLoad(ctx, cands, stickyPreferred)
	if err != nil {
		u, results, err2 := DiscoverBestPing(ctx, stickyPreferred)
		if err2 != nil {
			return nil, results, err2
		}
		return []string{u}, results, nil
	}
	results := make([]ProbeResult, 0, len(scored))
	var healthy []ScoredRelay
	for _, s := range scored {
		pr := ProbeResult{URL: s.URL, RTT: s.RTT}
		if s.Score >= 1e11 {
			pr.Err = errNoHealthy
		} else {
			healthy = append(healthy, s)
		}
		results = append(results, pr)
	}
	sort.SliceStable(healthy, func(i, j int) bool { return healthy[i].Score < healthy[j].Score })
	out := make([]string, 0, n)
	seen := map[string]bool{}
	if best != "" {
		out = append(out, best)
		seen[best] = true
	}
	for _, s := range healthy {
		if len(out) >= n {
			break
		}
		if seen[s.URL] {
			continue
		}
		out = append(out, s.URL)
		seen[s.URL] = true
	}
	log.Printf("p2p: selected %v (latency+busy top-%d)", out, n)
	return out, results, nil
}
