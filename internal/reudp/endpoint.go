package reudp

import (
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Endpoint is a client-side UDP association to a relay, with optional P2P direct path.
type Endpoint struct {
	conn      *net.UDPConn
	relayAddr *net.UDPAddr
	routeHash uint32
	deviceID  string

	mu           sync.Mutex
	nextSeq      uint32
	nextExpect   uint32
	inflight     map[uint32]*inflightPkt
	recvBuf      map[uint32][]byte
	incoming     chan []byte
	assocOK      chan *AssocOKPayload
	assocErr     chan error
	cong         *Congestion
	closed       bool
	directConn   *net.UDPConn // optional P2P path after hole-punch (legacy DialUDP)
	directAddr   *net.UDPAddr // same-LAN: WriteToUDP on main listen (port == QR lan port)
	preferDirect bool
	probeWait    chan *net.UDPAddr // waiter for REHP1|pong during ProbePreferDirect
}

type inflightPkt struct {
	data    []byte
	sentAt  time.Time
	retries int
}

func Dial(relayHostPort string) (*Endpoint, error) {
	return DialListen(relayHostPort, 0)
}

// DialListen binds an IPv4 UDP socket. preferPort>0 tries that port first (sticky
// LAN listen across Agent restarts so QR lan:port stays valid). On bind failure
// or preferPort==0, falls back to an ephemeral port.
func DialListen(relayHostPort string, preferPort int) (*Endpoint, error) {
	raddr, err := net.ResolveUDPAddr("udp", relayHostPort)
	if err != nil {
		return nil, err
	}
	if preferPort < 0 || preferPort > 65535 {
		preferPort = 0
	}
	// IPv4 unconnected listen: QR lanCandidates are IPv4 only, and dual-stack
	// "udp" sockets (esp. under Rosetta) have been observed to accept binds yet
	// never deliver LAN REHP1 to ReadFromUDP.
	conn, err := listenUDP4(preferPort)
	if err != nil {
		return nil, err
	}
	if preferPort > 0 {
		if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok && ua != nil && ua.Port == preferPort {
			log.Printf("reudp: reused sticky listen port %d", preferPort)
		}
	}
	_ = conn.SetReadBuffer(1 << 20)
	_ = conn.SetWriteBuffer(1 << 20)
	ep := &Endpoint{
		conn:      conn,
		relayAddr: raddr,
		inflight:  make(map[uint32]*inflightPkt),
		recvBuf:   make(map[uint32][]byte),
		incoming:  make(chan []byte, 1024),
		assocOK:   make(chan *AssocOKPayload, 1),
		assocErr:  make(chan error, 1),
		cong:      NewCongestion(),
	}
	go ep.readLoop()
	go ep.retransmitLoop()
	return ep, nil
}

func listenUDP4(preferPort int) (*net.UDPConn, error) {
	if preferPort > 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: preferPort})
		if err == nil {
			return conn, nil
		}
		log.Printf("reudp: sticky port %d busy (%v) — picking ephemeral", preferPort, err)
	}
	return net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
}

// ListenPort returns the bound local UDP port, or 0 if unknown.
func (ep *Endpoint) ListenPort() int {
	if ep == nil || ep.conn == nil {
		return 0
	}
	if ua, ok := ep.conn.LocalAddr().(*net.UDPAddr); ok && ua != nil {
		return ua.Port
	}
	return 0
}

func (ep *Endpoint) Close() error {
	ep.mu.Lock()
	if ep.closed {
		ep.mu.Unlock()
		return nil
	}
	ep.closed = true
	direct := ep.directConn
	ep.directConn = nil
	ep.directAddr = nil
	ep.preferDirect = false
	ep.probeWait = nil
	ep.mu.Unlock()
	if direct != nil {
		_ = direct.Close()
	}
	return ep.conn.Close()
}

func (ep *Endpoint) LocalAddr() net.Addr { return ep.conn.LocalAddr() }

// PreferDirect switches DATA/ACK writes to a punched peer UDP socket while
// keeping the relay socket for fallback reads. Media then benefits from P2P RTT.
func (ep *Endpoint) PreferDirect(conn *net.UDPConn) {
	if conn == nil {
		return
	}
	_ = conn.SetReadBuffer(1 << 20)
	_ = conn.SetWriteBuffer(1 << 20)
	ep.mu.Lock()
	old := ep.directConn
	ep.directConn = conn
	ep.directAddr = nil
	ep.preferDirect = true
	ep.mu.Unlock()
	if old != nil && old != conn {
		_ = old.Close()
	}
	ep.cong.BoostForLAN()
	go ep.readLoopConn(conn)
}

