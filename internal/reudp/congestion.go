package reudp

import (
	"sync"
	"time"
)

// Congestion is a minimal AIMD + pacing controller.
type Congestion struct {
	mu       sync.Mutex
	cwnd     float64 // packets
	ssthresh float64
	inFlight int
	minRTT   time.Duration
	lastSend time.Time
	paceGap  time.Duration
}

func NewCongestion() *Congestion {
	return &Congestion{
		cwnd:     8,
		ssthresh: 64,
		minRTT:   50 * time.Millisecond,
		paceGap:  2 * time.Millisecond,
	}
}

func (c *Congestion) CanSend() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight >= int(c.cwnd) {
		return false
	}
	now := time.Now()
	if !c.lastSend.IsZero() && now.Sub(c.lastSend) < c.paceGap {
		return false
	}
	return true
}

func (c *Congestion) OnSend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight++
	c.lastSend = time.Now()
}

func (c *Congestion) OnAck(n int, rtt time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n > c.inFlight {
		c.inFlight = 0
	} else {
		c.inFlight -= n
	}
	if rtt > 0 && (c.minRTT == 0 || rtt < c.minRTT) {
		c.minRTT = rtt
	}
	if c.cwnd < c.ssthresh {
		c.cwnd += float64(n) // slow start
	} else {
		c.cwnd += float64(n) / c.cwnd // congestion avoidance
	}
	if c.cwnd > 512 {
		c.cwnd = 512
	}
	// pace ~ RTT/cwnd
	if c.minRTT > 0 && c.cwnd > 0 {
		c.paceGap = time.Duration(float64(c.minRTT) / c.cwnd)
		if c.paceGap < time.Millisecond {
			c.paceGap = time.Millisecond
		}
		if c.paceGap > 20*time.Millisecond {
			c.paceGap = 20 * time.Millisecond
		}
	}
}

func (c *Congestion) OnLoss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ssthresh = c.cwnd / 2
	if c.ssthresh < 2 {
		c.ssthresh = 2
	}
	c.cwnd = c.ssthresh
	c.inFlight = 0
}

func (c *Congestion) Window() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(c.cwnd)
}
