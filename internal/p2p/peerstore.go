package p2p

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

// Peer is one known relay endpoint (Bitcoin-style addr entry).
type Peer struct {
	URL      string    `json:"url"`
	UDP      string    `json:"udp,omitempty"` // host:port REUDP
	Region   string    `json:"region,omitempty"`
	Load     int       `json:"load,omitempty"`
	Version  string    `json:"version,omitempty"`
	LastSeen time.Time `json:"last_seen"`
}

// PeersResponse is GET /v1/peers.
type PeersResponse struct {
	Self  string `json:"self,omitempty"`
	Peers []Peer `json:"peers"`
}

// AnnouncePeers is POST /v1/peers (addr gossip push).
type AnnouncePeers struct {
	Self  string `json:"self,omitempty"`
	Peers []Peer `json:"peers"`
}

// Store is an in-memory peer table with TTL.
type Store struct {
	mu      sync.RWMutex
	peers   map[string]*Peer
	selfURL string
	ttl     time.Duration
	allowWS bool
}

func NewStore(ttl time.Duration, allowWS bool) *Store {
	if ttl <= 0 {
		ttl = DefaultPeerTTL
	}
	return &Store{
		peers:   make(map[string]*Peer),
		ttl:     ttl,
		allowWS: allowWS,
	}
}

func (s *Store) SetSelf(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if nu, err := NormalizeRelayURL(url); err == nil {
		s.selfURL = nu
	} else {
		s.selfURL = strings.TrimSpace(url)
	}
}

func (s *Store) Self() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selfURL
}

func (s *Store) Upsert(p Peer) error {
	nu, err := NormalizeRelayURL(p.URL)
	if err != nil {
		return err
	}
	u, _ := url.Parse(nu)
	if u.Scheme == "ws" && !s.allowWS {
		return errInvalidScheme
	}
	if s.selfURL != "" && nu == s.selfURL {
		return nil
	}
	if p.LastSeen.IsZero() {
		p.LastSeen = time.Now().UTC()
	}
	p.URL = nu
	s.mu.Lock()
	s.peers[nu] = &p
	s.mu.Unlock()
	return nil
}

func (s *Store) UpsertMany(peers []Peer) int {
	n := 0
	for _, p := range peers {
		if err := s.Upsert(p); err == nil {
			n++
		}
	}
	return n
}

func (s *Store) List() []Peer {
	s.expire()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		cp := *p
		out = append(out, cp)
	}
	return out
}

// SnapshotForGossip returns self + peers for addr exchange.
func (s *Store) SnapshotForGossip(includeSelf bool) PeersResponse {
	list := s.List()
	resp := PeersResponse{Peers: list}
	if includeSelf {
		resp.Self = s.Self()
	}
	return resp
}

func (s *Store) RandomURLs(limit int) []string {
	list := s.List()
	if limit <= 0 || limit > len(list) {
		limit = len(list)
	}
	// simple rotate by time for variety
	if len(list) == 0 {
		return nil
	}
	off := int(time.Now().UnixNano() % int64(len(list)))
	out := make([]string, 0, limit)
	for i := 0; i < len(list) && len(out) < limit; i++ {
		out = append(out, list[(off+i)%len(list)].URL)
	}
	return out
}

func (s *Store) expire() {
	cutoff := time.Now().UTC().Add(-s.ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, p := range s.peers {
		if p.LastSeen.Before(cutoff) {
			delete(s.peers, k)
		}
	}
}

// NormalizeRelayURL canonicalizes a relay WebSocket URL to path /re2.
func NormalizeRelayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return "", errInvalidScheme
	}
	if u.Host == "" {
		return "", errInvalidHost
	}
	// v0.1: RE2 only — empty, /, or legacy /ws → /re2
	switch u.Path {
	case "", "/", "/ws":
		u.Path = "/re2"
	default:
		if !strings.HasPrefix(u.Path, "/re2") {
			u.Path = "/re2"
		}
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

const (
	errInvalidScheme simpleError = "url scheme must be wss (or ws in dev)"
	errInvalidHost   simpleError = "url host required"
	errNoPeers       simpleError = "no peers discovered"
	errNoHealthy     simpleError = "no healthy relays"
)
