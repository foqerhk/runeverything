package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/re2"
)

// A paired phone keeps deriving its Noise PSK from the pairing token it scanned, while
// the Agent rotates the QR token whenever it re-registers after expiry. Tokens that
// completed a handshake are remembered so rotation never orphans a paired phone.
const (
	maxPairedTokens = 16
	maxRecentTokens = 4
)

type pairedToken struct {
	Token  string `json:"token"`
	LastOK int64  `json:"last_ok"`
}

var pskTokens struct {
	mu     sync.Mutex
	loaded bool
	paired []pairedToken
	// Issued this run but not yet seen in a handshake (phone redeemed just before a rotation).
	recent []string
}

func pairedTokensPath() string {
	home, err := identity.HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "paired_tokens.json")
}

func loadPairedTokensLocked() {
	if pskTokens.loaded {
		return
	}
	pskTokens.loaded = true
	path := pairedTokensPath()
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &pskTokens.paired)
}

func savePairedTokensLocked() {
	path := pairedTokensPath()
	if path == "" {
		return
	}
	b, err := json.Marshal(pskTokens.paired)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// noteIssuedToken records a freshly minted QR token as a handshake candidate.
func noteIssuedToken(token string) {
	if token == "" {
		return
	}
	pskTokens.mu.Lock()
	defer pskTokens.mu.Unlock()
	for _, t := range pskTokens.recent {
		if t == token {
			return
		}
	}
	pskTokens.recent = append(pskTokens.recent, token)
	if n := len(pskTokens.recent); n > maxRecentTokens {
		pskTokens.recent = pskTokens.recent[n-maxRecentTokens:]
	}
}

// notePairedToken remembers a token that a phone just authenticated with.
func notePairedToken(token string) {
	if token == "" {
		return
	}
	pskTokens.mu.Lock()
	defer pskTokens.mu.Unlock()
	loadPairedTokensLocked()
	now := time.Now().Unix()
	found := false
	for i := range pskTokens.paired {
		if pskTokens.paired[i].Token == token {
			if now-pskTokens.paired[i].LastOK < 60 {
				return
			}
			pskTokens.paired[i].LastOK = now
			found = true
			break
		}
	}
	if !found {
		pskTokens.paired = append(pskTokens.paired, pairedToken{Token: token, LastOK: now})
	}
	sort.SliceStable(pskTokens.paired, func(i, j int) bool {
		return pskTokens.paired[i].LastOK > pskTokens.paired[j].LastOK
	})
	if len(pskTokens.paired) > maxPairedTokens {
		pskTokens.paired = pskTokens.paired[:maxPairedTokens]
	}
	savePairedTokensLocked()
}

// handshakeTokens lists candidate pairing tokens: the current one first, then tokens
// paired phones used (most recent first), then tokens issued earlier this run.
func (a *Agent) handshakeTokens() []string {
	pskTokens.mu.Lock()
	defer pskTokens.mu.Unlock()
	loadPairedTokensLocked()
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	add(a.pairingToken)
	for _, p := range pskTokens.paired {
		add(p.Token)
	}
	for i := len(pskTokens.recent) - 1; i >= 0; i-- {
		add(pskTokens.recent[i])
	}
	return out
}

// runAgentNoise answers a client Noise handshake with any known pairing token and
// returns the session, the client static key and the PSK that matched.
func (a *Agent) runAgentNoise(tr re2.Transport) (*re2.Session, []byte, []byte, error) {
	tokens := a.handshakeTokens()
	psks := make([][]byte, len(tokens))
	for i, t := range tokens {
		psks[i] = re2.DerivePSK(t)
	}
	hs, err := re2.NewAgentHandshakes(psks, a.noiseKP)
	if err != nil {
		return nil, nil, nil, err
	}
	sess, peer, idx, err := hs.RunAgent(tr)
	if err != nil {
		return nil, nil, nil, err
	}
	notePairedToken(tokens[idx])
	return sess, peer, psks[idx], nil
}
