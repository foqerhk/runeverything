package reudp

import "testing"

func TestAckSentinelDoesNotAcknowledgeMissingZero(t *testing.T) {
	cum := ^uint32(0)
	sack := uint64(1) << 1 // seq 1 arrived, seq 0 is still missing.
	if ackedBy(cum, sack, 0) {
		t.Fatal("missing sequence zero was falsely acknowledged")
	}
	if !ackedBy(cum, sack, 1) {
		t.Fatal("selectively acknowledged sequence one was not recognized")
	}
}

func TestAckCumulativeAndSelective(t *testing.T) {
	const cum uint32 = 4
	const sack uint64 = uint64(1) << 2 // base=5, acknowledges seq 7.
	for seq := uint32(0); seq <= 4; seq++ {
		if !ackedBy(cum, sack, seq) {
			t.Fatalf("cumulative sequence %d was not acknowledged", seq)
		}
	}
	if ackedBy(cum, sack, 5) || ackedBy(cum, sack, 6) {
		t.Fatal("unreceived sequence was selectively acknowledged")
	}
	if !ackedBy(cum, sack, 7) {
		t.Fatal("selective sequence seven was not acknowledged")
	}
}
