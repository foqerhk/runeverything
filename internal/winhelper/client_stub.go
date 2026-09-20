//go:build !windows

package winhelper

import (
	"fmt"
	"time"
)

// Client is a stub on non-Windows.
type Client struct{}

func Dial(time.Duration) (*Client, error) { return nil, fmt.Errorf("winhelper: windows only") }
func (c *Client) Close() error            { return nil }
func (c *Client) Call(Request) (Response, error) {
	return Response{}, fmt.Errorf("winhelper: windows only")
}
func Available() bool { return false }
