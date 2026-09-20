package reudp

import "encoding/json"

// NoiseTransport adapts Endpoint to re2.Transport (reliable DATA).
type NoiseTransport struct {
	EP *Endpoint
}

func (t *NoiseTransport) Send(msg []byte) error {
	return t.EP.SendReliable(msg)
}

func (t *NoiseTransport) Recv() ([]byte, error) {
	for {
		b, err := t.EP.Recv()
		if err != nil {
			return nil, err
		}
		// Skip assoc markers
		if len(b) > 0 && (b[0] == 0xFF || b[0] == 0xFE) {
			continue
		}
		return b, nil
	}
}

// MustJSON helper.
func MustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
