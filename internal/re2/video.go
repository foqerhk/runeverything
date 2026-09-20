package re2

import (
	"encoding/binary"
	"errors"
)

var ErrVideoShort = errors.New("re2: video body too short")

// VideoFlags for MsgVideo body.
const (
	VideoFlagKeyFrame byte = 1 << 0
)

// EncodeVideo packs one H.264 Annex-B fragment into MsgVideo body:
//
//	session_id_len u8 + session_id + frame_id u32 + flags u8 + part u16 + parts u16 + nal…
func EncodeVideo(sessionID string, frameID uint32, flags byte, part, parts uint16, nal []byte) []byte {
	sid := []byte(sessionID)
	if len(sid) > 255 {
		sid = sid[:255]
	}
	out := make([]byte, 1+len(sid)+4+1+2+2+len(nal))
	out[0] = byte(len(sid))
	copy(out[1:], sid)
	o := 1 + len(sid)
	binary.BigEndian.PutUint32(out[o:], frameID)
	o += 4
	out[o] = flags
	o++
	binary.BigEndian.PutUint16(out[o:], part)
	o += 2
	binary.BigEndian.PutUint16(out[o:], parts)
	o += 2
	copy(out[o:], nal)
	return out
}

func DecodeVideo(body []byte) (sessionID string, frameID uint32, flags byte, part, parts uint16, nal []byte, err error) {
	if len(body) < 1 {
		return "", 0, 0, 0, 0, nil, ErrVideoShort
	}
	n := int(body[0])
	if len(body) < 1+n+4+1+2+2 {
		return "", 0, 0, 0, 0, nil, ErrVideoShort
	}
	sessionID = string(body[1 : 1+n])
	o := 1 + n
	frameID = binary.BigEndian.Uint32(body[o:])
	o += 4
	flags = body[o]
	o++
	part = binary.BigEndian.Uint16(body[o:])
	o += 2
	parts = binary.BigEndian.Uint16(body[o:])
	o += 2
	nal = body[o:]
	return
}

func FragmentNAL(sessionID string, frameID uint32, flags byte, annexB []byte, maxChunk int) [][]byte {
	if maxChunk < 64 {
		maxChunk = 64
	}
	overhead := 1 + len(sessionID) + 9
	if len(sessionID) > 255 {
		overhead = 1 + 255 + 9
	}
	chunk := maxChunk - overhead
	if chunk < 32 {
		chunk = 32
	}
	if len(annexB) == 0 {
		return [][]byte{EncodeVideo(sessionID, frameID, flags, 0, 1, nil)}
	}
	nparts := (len(annexB) + chunk - 1) / chunk
	out := make([][]byte, 0, nparts)
	for i := 0; i < nparts; i++ {
		start := i * chunk
		end := start + chunk
		if end > len(annexB) {
			end = len(annexB)
		}
		out = append(out, EncodeVideo(sessionID, frameID, flags, uint16(i), uint16(nparts), annexB[start:end]))
	}
	return out
}
