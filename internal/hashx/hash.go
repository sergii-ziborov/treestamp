package hashx

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
)

// SHA256Prefix is the oracle content-hash encoding.
func SHA256Prefix(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func New() hash.Hash { return sha256.New() }

func Finish(h hash.Hash) string {
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func Copy(w io.Writer, r io.Reader) (int64, error) {
	return io.Copy(w, r)
}
