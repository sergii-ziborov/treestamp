package hashx

import (
	"bytes"
	"strings"
	"testing"
)

func TestSHA256PrefixAndFingerprint(t *testing.T) {
	got := SHA256Prefix([]byte("abc"))
	if !strings.HasPrefix(got, "sha256:") || len(got) < 20 {
		t.Fatalf("hash %s", got)
	}
	h := New()
	_, _ = h.Write([]byte("abc"))
	if Finish(h) != got {
		t.Fatal("finish")
	}
	fp := NewContentFingerprint()
	fp.Write([]byte("hello"))
	fp.Write([]byte("world"))
	out := fp.Finish()
	if !strings.HasPrefix(out, "fp128:") || len(out) != 6+32 {
		t.Fatalf("fp %s", out)
	}
	var buf bytes.Buffer
	n, err := Copy(&buf, strings.NewReader("xy"))
	if err != nil || n != 2 || buf.String() != "xy" {
		t.Fatalf("copy %d %v", n, err)
	}
	WriteU32LE(h, 1)
	WriteU64LE(h, 2)
}

func TestFingerprintEmpty(t *testing.T) {
	fp := NewContentFingerprint()
	if fp.Finish() == "" {
		t.Fatal("empty")
	}
}
