package main

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/auth"
	"github.com/foqerhk/runeverything/internal/reudp"
)

type udpPeer struct {
	addr *net.UDPAddr
}

func (h *Hub) setUDPAgent(id string, addr *net.UDPAddr) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{ID: id}
		h.devices[id] = d
	}
	d.UDPAgent = addr
}

func (h *Hub) setUDPClient(id string, addr *net.UDPAddr) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{ID: id}
		h.devices[id] = d
	}
	d.UDPClient = addr
}

func (h *Hub) udpPeerOf(id string, from *net.UDPAddr) *net.UDPAddr {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[id]
	if d == nil {
		return nil
	}
	if d.UDPAgent != nil && sameUDP(d.UDPAgent, from) {
		return d.UDPClient
	}
	if d.UDPClient != nil && sameUDP(d.UDPClient, from) {
		return d.UDPAgent
	}
	return nil
}

func sameUDP(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return false
	}
	return a.IP.Equal(b.IP) && a.Port == b.Port
}

func (h *Hub) deviceByHash(hash uint32) *deviceState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, d := range h.devices {
		if reudp.RouteHash(d.ID) == hash {
			return d
		}
	}
	return nil
}

func (h *Hub) lookupDeviceID(hash uint32, hint string) string {
	if hint != "" {
		return hint
	}
	d := h.deviceByHash(hash)
	if d != nil {
		return d.ID
	}
	return ""
}

// rateLimiter limits ASSOC attempts per source IP.
type rateLimiter struct {
	mu sync.Mutex
	m  map[string]int
}

func newRateLimiter() *rateLimiter {
	r := &rateLimiter{m: make(map[string]int)}
	go func() {
		for range time.Tick(time.Minute) {
			r.mu.Lock()
			r.m = make(map[string]int)
			r.mu.Unlock()
		}
	}()
	return r
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[ip]++
	return r.m[ip] <= 30
}

func serveUDP(hub *Hub, conn *net.UDPConn, publicUDP string) {
	lim := newRateLimiter()
	buf := make([]byte, reudp.MaxPacket+64)
	log.Printf("reudp listening on %s public_udp=%s", conn.LocalAddr(), publicUDP)

	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("reudp read: %v", err)
			return
		}
		pkt, err := reudp.Decode(buf[:n])
		if err != nil {
			continue
		}
		switch pkt.Type {
		case reudp.TypeAssoc:
			if !lim.allow(addr.IP.String()) {
				continue
			}
			handleUDPAssoc(hub, conn, addr, pkt, publicUDP)
		case reudp.TypeData, reudp.TypeAck, reudp.TypePing, reudp.TypePong:
			handleUDPForward(hub, conn, addr, pkt, buf[:n])
		default:
			// ignore
		}
	}
}

func handleUDPAssoc(hub *Hub, conn *net.UDPConn, addr *net.UDPAddr, pkt *reudp.Packet, publicUDP string) {
	var req reudp.AssocPayload
	if err := json.Unmarshal(pkt.Payload, &req); err != nil {
		writeUDPErr(conn, addr, pkt.RouteHash, "bad_data", err.Error())
		return
	}
	if req.DeviceID == "" {
		writeUDPErr(conn, addr, pkt.RouteHash, "bad_data", "device_id required")
		return
	}
	switch req.Role {
	case reudp.RoleAgent:
		hub.mu.RLock()
		d := hub.devices[req.DeviceID]
		secret := ""
		if d != nil {
			secret = d.Secret
		}
		hub.mu.RUnlock()
		if secret == "" || !auth.ConstantTimeEqual(secret, req.DeviceSecret) {
			// Allow assoc if agent already registered on wss with matching secret stored.
			writeUDPErr(conn, addr, reudp.RouteHash(req.DeviceID), "auth_failed", "register on wss first")
			return
		}
		hub.setUDPAgent(req.DeviceID, cloneUDPAddr(addr))
		writeUDPAssocOK(conn, addr, req.DeviceID, publicUDP)
		log.Printf("reudp agent assoc device=%s from=%s", req.DeviceID, addr)

	case reudp.RoleClient:
		if !hub.auth.ValidateSession(req.DeviceID, req.SessionTicket) {
			writeUDPErr(conn, addr, reudp.RouteHash(req.DeviceID), "auth_failed", "invalid session ticket")
			return
		}
		hub.setUDPClient(req.DeviceID, cloneUDPAddr(addr))
		writeUDPAssocOK(conn, addr, req.DeviceID, publicUDP)
		log.Printf("reudp client assoc device=%s from=%s", req.DeviceID, addr)

	default:
		writeUDPErr(conn, addr, pkt.RouteHash, "bad_role", "role=agent|client")
	}
}

func handleUDPForward(hub *Hub, conn *net.UDPConn, from *net.UDPAddr, pkt *reudp.Packet, raw []byte) {
	id := hub.lookupDeviceID(pkt.RouteHash, "")
	if id == "" {
		return
	}
	peer := hub.udpPeerOf(id, from)
	if peer == nil {
		return
	}
	_, _ = conn.WriteToUDP(raw, peer)
}

func writeUDPAssocOK(conn *net.UDPConn, addr *net.UDPAddr, deviceID, publicUDP string) {
	body := reudp.MustJSON(reudp.AssocOKPayload{OK: true, DeviceID: deviceID, UDPHint: publicUDP})
	b, err := reudp.Encode(reudp.Packet{
		Type:      reudp.TypeAssocOK,
		RouteHash: reudp.RouteHash(deviceID),
		Payload:   body,
	}, nil)
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(b, addr)
}

func writeUDPErr(conn *net.UDPConn, addr *net.UDPAddr, hash uint32, code, msg string) {
	body := reudp.MustJSON(reudp.ErrorPayload{Code: code, Message: msg})
	b, err := reudp.Encode(reudp.Packet{
		Type:      reudp.TypeError,
		RouteHash: hash,
		Payload:   body,
	}, nil)
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(b, addr)
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	ip := make(net.IP, len(a.IP))
	copy(ip, a.IP)
	return &net.UDPAddr{IP: ip, Port: a.Port, Zone: a.Zone}
}
