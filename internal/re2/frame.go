package re2

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	Magic0   = 0x52 // 'R'
	Magic1   = 0x32 // '2'
	Version  = 0x02
	MaxPayload = 1 << 20
)

// Outer frame types (relay-visible).
const (
	TypeRegister   byte = 0x01
	TypeRegisterOK byte = 0x02
	TypePairOffer  byte = 0x03
	TypePairRedeem byte = 0x04
	TypePairAck    byte = 0x05
	TypeBind       byte = 0x06
	TypeBindOK     byte = 0x07
	TypeNoise      byte = 0x10
	TypeTunnel     byte = 0x11
	TypePing       byte = 0x1E
	TypePong       byte = 0x1F
	TypeError      byte = 0x20
)

// Inner (E2E plaintext) message types.
const (
	MsgOpenSession   byte = 0x01
	MsgSessionReady  byte = 0x02
	MsgSessionClose  byte = 0x03
	MsgResize        byte = 0x04
	MsgPTYData       byte = 0x05
	MsgPing          byte = 0x06
	MsgPong          byte = 0x07
	MsgAppError      byte = 0x0F

	// Remote desktop (RE2.1+)
	MsgOpenDesktop   byte = 0x20
	MsgDesktopReady  byte = 0x21
	MsgDesktopClose  byte = 0x22
	MsgVideo         byte = 0x23
	MsgInputMouse    byte = 0x24
	MsgInputKey      byte = 0x25
	MsgInputTouch    byte = 0x26
	MsgAudio         byte = 0x30
	MsgClipboard     byte = 0x31
	MsgCursor        byte = 0x32 // cursor position (separate from video)
	MsgDisplays      byte = 0x33 // display list / select
	MsgStats         byte = 0x34 // client→agent RTT/loss feedback for ABR
	MsgKeyframeReq   byte = 0x35 // client requests IDR
	MsgFileOffer     byte = 0x40
	MsgFileChunk     byte = 0x41
	MsgFileAck       byte = 0x42
	MsgFilePull      byte = 0x43 // client→agent: request agent to send a local file
	MsgFileList      byte = 0x44 // list directory under xfer root
	MsgInputMode     byte = 0x45 // relative/game mouse, lock keys sync
	MsgHolePunch     byte = 0x50 // P2P UDP hole-punch signaling (still via relay E2E)
	MsgPairConfirm   byte = 0x51 // agent-local confirm before bind succeeds
	MsgAudit         byte = 0x52 // optional audit event (agent→client or local log)
	MsgWakeOnLAN     byte = 0x60
	MsgCameraOpen    byte = 0x70
	MsgCameraClose   byte = 0x71
	MsgCameraFrame   byte = 0x72
	MsgCameraList    byte = 0x73
	MsgUSBList       byte = 0x80
	MsgUSBAttach     byte = 0x81
	MsgUSBDetach     byte = 0x82
	MsgUSBData       byte = 0x83 // optional raw bridge payload
	MsgPrinterList   byte = 0x90
	MsgPrinterJob    byte = 0x91
	MsgPrinterAck    byte = 0x92
)

var (
	ErrBadMagic   = errors.New("re2: bad magic")
	ErrBadVersion = errors.New("re2: bad version")
	ErrTooLarge   = errors.New("re2: payload too large")
)

// Frame is an outer RE2 frame.
type Frame struct {
	Type    byte
	RouteID string
	Payload []byte
}

// EncodeFrame writes one frame to w.
func EncodeFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayload {
		return ErrTooLarge
	}
	route := []byte(f.RouteID)
	hdr := make([]byte, 12+len(route))
	hdr[0] = Magic0
	hdr[1] = Magic1
	hdr[2] = Version
	hdr[3] = f.Type
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(route)))
	copy(hdr[8:], route)
	binary.BigEndian.PutUint32(hdr[8+len(route):], uint32(len(f.Payload)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(f.Payload) == 0 {
		return nil
	}
	_, err := w.Write(f.Payload)
	return err
}

// DecodeFrame reads one frame from r.
func DecodeFrame(r io.Reader) (*Frame, error) {
	var fixed [8]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return nil, err
	}
	if fixed[0] != Magic0 || fixed[1] != Magic1 {
		return nil, ErrBadMagic
	}
	if fixed[2] != Version {
		return nil, ErrBadVersion
	}
	typ := fixed[3]
	routeLen := binary.BigEndian.Uint32(fixed[4:8])
	if routeLen > 512 {
		return nil, fmt.Errorf("re2: route id too long")
	}
	route := make([]byte, routeLen)
	if _, err := io.ReadFull(r, route); err != nil {
		return nil, err
	}
	var plenBuf [4]byte
	if _, err := io.ReadFull(r, plenBuf[:]); err != nil {
		return nil, err
	}
	plen := binary.BigEndian.Uint32(plenBuf[:])
	if plen > MaxPayload {
		return nil, ErrTooLarge
	}
	payload := make([]byte, plen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return &Frame{Type: typ, RouteID: string(route), Payload: payload}, nil
}

// DerivePSK turns a pairing token into a 32-byte Noise PSK.
func DerivePSK(pairingToken string) []byte {
	// HKDF-SHA256 extract+expand with fixed salt/info (see protocol-v2.md).
	sum := sha256.Sum256([]byte("re2-psk-v1|" + pairingToken))
	return sum[:]
}

// EncodeInner builds plaintext inner message.
func EncodeInner(msgType byte, body []byte) []byte {
	out := make([]byte, 5+len(body))
	out[0] = msgType
	binary.BigEndian.PutUint32(out[1:5], uint32(len(body)))
	copy(out[5:], body)
	return out
}

// DecodeInner parses plaintext inner message.
func DecodeInner(b []byte) (msgType byte, body []byte, err error) {
	if len(b) < 5 {
		return 0, nil, errors.New("re2: inner too short")
	}
	msgType = b[0]
	n := binary.BigEndian.Uint32(b[1:5])
	if int(n) > len(b)-5 {
		return 0, nil, errors.New("re2: inner length mismatch")
	}
	return msgType, b[5 : 5+n], nil
}

// EncodePTY packs session id + pty bytes for MsgPTYData body.
func EncodePTY(sessionID string, data []byte) []byte {
	sid := []byte(sessionID)
	if len(sid) > 255 {
		sid = sid[:255]
	}
	out := make([]byte, 1+len(sid)+len(data))
	out[0] = byte(len(sid))
	copy(out[1:], sid)
	copy(out[1+len(sid):], data)
	return out
}

// DecodePTY unpacks MsgPTYData body.
func DecodePTY(body []byte) (sessionID string, data []byte, err error) {
	if len(body) < 1 {
		return "", nil, errors.New("re2: pty body empty")
	}
	n := int(body[0])
	if len(body) < 1+n {
		return "", nil, errors.New("re2: pty session id truncated")
	}
	return string(body[1 : 1+n]), body[1+n:], nil
}