// PreferDirectAddr routes DATA via the main listen socket to peer.
// Prefer this on same-LAN: QR/LAN advertise this listen port, so App→Agent and
// Agent→App share one firewall hole (no ephemeral DialUDP socket).
func (ep *Endpoint) PreferDirectAddr(addr *net.UDPAddr) {
	if addr == nil {
		return
	}
	ep.mu.Lock()
	old := ep.directConn
	ep.directConn = nil
	ep.directAddr = cloneUDPAddr(addr)
	ep.preferDirect = true
	ep.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	ep.cong.BoostForLAN()
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	ip := make(net.IP, len(a.IP))
	copy(ip, a.IP)
	return &net.UDPAddr{IP: ip, Port: a.Port, Zone: a.Zone}
}

func udpAddrEqual(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

func isPrivateUDPAddr(a *net.UDPAddr) bool {
	if a == nil || a.IP == nil {
		return false
	}
	ip := a.IP.To4()
	if ip == nil {
		return false
	}
	if ip[0] == 10 {
		return true
	}
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	return false
}

// ProbePreferDirect sends REHP1 from the main listen port and latches PreferDirectAddr
// when the peer replies. Same port as QR lanCandidates — critical for LAN P2P.
func (ep *Endpoint) ProbePreferDirect(peerHostPort, token string, timeout time.Duration) error {
	raddr, err := net.ResolveUDPAddr("udp", peerHostPort)
	if err != nil {
		return err
	}
	ch := make(chan *net.UDPAddr, 1)
	ep.mu.Lock()
	ep.probeWait = ch
	ep.mu.Unlock()
	defer func() {
		ep.mu.Lock()
		if ep.probeWait == ch {
			ep.probeWait = nil
		}
		ep.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	msg := []byte("REHP1|" + token)
	for time.Now().Before(deadline) {
		_, _ = ep.conn.WriteToUDP(msg, raddr)
		select {
		case addr := <-ch:
			ep.PreferDirectAddr(addr)
			return nil
		case <-time.After(150 * time.Millisecond):
		}
	}
	return &opError{msg: "reudp: no REHP1 pong from " + peerHostPort}
}

type opError struct{ msg string }

func (e *opError) Error() string { return e.msg }

// ClearDirect falls back to relay-only path.
func (ep *Endpoint) ClearDirect() {
	ep.mu.Lock()
	old := ep.directConn
	ep.directConn = nil
	ep.directAddr = nil
	ep.preferDirect = false
	ep.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func (ep *Endpoint) UsingDirect() bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.preferDirect && (ep.directConn != nil || ep.directAddr != nil)
}

// ResetReliableSession clears seq/ACK state after a new Noise session.
// App creates a fresh Endpoint (nextExpect=0) on each connect; Agent keeps the
// same udpEP across rescans — without this, OPEN arrives as seq=500+ and the
// App buffers forever ("正在打开桌面").
func (ep *Endpoint) ResetReliableSession() {
	ep.mu.Lock()
	ep.nextSeq = 0
	ep.nextExpect = 0
	ep.inflight = make(map[uint32]*inflightPkt)
	ep.recvBuf = make(map[uint32][]byte)
	ep.mu.Unlock()
	if ep.cong != nil {
		ep.cong = NewCongestion()
	}
}

// ResetReliableSend restarts only the outbound reliable stream. The client sends its
// first reliable message (OPEN, seq 0) right after Noise msg3, so it can already be
// received when the Agent finishes the handshake; resetting the inbound side then would
// make the Agent wait for seq 0 again and hold every later message forever.
func (ep *Endpoint) ResetReliableSend() {
	ep.mu.Lock()
	ep.nextSeq = 0
	ep.inflight = make(map[uint32]*inflightPkt)
	ep.mu.Unlock()
	if ep.cong != nil {
		ep.cong = NewCongestion()
	}
}

func (ep *Endpoint) writeBytes(b []byte) error {
	ep.mu.Lock()
	direct := ep.directConn
	directAddr := ep.directAddr
	prefer := ep.preferDirect
	relay := ep.relayAddr
	ep.mu.Unlock()
	if prefer && direct != nil {
		_, err := direct.Write(b)
		if err == nil {
			return nil
		}
		ep.mu.Lock()
		ep.preferDirect = false
		ep.mu.Unlock()
	}
	if prefer && directAddr != nil {
		_, err := ep.conn.WriteToUDP(b, directAddr)
		if err == nil {
			return nil
		}
		ep.mu.Lock()
		ep.preferDirect = false
		ep.mu.Unlock()
	}
	if relay != nil {
		_, err := ep.conn.WriteToUDP(b, relay)
		return err
	}
	_, err := ep.conn.Write(b)
	return err
}

func (ep *Endpoint) AssocAgent(deviceID, deviceSecret string) error {
	ep.deviceID = deviceID
	ep.routeHash = RouteHash(deviceID)
	body, _ := json.Marshal(AssocPayload{
		Role:         RoleAgent,
		DeviceID:     deviceID,
		DeviceSecret: deviceSecret,
	})
	return ep.writeRaw(Packet{Type: TypeAssoc, RouteHash: ep.routeHash, Payload: body})
}

func (ep *Endpoint) AssocClient(deviceID, sessionTicket string) error {
	ep.deviceID = deviceID
	ep.routeHash = RouteHash(deviceID)
	body, _ := json.Marshal(AssocPayload{
		Role:          RoleClient,
		DeviceID:      deviceID,
		SessionTicket: sessionTicket,
	})
	return ep.writeRaw(Packet{Type: TypeAssoc, RouteHash: ep.routeHash, Payload: body})
}

func (ep *Endpoint) WaitAssocOK(d time.Duration) (*AssocOKPayload, error) {
	select {
	case ok := <-ep.assocOK:
		return ok, nil
	case err := <-ep.assocErr:
		return nil, err
	case <-time.After(d):
		return nil, errTimeout
	}
}

func (ep *Endpoint) writeRaw(p Packet) error {
	b, err := Encode(p, nil)
	if err != nil {
		return err
	}
	return ep.writeBytes(b)
}

func (ep *Endpoint) CanSend() bool {
	if ep == nil || ep.cong == nil {
		return false
	}
	return ep.cong.CanSend()
}

func (ep *Endpoint) SendUnreliable(payload []byte) error {
	return ep.sendData(payload, 0)
}

func (ep *Endpoint) SendReliable(payload []byte) error {
	return ep.sendData(payload, FlagReliable)
}

func (ep *Endpoint) SendLatest(payload []byte) error {
	return ep.sendData(payload, FlagLatest)
}

func (ep *Endpoint) sendData(payload []byte, flags byte) error {
	// Reject before consuming a seq — Encode failure must not punch a HOL hole.
	if len(payload) > MaxPayload {
		return ErrTooLarge
	}
	if flags&FlagReliable != 0 {
		can := false
		for i := 0; i < 500; i++ {
			if ep.cong.CanSend() {
				can = true
				break
			}
			time.Sleep(time.Millisecond)
			ep.mu.Lock()
			closed := ep.closed
			ep.mu.Unlock()
			if closed {
				return net.ErrClosed
			}
		}
		// Reliable control must respect cwnd. Unreliable video is deliberately
		// independent: waiting 500ms per IDR fragment turns one lost control ACK
		// into a 15–50 second video freeze.
		if !can {
			return ErrCongested
		}
	}
	ep.mu.Lock()
	var seq uint32
	if flags&FlagReliable != 0 {
		seq = ep.nextSeq
		ep.nextSeq++
	} else {
		// Unreliable/Latest must NOT consume the reliable seq space — otherwise
		// Noise/video sprays punch HOL holes and later OPEN/READY never deliver
		// (LAN E2E: UDP Noise OK, desktop open timeout).
		seq = 0
	}
	// MaxUint32 means "no contiguous reliable packet received yet". Zero would
	// falsely acknowledge peer sequence 0 when this data packet piggybacks ACKs.
	ack := ^uint32(0)
	if ep.nextExpect > 0 {
		ack = ep.nextExpect - 1
	}
	ep.mu.Unlock()

	p := Packet{
		Type:      TypeData,
		Flags:     flags,
		RouteHash: ep.routeHash,
		Seq:       seq,
		Ack:       ack,
		Payload:   payload,
	}
	b, err := Encode(p, nil)
	if err != nil {
		// Extremely unlikely after the length check; still avoid leaving a hole
		// by not recording inflight (receiver will stall — log upstream).
		return err
	}
	if flags&FlagReliable != 0 {
		ep.mu.Lock()
		ep.inflight[seq] = &inflightPkt{data: append([]byte(nil), b...), sentAt: time.Now()}
		ep.mu.Unlock()
		ep.cong.OnSend()
	}
	// Unreliable/Latest must not consume cong.inFlight — ACKs never free them,
	// so ~8 video parts permanently stall CanSend (LAN: first paint then freeze).
	return ep.writeBytes(b)
}

func (ep *Endpoint) Recv() ([]byte, error) {
	b, ok := <-ep.incoming
	if !ok {
		return nil, net.ErrClosed
	}
	return b, nil
}

func (ep *Endpoint) RecvTimeout(d time.Duration) ([]byte, error) {
	select {
	case b, ok := <-ep.incoming:
		if !ok {
			return nil, net.ErrClosed
		}
		return b, nil
	case <-time.After(d):
		return nil, errTimeout
	}
}

var errTimeout = timeoutErr("reudp: recv timeout")

type timeoutErr string

func (e timeoutErr) Error() string { return string(e) }
func (e timeoutErr) Timeout() bool { return true }

func (ep *Endpoint) readLoop() {
	ep.readLoopConn(ep.conn)
}

func (ep *Endpoint) readLoopConn(conn *net.UDPConn) {
	buf := make([]byte, MaxPacket+64)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			ep.mu.Lock()
			closed := ep.closed
			isPrimary := conn == ep.conn
			ep.mu.Unlock()
			if isPrimary && !closed {
				log.Printf("reudp: readLoop exit on primary: %v", err)
				close(ep.incoming)
			}
			return
		}
		// Same-LAN hole-punch probes (REHP1|token) — reply so PreferDirect can latch.
		// Distinguish peer pongs (our ProbePreferDirect) from inbound probes.
		if n >= 6 && string(buf[:6]) == "REHP1|" && addr != nil {
			msg := string(buf[:n])
			if strings.HasPrefix(msg, "REHP1|pong") || msg == "REHP1|pong" {
				ep.mu.Lock()
				w := ep.probeWait
				ep.mu.Unlock()
				if w != nil {
					select {
					case w <- cloneUDPAddr(addr):
					default:
					}
				}
				continue
			}
			log.Printf("reudp: REHP1 from %v len=%d", addr, n)
			if _, err := conn.WriteToUDP([]byte("REHP1|pong"), addr); err != nil {
				log.Printf("reudp: REHP1 pong to %v failed: %v", addr, err)
			}
			continue
		}
		pkt, err := Decode(buf[:n])
		if err != nil {
			continue
		}
		switch pkt.Type {
		case TypeAssocOK:
			var ok AssocOKPayload
			if err := json.Unmarshal(pkt.Payload, &ok); err != nil {
				select {
				case ep.assocErr <- err:
				default:
				}
				continue
			}
			select {
			case ep.assocOK <- &ok:
			default:
			}
		case TypeAck:
			cum, sack, err := DecodeAck(pkt.Payload)
			if err != nil {
				continue
			}
			ep.handleAck(cum, sack)
		case TypeData:
			// Ack zero validly acknowledges reliable sequence zero. MaxUint32
			// explicitly represents "no cumulative ACK yet".
			ep.handleAck(pkt.Ack, 0)
			// Same-LAN: DATA from a private peer (Noise msg1) — latch PreferDirect
			// so msg2/ACK ride LAN instead of volunteer-relay UDP (often black-holed).
			// Always refresh when the peer port changes (sim relaunch gets a new port).
			if addr != nil && isPrivateUDPAddr(addr) && !udpAddrEqual(addr, ep.relayAddr) {
				ep.mu.Lock()
				same := ep.preferDirect && udpAddrEqual(addr, ep.directAddr)
				ep.mu.Unlock()
				if !same {
					ep.PreferDirectAddr(addr)
				}
			}
			ep.handleData(pkt)
		case TypePing:
			_ = ep.writeRaw(Packet{Type: TypePong, RouteHash: ep.routeHash, Payload: pkt.Payload})
		case TypeError:
			var epay ErrorPayload
			_ = json.Unmarshal(pkt.Payload, &epay)
			select {
			case ep.assocErr <- &assocError{epay.Code, epay.Message}:
			default:
			}
		}
	}
}

