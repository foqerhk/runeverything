package p2p

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeRelayURL(t *testing.T) {
	got, err := NormalizeRelayURL("wss://a.example")
	if err != nil || got != "wss://a.example/re2" {
		t.Fatalf("got %q err=%v", got, err)
	}
	got, err = NormalizeRelayURL("wss://a.example/ws")
	if err != nil || got != "wss://a.example/re2" {
		t.Fatalf("legacy /ws got %q err=%v", got, err)
	}
}

func TestPeerStoreUpsertList(t *testing.T) {
	s := NewStore(DefaultPeerTTL, true)
	s.SetSelf("ws://127.0.0.1:8787/re2")
	if err := s.Upsert(Peer{URL: "ws://127.0.0.1:8788/re2", LastSeen: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	snap := s.SnapshotForGossip(true)
	if snap.Self != "ws://127.0.0.1:8787/re2" || len(snap.Peers) != 1 {
		t.Fatalf("%+v", snap)
	}
}

func TestSelectLowestPingEmpty(t *testing.T) {
	_, _, err := SelectLowestPing(context.Background(), nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
}
