package main

import (
	"testing"

	"github.com/foqerhk/runeverything/internal/re2"
)

func TestBindChannelsExclusivePerController(t *testing.T) {
	h := &Hub{devices: map[string]*deviceState{}}
	const dev = "dev1"
	aDesk, aData, bData := &re2.Conn{}, &re2.Conn{}, &re2.Conn{}

	if res := h.bindRE2Client(dev, aDesk, re2.BindPayload{ClientID: "A", ClientName: "phone A"}); res.busy || len(res.displaced) > 0 {
		t.Fatalf("A desktop bind: %+v", res)
	}
	if res := h.bindRE2Client(dev, aData, re2.BindPayload{ClientID: "A", Channel: re2.ChannelData}); res.busy || len(res.displaced) > 0 {
		t.Fatalf("A data bind must coexist with A desktop: %+v", res)
	}
	if h.re2ClientOf(dev) != aDesk || h.re2ClientOf(re2.ChannelRoute(dev, re2.ChannelData)) != aData {
		t.Fatal("routes do not resolve to A's channels")
	}

	res := h.bindRE2Client(dev, bData, re2.BindPayload{ClientID: "B", Channel: re2.ChannelData})
	if !res.busy || res.peer != "phone A" {
		t.Fatalf("B without force must be refused: %+v", res)
	}

	res = h.bindRE2Client(dev, bData, re2.BindPayload{ClientID: "B", ClientName: "phone B", Channel: re2.ChannelData, Force: true})
	if res.busy || len(res.displaced) != 2 {
		t.Fatalf("B force must displace both of A's channels: %+v", res)
	}
	if h.re2ClientOf(dev) != nil || h.re2ClientOf(re2.ChannelRoute(dev, re2.ChannelData)) != bData {
		t.Fatal("after takeover only B's data channel should be bound")
	}

	if _, route := h.clearRE2Client(dev, aDesk); route != "" {
		t.Fatalf("a displaced conn must not clear anything, got route %q", route)
	}
	if _, route := h.clearRE2Client(dev, bData); route != re2.ChannelRoute(dev, re2.ChannelData) {
		t.Fatalf("clear B data route = %q", route)
	}
	if d := h.devices[dev]; d.RE2ClientID != "" {
		t.Fatalf("controller identity should reset once no channel is bound, got %q", d.RE2ClientID)
	}
}
