package reudp

import (
	"encoding/json"
	"net"
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
	directConn   *net.UDPConn // optional P2P path after hole-punch
	preferDirect bool
}

type inflightPkt struct {
	data    []byte
	sentAt  time.Time
	retries int
}

func Dial(relayHostPort string) (*Endpoint, error) {
	raddr, err := net.ResolveUDPAddr("udp", relayHostPort)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadBuffer(1 << 20)
	_ = conn.SetWriteBuffer(1 << 20)
	ep := &Endpoint{
		conn:      conn,
		relayAddr: raddr,
		inflight:  make(map[uint32]*inflightPkt),
		recvBuf:   make(map[uint32][]byte),
		incoming:  make(chan []byte, 256),
		assocOK:   make(chan *AssocOKPayload, 1),
		assocErr:  make(chan error, 1),
		cong:      NewCongestion(),
	}
	go ep.readLoop()
	go ep.retransmitLoop()
	return ep, nil
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
	ep.preferDirect = false
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
	ep.preferDirect = true
	ep.mu.Unlock()
	if old != nil && old != conn {
		_ = old.Close()
	}
	go ep.readLoopConn(conn)
}

// ClearDirect falls back to relay-only path.
func (ep *Endpoint) ClearDirect() {
	ep.mu.Lock()
	old := ep.directConn
	ep.directConn = nil
	ep.preferDirect = false
	ep.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func (ep *Endpoint) UsingDirect() bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.preferDirect && ep.directConn != nil
}

func (ep *Endpoint) writeBytes(b []byte) error {
	ep.mu.Lock()
	direct := ep.directConn
	prefer := ep.preferDirect
	ep.mu.Unlock()
	if prefer && direct != nil {
		_, err := direct.Write(b)
		if err == nil {
			return nil
		}
		// Fall back to relay on direct write failure.
		ep.mu.Lock()
		ep.preferDirect = false
		ep.mu.Unlock()
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
	for i := 0; i < 500; i++ {
		if ep.cong.CanSend() {
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
	ep.mu.Lock()
	seq := ep.nextSeq
	ep.nextSeq++
	ack := uint32(0)
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
		return err
	}
	if flags&FlagReliable != 0 {
		ep.mu.Lock()
		ep.inflight[seq] = &inflightPkt{data: append([]byte(nil), b...), sentAt: time.Now()}
		ep.mu.Unlock()
	}
	ep.cong.OnSend()
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
		n, err := conn.Read(buf)
		if err != nil {
			ep.mu.Lock()
			closed := ep.closed
			isPrimary := conn == ep.conn
			ep.mu.Unlock()
			if isPrimary && !closed {
				close(ep.incoming)
			}
			return
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
			cum, _, err := DecodeAck(pkt.Payload)
			if err != nil {
				continue
			}
			ep.handleAck(cum)
		case TypeData:
			if pkt.Ack > 0 {
				ep.handleAck(pkt.Ack)
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

func (ep *Endpoint) handleAck(cum uint32) {
	ep.mu.Lock()
	n := 0
	var rtt time.Duration
	for seq, inf := range ep.inflight {
		if seq <= cum {
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
		select {
		case ep.incoming <- body:
		default:
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
	_ = ep.writeRaw(Packet{
		Type:      TypeAck,
		RouteHash: ep.routeHash,
		Ack:       cum,
		Payload:   EncodeAck(cum, 0),
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
		for _, inf := range ep.inflight {
			if now.Sub(inf.sentAt) > 200*time.Millisecond && inf.retries < 8 {
				_ = ep.writeBytes(inf.data)
				inf.sentAt = now
				inf.retries++
				if inf.retries >= 3 {
					lost = true
				}
			}
		}
		ep.mu.Unlock()
		if lost {
			ep.cong.OnLoss()
		}
	}
}

type assocError struct{ code, msg string }

func (e *assocError) Error() string { return e.code + ": " + e.msg }
