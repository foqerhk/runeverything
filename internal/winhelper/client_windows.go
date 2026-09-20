//go:build windows

package winhelper

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// Client talks to the LocalSystem helper over the named pipe.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to the helper pipe (short timeout).
func Dial(timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	conn, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, r: bufio.NewReaderSize(conn, 1<<20)}, nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) Call(req Request) (Response, error) {
	if c.conn == nil {
		return Response{}, fmt.Errorf("helper: closed")
	}
	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := c.conn.Write(append(b, '\n')); err != nil {
		return Response{}, err
	}
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "helper rejected"
		}
		return resp, fmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}

// Available reports whether the helper pipe is reachable.
func Available() bool {
	c, err := Dial(400 * time.Millisecond)
	if err != nil {
		return false
	}
	_, err = c.Call(Request{Op: "ping"})
	_ = c.Close()
	return err == nil
}
