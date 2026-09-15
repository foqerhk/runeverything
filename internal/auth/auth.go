package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

type PairingEntry struct {
	Token     string
	DeviceID  string
	Name      string
	Relay     string
	ExpiresAt time.Time
}

type SessionEntry struct {
	Token    string
	DeviceID string
	Name     string
}

// Store holds short-lived pairing tokens and long-lived client session tokens (in-memory for MVP).
type Store struct {
	mu       sync.RWMutex
	pairing  map[string]PairingEntry // token -> entry
	sessions map[string]SessionEntry // token -> entry
}

func NewStore() *Store {
	return &Store{
		pairing:  make(map[string]PairingEntry),
		sessions: make(map[string]SessionEntry),
	}
}

func RandomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) PutPairing(deviceID, name, relay, token string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Invalidate previous tokens for this device.
	for k, v := range s.pairing {
		if v.DeviceID == deviceID {
			delete(s.pairing, k)
		}
	}
	s.pairing[token] = PairingEntry{
		Token:     token,
		DeviceID:  deviceID,
		Name:      name,
		Relay:     relay,
		ExpiresAt: time.Now().Add(ttl),
	}
}

func (s *Store) RedeemPairing(deviceID, token string) (PairAck, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ent, ok := s.pairing[token]
	if !ok || ent.DeviceID != deviceID {
		return PairAck{}, false
	}
	if time.Now().After(ent.ExpiresAt) {
		delete(s.pairing, token)
		return PairAck{}, false
	}
	delete(s.pairing, token)
	sess, err := RandomToken(24)
	if err != nil {
		return PairAck{}, false
	}
	s.sessions[sess] = SessionEntry{
		Token:    sess,
		DeviceID: deviceID,
		Name:     ent.Name,
	}
	return PairAck{
		DeviceID:     deviceID,
		SessionToken: sess,
		Name:         ent.Name,
		Relay:        ent.Relay,
	}, true
}

type PairAck struct {
	DeviceID     string
	SessionToken string
	Name         string
	Relay        string
}

func (s *Store) ValidateSession(deviceID, token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ent, ok := s.sessions[token]
	if !ok {
		return false
	}
	return ent.DeviceID == deviceID
}

func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
