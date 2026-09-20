package holepunch

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const prefix = "REHP1|"

// ServeEcho listens and replies to hole-punch pings (run on both sides during punch).
func ServeEcho(conn *net.UDPConn, stop <-chan struct{}) {
	buf := make([]byte, 256)
	for {
		select {
		case <-stop:
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if n >= len(prefix) && strings.HasPrefix(string(buf[:n]), prefix) {
			_, _ = conn.WriteToUDP([]byte(prefix+"pong"), addr)
		}
	}
}

// TryDirect attempts a brief UDP exchange with peer host:port.
func TryDirect(localPort int, peerHostPort string, token string, timeout time.Duration) (*net.UDPConn, error) {
	raddr, err := net.ResolveUDPAddr("udp", peerHostPort)
	if err != nil {
		return nil, err
	}
	var laddr *net.UDPAddr
	if localPort > 0 {
		laddr = &net.UDPAddr{Port: localPort}
	}
	c, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}
	stop := make(chan struct{})
	go ServeEcho(c, stop)
	defer close(stop)

	deadline := time.Now().Add(timeout)
	msg := []byte(prefix + token)
	for time.Now().Before(deadline) {
		_, _ = c.WriteToUDP(msg, raddr)
		_ = c.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		buf := make([]byte, 256)
		n, addr, err := c.ReadFromUDP(buf)
		if err == nil && n > 0 && strings.Contains(string(buf[:n]), "pong") {
			_ = c.SetReadDeadline(time.Time{})
			peer, err := net.DialUDP("udp", nil, addr)
			_ = c.Close()
			if err != nil {
				return nil, err
			}
			return peer, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = c.Close()
	return nil, fmt.Errorf("holepunch: no response from %s", peerHostPort)
}
