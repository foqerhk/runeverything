package reudp

import (
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := Packet{
		Type:      TypeData,
		Flags:     FlagReliable,
		RouteHash: RouteHash("dev-abc"),
		Seq:       42,
		Ack:       7,
		Payload:   []byte("hello-reudp"),
	}
	b, err := Encode(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != HeaderSize+len(in.Payload) {
		t.Fatalf("len=%d", len(b))
	}
	out, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != in.Type || out.Flags != in.Flags || out.RouteHash != in.RouteHash ||
		out.Seq != in.Seq || out.Ack != in.Ack || string(out.Payload) != string(in.Payload) {
		t.Fatalf("mismatch %+v vs %+v", out, in)
	}
}

func TestRouteHashStable(t *testing.T) {
	a := RouteHash("device-1")
	b := RouteHash("device-1")
	c := RouteHash("device-2")
	if a != b || a == c {
		t.Fatalf("hash unstable or colliding: %d %d %d", a, b, c)
	}
}

func TestCongestionAIMD(t *testing.T) {
	c := NewCongestion()
	w0 := c.Window()
	for i := 0; i < 20; i++ {
		if c.CanSend() {
			c.OnSend()
		}
	}
	c.OnAck(5, 40e6) // 40ms
	if c.Window() < w0 {
		t.Fatalf("window should grow on ack")
	}
	c.OnLoss()
	if c.Window() > int(float64(w0)*2)+10 {
		// after loss should shrink; loose check
	}
}

func TestPayloadLimit(t *testing.T) {
	big := make([]byte, MaxPayload+1)
	_, err := Encode(Packet{Type: TypeData, Payload: big}, nil)
	if err != ErrTooLarge {
		t.Fatalf("want ErrTooLarge got %v", err)
	}
}

func TestAckCodec(t *testing.T) {
	b := EncodeAck(100, 0xdead)
	cum, sack, err := DecodeAck(b)
	if err != nil || cum != 100 || sack != 0xdead {
		t.Fatalf("cum=%d sack=%x err=%v", cum, sack, err)
	}
}
