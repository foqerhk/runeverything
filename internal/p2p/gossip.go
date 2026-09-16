package p2p

import (
	"context"
	"log"
	"sync"
	"time"
)

// Gossiper periodically exchanges addr lists with known peers and seeds (Bitcoin-like).
type Gossiper struct {
	Store    *Store
	Share    bool // advertise self
	Region   string
	Version  string
	LoadFunc func() int
	Interval time.Duration

	stop chan struct{}
	once sync.Once
}

func (g *Gossiper) Start() {
	if g == nil || g.Store == nil {
		return
	}
	if g.Interval <= 0 {
		g.Interval = DefaultGossipInterval
	}
	g.stop = make(chan struct{})
	go g.loop()
}

func (g *Gossiper) Stop() {
	if g == nil || g.stop == nil {
		return
	}
	g.once.Do(func() { close(g.stop) })
}

func (g *Gossiper) loop() {
	g.tick()
	t := time.NewTicker(g.Interval)
	defer t.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-t.C:
			g.tick()
		}
	}
}

func (g *Gossiper) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Refresh seeds into store.
	if seeds, err := SeedURLs(ctx); err == nil {
		for _, s := range seeds {
			_ = g.Store.Upsert(Peer{URL: s, LastSeen: time.Now().UTC()})
		}
	}

	targets := g.Store.RandomURLs(8)
	if len(targets) == 0 {
		return
	}

	msg := g.outboundMsg()
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(peerURL string) {
			defer wg.Done()
			pctx, c := context.WithTimeout(ctx, 5*time.Second)
			defer c()
			// pull
			if resp, err := FetchPeers(pctx, peerURL); err == nil {
				if resp.Self != "" {
					_ = g.Store.Upsert(Peer{URL: resp.Self, LastSeen: time.Now().UTC()})
				}
				g.Store.UpsertMany(resp.Peers)
			}
			// push (only when sharing)
			if g.Share && msg.Self != "" {
				_ = PushPeers(pctx, peerURL, msg)
			}
		}(t)
	}
	wg.Wait()
	log.Printf("p2p gossip: peers=%d", len(g.Store.List()))
}

func (g *Gossiper) outboundMsg() AnnouncePeers {
	load := 0
	if g.LoadFunc != nil {
		load = g.LoadFunc()
	}
	snap := g.Store.SnapshotForGossip(g.Share)
	if g.Share && snap.Self != "" {
		// ensure self appears with fresh metadata in push
		selfPeer := Peer{
			URL:      snap.Self,
			Region:   g.Region,
			Load:     load,
			Version:  g.Version,
			LastSeen: time.Now().UTC(),
		}
		found := false
		for i := range snap.Peers {
			if snap.Peers[i].URL == snap.Self {
				snap.Peers[i] = selfPeer
				found = true
				break
			}
		}
		if !found {
			snap.Peers = append([]Peer{selfPeer}, snap.Peers...)
		}
	}
	return AnnouncePeers{Self: snap.Self, Peers: snap.Peers}
}
