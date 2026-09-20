package reudp

import (
	"encoding/binary"
	"hash/fnv"
)

// RouteHash returns a stable u32 for device_id (relay fast path).
func RouteHash(deviceID string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(deviceID))
	return h.Sum32()
}

// PutU32 is a tiny helper for tests/tools.
func PutU32(b []byte, v uint32) {
	binary.BigEndian.PutUint32(b, v)
}
