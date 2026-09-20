package directory

import "testing"

func TestNormalizeRelayURL(t *testing.T) {
	got, err := NormalizeRelayURL("wss://relay.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://relay.example.com/re2" {
		t.Fatalf("got %q", got)
	}
	got, err = NormalizeRelayURL("wss://relay.example.com/ws")
	if err != nil || got != "wss://relay.example.com/re2" {
		t.Fatalf("legacy /ws got %q err=%v", got, err)
	}
	if _, err := NormalizeRelayURL("http://bad"); err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestRegistryAnnounceAndExpire(t *testing.T) {
	r := NewRegistry(DefaultTTL, true)
	ent, err := r.Announce(AnnounceRequest{URL: "ws://127.0.0.1:8787/re2", Load: 2, Region: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if ent.URL != "ws://127.0.0.1:8787/re2" {
		t.Fatalf("url %q", ent.URL)
	}
	list := r.List()
	if len(list) != 1 || list[0].Load != 2 {
		t.Fatalf("list=%+v", list)
	}
}

func TestParseListBody(t *testing.T) {
	a, err := parseListBody([]byte(`{"relays":[{"url":"wss://a.example/re2","load":1}]}`))
	if err != nil || len(a) != 1 || a[0].URL != "wss://a.example/re2" {
		t.Fatalf("wrapped: %v %+v", err, a)
	}
	b, err := parseListBody([]byte(`[{"url":"wss://b.example/re2"}]`))
	if err != nil || len(b) != 1 {
		t.Fatalf("array: %v %+v", err, b)
	}
}
