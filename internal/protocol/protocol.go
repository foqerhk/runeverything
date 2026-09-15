package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

const Version = 1

// Message types (JSON control plane).
const (
	TypeRegister     = "register"
	TypeRegisterOK   = "register_ok"
	TypePairOffer    = "pair_offer"
	TypePairRedeem   = "pair_redeem"
	TypePairAck      = "pair_ack"
	TypeClientHello  = "client_hello"
	TypeClientOK     = "client_ok"
	TypeSessionOpen  = "session_open"
	TypeSessionReady = "session_ready"
	TypeSessionClose = "session_close"
	TypeResize       = "resize"
	TypeError        = "error"
	TypePing         = "ping"
	TypePong         = "pong"
)

// Roles
const (
	RoleAgent  = "agent"
	RoleClient = "client"
)

// Envelope is a JSON control message sent as a WebSocket text frame.
type Envelope struct {
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

func Encode(typ, id string, data interface{}) ([]byte, error) {
	env := Envelope{Type: typ, ID: id}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		env.Data = raw
	}
	return json.Marshal(env)
}

func Decode(b []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Type == "" {
		return nil, errors.New("missing type")
	}
	return &env, nil
}

func DecodeData(env *Envelope, dest interface{}) error {
	if len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, dest)
}

// RegisterData is sent by Agent on connect.
type RegisterData struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
	Name         string `json:"name"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
}

type RegisterOKData struct {
	DeviceID string `json:"device_id"`
}

// PairOfferData is published by Agent (also used to refresh pairing QR).
type PairOfferData struct {
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
	Name         string `json:"name"`
	ExpiresAt    int64  `json:"expires_at"` // unix seconds
	Relay        string `json:"relay"`      // public WSS URL for clients
}

// PairRedeemData is sent by Client after scanning QR.
type PairRedeemData struct {
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
}

// PairAckData is returned to Client after successful redeem.
type PairAckData struct {
	DeviceID     string `json:"device_id"`
	SessionToken string `json:"session_token"`
	Name         string `json:"name"`
	Relay        string `json:"relay"`
}

// ClientHelloData authenticates a Client connection for sessions.
type ClientHelloData struct {
	DeviceID     string `json:"device_id"`
	SessionToken string `json:"session_token"`
}

type ClientOKData struct {
	DeviceID string `json:"device_id"`
	Online   bool   `json:"online"`
}

// SessionOpenData requests a new PTY session on the Agent.
type SessionOpenData struct {
	SessionID string   `json:"session_id"`
	Cwd       string   `json:"cwd,omitempty"`
	Cmd       []string `json:"cmd,omitempty"` // default: login shell
	Cols      int      `json:"cols,omitempty"`
	Rows      int      `json:"rows,omitempty"`
	UseTmux   bool     `json:"use_tmux,omitempty"`
	TmuxName  string   `json:"tmux_name,omitempty"`
}

type SessionReadyData struct {
	SessionID string `json:"session_id"`
}

type SessionCloseData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

type ResizeData struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PairingPayload is encoded into the QR code (JSON).
type PairingPayload struct {
	V            int    `json:"v"`
	Relay        string `json:"relay"`
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
	Name         string `json:"name"`
	ExpiresAt    int64  `json:"expires_at"`
}

func (p PairingPayload) DeepLink() string {
	q := url.Values{}
	q.Set("v", fmt.Sprintf("%d", p.V))
	q.Set("relay", p.Relay)
	q.Set("device_id", p.DeviceID)
	q.Set("pairing_token", p.PairingToken)
	q.Set("name", p.Name)
	q.Set("expires_at", fmt.Sprintf("%d", p.ExpiresAt))
	return "koko://pair?" + q.Encode()
}

// Binary PTY frame layout (WebSocket binary message):
//
//	0      : direction (1=stdin client→agent, 2=stdout agent→client)
//	1..16  : session_id UTF-8, zero-padded / truncated to 16 bytes
//	17..   : payload bytes
const (
	DirStdin     byte = 1
	DirStdout    byte = 2
	SessionIDLen      = 16
)

var ErrFrameShort = errors.New("pty frame too short")

func EncodePTYFrame(dir byte, sessionID string, payload []byte) []byte {
	out := make([]byte, 1+SessionIDLen+len(payload))
	out[0] = dir
	id := []byte(sessionID)
	if len(id) > SessionIDLen {
		id = id[:SessionIDLen]
	}
	copy(out[1:], id)
	copy(out[1+SessionIDLen:], payload)
	return out
}

func DecodePTYFrame(b []byte) (dir byte, sessionID string, payload []byte, err error) {
	if len(b) < 1+SessionIDLen {
		return 0, "", nil, ErrFrameShort
	}
	dir = b[0]
	raw := b[1 : 1+SessionIDLen]
	end := SessionIDLen
	for i := 0; i < SessionIDLen; i++ {
		if raw[i] == 0 {
			end = i
			break
		}
	}
	sessionID = string(raw[:end])
	payload = b[1+SessionIDLen:]
	return dir, sessionID, payload, nil
}

// PutU16 kept for potential length-prefixed extensions.
func PutU16(b []byte, v uint16) {
	binary.BigEndian.PutUint16(b, v)
}
