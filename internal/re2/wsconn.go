package re2

import (
	"fmt"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// Conn is a RE2 outer-frame connection over a WebSocket (binary only).
type Conn struct {
	WS *websocket.Conn
	mu sync.Mutex
}

func WrapWS(ws *websocket.Conn) *Conn {
	return &Conn{WS: ws}
}

func (c *Conn) WriteFrame(f Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var buf writerBuffer
	if err := EncodeFrame(&buf, f); err != nil {
		return err
	}
	return c.WS.WriteMessage(websocket.BinaryMessage, buf.Bytes())
}

func (c *Conn) ReadFrame() (*Frame, error) {
	for {
		mt, data, err := c.WS.ReadMessage()
		if err != nil {
			return nil, err
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		br := byteReader(data)
		return DecodeFrame(&br)
	}
}

func (c *Conn) Close() error {
	return c.WS.Close()
}

type writerBuffer struct{ b []byte }

func (w *writerBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
func (w *writerBuffer) Bytes() []byte { return w.b }

type byteReader []byte

func (b *byteReader) Read(p []byte) (int, error) {
	if len(*b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, *b)
	*b = (*b)[n:]
	return n, nil
}

// FrameNoiseTransport uses TypeNoise frames on Conn for Handshake Transport.
// Only safe when the peer exclusively sends Noise handshake frames until done
// (relay must not interleave other types to this endpoint during handshake).
type FrameNoiseTransport struct {
	Conn    *Conn
	RouteID string
}

func (t *FrameNoiseTransport) Send(msg []byte) error {
	return t.Conn.WriteFrame(Frame{Type: TypeNoise, RouteID: t.RouteID, Payload: msg})
}

func (t *FrameNoiseTransport) Recv() ([]byte, error) {
	for {
		f, err := t.Conn.ReadFrame()
		if err != nil {
			return nil, err
		}
		if f.Type == TypeNoise {
			return f.Payload, nil
		}
		if f.Type == TypeError {
			return nil, fmt.Errorf("re2: peer error during handshake: %s", string(f.Payload))
		}
		// skip ping/pong
	}
}
