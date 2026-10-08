package re2

import (
	"encoding/binary"
	"errors"
)

// Phone-cam binary body for MsgPhoneCamFrame (VideoMedia / UDP).
// Wire: 'P''1' | sid_len u8 | sid | frame_id u32 BE | flags u8 | part u16 BE |
//       parts u16 BE | width u16 BE | height u16 BE | codec u8 | raw…
//
// flags: bit0 = keyframe. codec: 0=mjpeg, 1=h264.
// JSON CameraFramePayload remains accepted for WSS / older clients.

const (
	PhoneCamBinMagic0 = 'P'
	PhoneCamBinMagic1 = '1'
	PhoneCamFlagKey   = 1 << 0
	PhoneCamCodecJPEG = 0
	PhoneCamCodecH264 = 1
)

var ErrPhoneCamShort = errors.New("re2: phonecam body too short")

// IsPhoneCamBinary reports whether body uses the P1 binary layout (not JSON).
func IsPhoneCamBinary(body []byte) bool {
	return len(body) >= 2 && body[0] == PhoneCamBinMagic0 && body[1] == PhoneCamBinMagic1
}

// EncodePhoneCam packs one raw JPEG/H264 fragment.
func EncodePhoneCam(sessionID string, frameID uint32, flags byte, part, parts uint16, width, height int, codec byte, raw []byte) []byte {
	sid := []byte(sessionID)
	if len(sid) > 255 {
		sid = sid[:255]
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if width > 0xffff {
		width = 0xffff
	}
	if height > 0xffff {
		height = 0xffff
	}
	out := make([]byte, 2+1+len(sid)+4+1+2+2+2+2+1+len(raw))
	out[0] = PhoneCamBinMagic0
	out[1] = PhoneCamBinMagic1
	out[2] = byte(len(sid))
	copy(out[3:], sid)
	o := 3 + len(sid)
	binary.BigEndian.PutUint32(out[o:], frameID)
	o += 4
	out[o] = flags
	o++
	binary.BigEndian.PutUint16(out[o:], part)
	o += 2
	binary.BigEndian.PutUint16(out[o:], parts)
	o += 2
	binary.BigEndian.PutUint16(out[o:], uint16(width))
	o += 2
	binary.BigEndian.PutUint16(out[o:], uint16(height))
	o += 2
	out[o] = codec
	o++
	copy(out[o:], raw)
	return out
}

// DecodePhoneCam parses a P1 binary phone-cam part.
func DecodePhoneCam(body []byte) (sessionID string, frameID uint32, flags byte, part, parts uint16, width, height int, codec byte, raw []byte, err error) {
	if !IsPhoneCamBinary(body) {
		return "", 0, 0, 0, 0, 0, 0, 0, nil, ErrPhoneCamShort
	}
	if len(body) < 3 {
		return "", 0, 0, 0, 0, 0, 0, 0, nil, ErrPhoneCamShort
	}
	n := int(body[2])
	// magic2 + sid_len1 + sid + frame4 + flags1 + part2 + parts2 + w2 + h2 + codec1
	need := 3 + n + 4 + 1 + 2 + 2 + 2 + 2 + 1
	if len(body) < need {
		return "", 0, 0, 0, 0, 0, 0, 0, nil, ErrPhoneCamShort
	}
	sessionID = string(body[3 : 3+n])
	o := 3 + n
	frameID = binary.BigEndian.Uint32(body[o:])
	o += 4
	flags = body[o]
	o++
	part = binary.BigEndian.Uint16(body[o:])
	o += 2
	parts = binary.BigEndian.Uint16(body[o:])
	o += 2
	width = int(binary.BigEndian.Uint16(body[o:]))
	o += 2
	height = int(binary.BigEndian.Uint16(body[o:]))
	o += 2
	codec = body[o]
	o++
	raw = body[o:]
	return
}

// PhoneCamChunkSize returns max raw bytes per part for a VideoMedia UDP datagram.
func PhoneCamChunkSize(sessionID string) int {
	sid := len(sessionID)
	if sid > 255 {
		sid = 255
	}
	// seal overhead 30 + EncodeInner 5 + P1 header (2+1+sid+4+1+2+2+2+2+1)
	hdr := 30 + 5 + 2 + 1 + sid + 4 + 1 + 2 + 2 + 2 + 2 + 1
	chunk := videoUDPMaxPayload - hdr
	if chunk < 64 {
		chunk = 64
	}
	return chunk
}

// CodecByte maps codec string → wire byte.
func CodecByte(codec string) byte {
	switch codec {
	case "h264", "avc":
		return PhoneCamCodecH264
	default:
		return PhoneCamCodecJPEG
	}
}

// CodecString maps wire byte → codec string.
func CodecString(c byte) string {
	if c == PhoneCamCodecH264 {
		return "h264"
	}
	return "mjpeg"
}
