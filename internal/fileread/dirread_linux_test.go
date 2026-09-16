//go:build linux

package fileread

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
)

func TestDirReadRejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := dirread.Read(fifo, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fifo open succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked opening fifo")
	}
}

func TestParseRecordsTruncated(t *testing.T) {
	if _, err := dirread.ParseRecords([]byte{1, 2, 3}); err == nil {
		t.Fatal("truncated record must fail")
	}
}

func TestParseRecordsNativeEndian(t *testing.T) {
	buf := make([]byte, 24)
	reclenOff := unsafe.Offsetof(syscall.Dirent{}.Reclen)
	inoOff := unsafe.Offsetof(syscall.Dirent{}.Ino)
	nameOff := unsafe.Offsetof(syscall.Dirent{}.Name)
	if int(nameOff)+2 > len(buf) {
		buf = make([]byte, nameOff+4)
	}
	binary.NativeEndian.PutUint16(buf[reclenOff:], uint16(len(buf)))
	binary.NativeEndian.PutUint64(buf[inoOff:], 1)
	copy(buf[nameOff:], []byte("a\x00"))
	recs, err := dirread.ParseRecords(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "a" {
		t.Fatalf("%+v", recs)
	}
}

func TestDirScannerFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		s, err := dirread.NewScanner(fifo, nil)
		if err != nil {
			done <- err
			return
		}
		done <- s.Close()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fifo scanner")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked scanner fifo")
	}
	_ = os.Remove(fifo)
}