func (ep *Endpoint) handleAck(cum uint32, sack uint64) {
	ep.mu.Lock()
	n := 0
	var rtt time.Duration
	for seq, inf := range ep.inflight {
		if ackedBy(cum, sack, seq) {
			if rtt == 0 {
				rtt = time.Since(inf.sentAt)
			}
			delete(ep.inflight, seq)
			n++
		}
	}
	ep.mu.Unlock()
	if n > 0 {
		ep.cong.OnAck(n, rtt)
	}
}

func ackedBy(cum uint32, sack uint64, seq uint32) bool {
	if cum != ^uint32(0) && seq <= cum {
		return true
	}
	base := cum + 1 // MaxUint32 intentionally wraps to sequence 0.
	delta := seq - base
	return delta < 64 && sack&(uint64(1)<<delta) != 0
}

func (ep *Endpoint) handleData(pkt *Packet) {
	if pkt.Flags&FlagReliable == 0 {
		select {
		case ep.incoming <- pkt.Payload:
		default:
		}
		return
	}
	ep.mu.Lock()
	if pkt.Seq < ep.nextExpect {
		cum := ep.nextExpect - 1
		ep.mu.Unlock()
		ep.sendAck(cum)
		return
	}
	ep.recvBuf[pkt.Seq] = pkt.Payload
	for {
		body, ok := ep.recvBuf[ep.nextExpect]
		if !ok {
			break
		}
		delete(ep.recvBuf, ep.nextExpect)
		ep.nextExpect++
		ep.mu.Unlock()
		// Never drop reliable payloads — blocking briefly beats HOL stalls upstream.
		select {
		case ep.incoming <- body:
		case <-time.After(2 * time.Second):
			select {
			case ep.incoming <- body:
			default:
			}
		}
		ep.mu.Lock()
	}
	var cum uint32
	if ep.nextExpect > 0 {
		cum = ep.nextExpect - 1
	}
	ep.mu.Unlock()
	ep.sendAck(cum)
}

