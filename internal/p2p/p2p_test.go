package p2p

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeRelayURL(t *testing.T) {
	got, err := NormalizeRelayURL("wss://a.example")
	if err != nil || got != "wss://a.example/ws" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestPeerStoreUpsertList(t *testing.T) {
	s := NewStore(DefaultPeerTTL, true)
	s.SetSelf("ws://127.0.0.1:8787/ws")
	if err := s.Upsert(Peer{URL: "ws://127.0.0.1:8788/ws", LastSeen: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	snap := s.SnapshotForGossip(true)
	if snap.Self != "ws://127.0.0.1:8787/ws" || len(snap.Peers) != 1 {
		t.Fatalf("%+v", snap)
	}
}

func TestSelectLowestPingEmpty(t *testing.T) {
	_, _, err := SelectLowestPing(context.Background(), nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
}
