package hashx

import (
	"encoding/binary"
	"fmt"
)

// ContentFingerprint is the oracle 128-bit whole-file cache fingerprint.
type ContentFingerprint struct {
	first  uint64
	second uint64
	bytes  uint64
}

func NewContentFingerprint() ContentFingerprint {
	return ContentFingerprint{
		first:  0xcbf29ce484222325,
		second: 0x9e3779b97f4a7c15,
	}
}

func (f *ContentFingerprint) Write(input []byte) {
	for _, b := range input {
		f.first ^= uint64(b)
		f.first *= 0x00000100000001b3
		f.second ^= uint64(b) + f.bytes
		f.second = rotl(f.second, 13) * 0x9e3779b185ebca87
		f.bytes++
	}
}

func (f ContentFingerprint) Finish() string {
	first := mix64(f.first ^ f.bytes)
	second := mix64(f.second ^ rotl(f.bytes, 29))
	return fmt.Sprintf("fp128:%016x%016x", first, second)
}

func rotl(value uint64, bits uint) uint64 {
	return value<<bits | value>>(64-bits)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ (value >> 31)
}

// WriteU32LE writes a little-endian uint32.
func WriteU32LE(h interface{ Write([]byte) (int, error) }, v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	_, _ = h.Write(buf[:])
}

// WriteU64LE writes a little-endian uint64.
func WriteU64LE(h interface{ Write([]byte) (int, error) }, v uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}