func (ep *Endpoint) sendAck(cum uint32) {
	ep.mu.Lock()
	// nextExpect==0 means no contiguous packet has arrived. MaxUint32 is the
	// explicit "no cumulative ACK" sentinel; zero used to falsely ACK seq 0.
	if ep.nextExpect == 0 {
		cum = ^uint32(0)
	}
	base := ep.nextExpect
	var sack uint64
	for seq := range ep.recvBuf {
		delta := seq - base
		if delta < 64 {
			sack |= uint64(1) << delta
		}
	}
	ep.mu.Unlock()
	_ = ep.writeRaw(Packet{
		Type:      TypeAck,
		RouteHash: ep.routeHash,
		Ack:       cum,
		Payload:   EncodeAck(cum, sack),
	})
}

func (ep *Endpoint) retransmitLoop() {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		ep.mu.Lock()
		if ep.closed {
			ep.mu.Unlock()
			return
		}
		now := time.Now()
		lost := false
		var toSend [][]byte
		for _, inf := range ep.inflight {
			age := now.Sub(inf.sentAt)
			// Keep retransmitting while unacked — volunteer relays drop heavily;
			// giving up after a few tries permanently HOL-blocks the peer.
			gap := 200 * time.Millisecond
			if inf.retries > 10 {
				gap = 500 * time.Millisecond
			}
			if age > gap {
				toSend = append(toSend, append([]byte(nil), inf.data...))
				inf.sentAt = now
				inf.retries++
				if inf.retries == 3 || inf.retries == 10 {
					lost = true
				}
			}
		}
		ep.mu.Unlock()
		// writeBytes also takes ep.mu — never call it while holding the lock.
		for _, b := range toSend {
			_ = ep.writeBytes(b)
		}
		if lost {
			ep.cong.OnLoss()
		}
	}
}

type assocError struct{ code, msg string }

func (e *assocError) Error() string { return e.code + ": " + e.msg }
