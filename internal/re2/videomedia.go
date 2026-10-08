package re2

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Video plane wire prefix — distinguishes gap-tolerant video AEAD from ordered Noise
// ciphertext on the same REUDP DATA channel.
const (
	VideoPlaneMagic0  = 0x56 // 'V'
	VideoPlaneMagic1  = 0x31 // '1'
	VideoPlaneVersion = 1
	videoNonceSize    = chacha20poly1305.NonceSize // 12
	videoOverhead     = 2 + videoNonceSize + 16    // magic + nonce + Poly1305 tag
	// Match reudp.MaxPayload without importing reudp (avoid cycles).
	videoUDPMaxPayload = 1200
)

// VideoMedia is a bidirectional, loss-tolerant AEAD for MsgVideo datagrams.
// Nonces are explicit in the clear — drops do not desync control Noise.
type VideoMedia struct {
	send cipher.AEAD
	recv cipher.AEAD
}

// DeriveVideoMediaKeys builds agent→client and client→agent keys after Noise XX.
// initiatorStatic = client ephemeral static; responderStatic = agent long-term static.
func DeriveVideoMediaKeys(psk, initiatorStatic, responderStatic []byte) (a2c, c2a []byte, err error) {
	if len(psk) != 32 || len(initiatorStatic) != 32 || len(responderStatic) != 32 {
		return nil, nil, errors.New("re2: video media key material must be 32 bytes each")
	}
	salt := sha256.Sum256([]byte("re2-video-v1"))
	ikm := make([]byte, 0, 96)
	ikm = append(ikm, psk...)
	ikm = append(ikm, initiatorStatic...)
	ikm = append(ikm, responderStatic...)
	r := hkdf.New(sha256.New, ikm, salt[:], []byte("re2-video-v1"))
	a2c = make([]byte, 32)
	c2a = make([]byte, 32)
	if _, err = io.ReadFull(r, a2c); err != nil {
		return nil, nil, err
	}
	if _, err = io.ReadFull(r, c2a); err != nil {
		return nil, nil, err
	}
	return a2c, c2a, nil
}

// NewVideoMediaAgent: Agent sends with a2c, receives with c2a.
func NewVideoMediaAgent(a2c, c2a []byte) (*VideoMedia, error) {
	return newVideoMedia(a2c, c2a)
}

// NewVideoMediaClient: Client sends with c2a, receives with a2c.
func NewVideoMediaClient(a2c, c2a []byte) (*VideoMedia, error) {
	return newVideoMedia(c2a, a2c)
}

func newVideoMedia(sendKey, recvKey []byte) (*VideoMedia, error) {
	sa, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, err
	}
	ra, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, err
	}
	return &VideoMedia{send: sa, recv: ra}, nil
}

// IsVideoPlane reports whether payload is a v1 video-plane datagram.
func IsVideoPlane(payload []byte) bool {
	return len(payload) >= videoOverhead &&
		payload[0] == VideoPlaneMagic0 &&
		payload[1] == VideoPlaneMagic1
}

// Seal wraps plaintext (typically EncodeInner(MsgVideo, …)) for unreliable send.
func (v *VideoMedia) Seal(plain []byte) ([]byte, error) {
	if v == nil || v.send == nil {
		return nil, errors.New("re2: video media not ready")
	}
	nonce := make([]byte, videoNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := v.send.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, 2+len(nonce)+len(ct))
	out = append(out, VideoPlaneMagic0, VideoPlaneMagic1)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Open decrypts a video-plane datagram. Gaps/loss are fine — failed opens return error.
func (v *VideoMedia) Open(packet []byte) ([]byte, error) {
	if v == nil || v.recv == nil {
		return nil, errors.New("re2: video media not ready")
	}
	if !IsVideoPlane(packet) {
		return nil, errors.New("re2: not a video plane packet")
	}
	nonce := packet[2 : 2+videoNonceSize]
	ct := packet[2+videoNonceSize:]
	return v.recv.Open(nil, nonce, ct, nil)
}

// MaxVideoPlainUDP is the largest plaintext that fits in one REUDP DATA after seal.
func MaxVideoPlainUDP() int {
	return videoUDPMaxPayload - videoOverhead
}
