package tunnel

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var DefaultUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 64,
	WriteBufferSize: 1024 * 64,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Conn wraps a websocket with a write mutex.
type Conn struct {
	WS *websocket.Conn
	mu sync.Mutex
}

func Wrap(ws *websocket.Conn) *Conn {
	ws.SetReadLimit(1 << 20)
	_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	return &Conn{WS: ws}
}

func (c *Conn) WriteText(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.WS.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.WS.WriteMessage(websocket.TextMessage, b)
}

func (c *Conn) WriteBinary(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.WS.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.WS.WriteMessage(websocket.BinaryMessage, b)
}

func (c *Conn) WriteControlPing() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.WS.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.WS.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(10*time.Second))
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.WS.Close()
}

func Dial(urlStr string, header http.Header) (*Conn, *http.Response, error) {
	d := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}
	ws, resp, err := d.Dial(urlStr, header)
	if err != nil {
		return nil, resp, err
	}
	return Wrap(ws), resp, nil
}
