package reudp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Magic0     = 0x52 // 'R'
	Magic1     = 0x55 // 'U'
	Version    = 0x01
	HeaderSize = 20
	MaxPayload = 1200
	MaxPacket  = HeaderSize + MaxPayload
)

// Packet types (relay-visible).
const (
	TypeAssoc   byte = 0x01
	TypeAssocOK byte = 0x02
	TypeData    byte = 0x10
	TypeAck     byte = 0x11
	TypePing    byte = 0x1E
	TypePong    byte = 0x1F
	TypeError   byte = 0x20
)

// Flags
const (
	FlagReliable byte = 1 << 0
	FlagFin      byte = 1 << 1
	FlagLatest   byte = 1 << 2 // semi-reliable: mouse move etc.
)

var (
	ErrBadMagic   = errors.New("reudp: bad magic")
	ErrBadVersion = errors.New("reudp: bad version")
	ErrTooLarge   = errors.New("reudp: payload too large")
	ErrShort      = errors.New("reudp: packet too short")
)

// Packet is one UDP datagram.
// Layout: magic(2) ver(1) type(1) flags(1) rsv(1) route_hash(4) seq(4) ack(4) plen(2) payload
type Packet struct {
	Type      byte
	Flags     byte
	RouteHash uint32
	Seq       uint32
	Ack       uint32
	Payload   []byte
}

func Encode(p Packet, dst []byte) ([]byte, error) {
	if len(p.Payload) > MaxPayload {
		return nil, ErrTooLarge
	}
	need := HeaderSize + len(p.Payload)
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	dst[0] = Magic0
	dst[1] = Magic1
	dst[2] = Version
	dst[3] = p.Type
	dst[4] = p.Flags
	dst[5] = 0
	binary.BigEndian.PutUint32(dst[6:10], p.RouteHash)
	binary.BigEndian.PutUint32(dst[10:14], p.Seq)
	binary.BigEndian.PutUint32(dst[14:18], p.Ack)
	binary.BigEndian.PutUint16(dst[18:20], uint16(len(p.Payload)))
	copy(dst[HeaderSize:], p.Payload)
	return dst, nil
}

func Decode(b []byte) (*Packet, error) {
	if len(b) < HeaderSize {
		return nil, ErrShort
	}
	if b[0] != Magic0 || b[1] != Magic1 {
		return nil, ErrBadMagic
	}
	if b[2] != Version {
		return nil, ErrBadVersion
	}
	plen := int(binary.BigEndian.Uint16(b[18:20]))
	if plen > MaxPayload || HeaderSize+plen > len(b) {
		return nil, ErrTooLarge
	}
	payload := make([]byte, plen)
	copy(payload, b[HeaderSize:HeaderSize+plen])
	return &Packet{
		Type:      b[3],
		Flags:     b[4],
		RouteHash: binary.BigEndian.Uint32(b[6:10]),
		Seq:       binary.BigEndian.Uint32(b[10:14]),
		Ack:       binary.BigEndian.Uint32(b[14:18]),
		Payload:   payload,
	}, nil
}

func EncodeAck(cumAck uint32, sack uint64) []byte {
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], cumAck)
	binary.BigEndian.PutUint64(out[4:12], sack)
	return out
}

func DecodeAck(b []byte) (cumAck uint32, sack uint64, err error) {
	if len(b) < 4 {
		return 0, 0, fmt.Errorf("reudp: ack too short")
	}
	cumAck = binary.BigEndian.Uint32(b[0:4])
	if len(b) >= 12 {
		sack = binary.BigEndian.Uint64(b[4:12])
	}
	return cumAck, sack, nil
}

const (
	RoleAgent  = "agent"
	RoleClient = "client"
)

type AssocPayload struct {
	Role          string `json:"role"`
	DeviceID      string `json:"device_id"`
	SessionTicket string `json:"session_ticket,omitempty"`
	DeviceSecret  string `json:"device_secret,omitempty"`
}

type AssocOKPayload struct {
	OK       bool   `json:"ok"`
	DeviceID string `json:"device_id"`
	UDPHint  string `json:"udp,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
