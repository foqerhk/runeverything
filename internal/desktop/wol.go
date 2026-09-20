package desktop

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// WakeOnLAN sends a magic packet to macAddr (AA:BB:CC:DD:EE:FF).
func WakeOnLAN(macAddr, broadcast string) error {
	mac, err := net.ParseMAC(strings.TrimSpace(macAddr))
	if err != nil {
		return fmt.Errorf("wol: bad mac: %w", err)
	}
	if len(mac) != 6 {
		return fmt.Errorf("wol: mac must be 6 bytes")
	}
	pkt := make([]byte, 6+16*6)
	for i := 0; i < 6; i++ {
		pkt[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		copy(pkt[6+i*6:], mac)
	}
	if broadcast == "" {
		broadcast = "255.255.255.255:9"
	}
	if !strings.Contains(broadcast, ":") {
		broadcast += ":9"
	}
	addr, err := net.ResolveUDPAddr("udp", broadcast)
	if err != nil {
		return err
	}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = c.Write(pkt)
	return err
}
