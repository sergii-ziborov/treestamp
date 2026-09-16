//go:build linux

package dirread

import (
	"os"
	"syscall"
	"unsafe"
)

const blockSize = 32768

// Read appends directory records using getdents, matching fastwalk's Unix path.
func Read(dir string, buf []Record) ([]Record, error) {
	var stack [blockSize]byte
	return readInto(dir, buf, stack[:])
}

// ReadScratch is Read with a reusable getdents buffer.
func ReadScratch(dir string, buf []Record, scratch []byte) ([]Record, error) {
	if len(scratch) >= MinimumScratch() && len(scratch) > 0 {
		return readInto(dir, buf, scratch)
	}
	var stack [blockSize]byte
	return readInto(dir, buf, stack[:])
}

func OSEntries(dir string) ([]os.DirEntry, error) {
	return OSEntriesScratch(dir, nil)
}

func OSEntriesScratch(dir string, scratch []byte) ([]os.DirEntry, error) {
	recs, err := ReadScratch(dir, nil, scratch)
	if err != nil {
		return nil, err
	}
	out := make([]os.DirEntry, len(recs))
	for i, rec := range recs {
		out[i] = Dent{dir: dir, name: rec.Name, typ: rec.Type}
	}
	return out, nil
}

func Names(dir string, scratch []byte) ([]string, error) {
	recs, err := ReadScratch(dir, nil, scratch)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, rec := range recs {
		out[i] = rec.Name
	}
	return out, nil
}

func readInto(dir string, buf []Record, tmp []byte) ([]Record, error) {
	fd, err := openRetry(dir)
	if err != nil {
		return buf, &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	for {
		n, err := readRetry(fd, tmp)
		if err != nil {
			return buf, os.NewSyscallError("readdirent", err)
		}
		if n <= 0 {
			return buf, nil
		}
		next, err := appendRecords(dir, tmp[:n], buf)
		if err != nil {
			return buf, err
		}
		buf = next
	}
}

func appendRecords(dir string, block []byte, buf []Record) ([]Record, error) {
	for consumed := 0; consumed < len(block); {
		adv, name, typ := parseLinux(block[consumed:])
		if adv == 0 {
			break
		}
		consumed += adv
		if name == "" || name == "." || name == ".." {
			continue
		}
		if typ == UnknownType {
			resolved, err := resolveUnknown(dir, name)
			if err != nil {
				return buf, err
			}
			if resolved == UnknownType {
				continue
			}
			typ = resolved
		}
		buf = append(buf, Record{Name: name, Type: typ})
	}
	return buf, nil
}

func resolveUnknown(dir, name string) (fsMode, error) {
	info, err := os.Lstat(dir + "/" + name)
	if err != nil {
		if os.IsNotExist(err) {
			return UnknownType, nil
		}
		return 0, err
	}
	return info.Mode().Type(), nil
}

type fsMode = os.FileMode

func openRetry(path string) (int, error) {
	for {
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
		if err != syscall.EINTR {
			return fd, err
		}
	}
}

func readRetry(fd int, buf []byte) (int, error) {
	for {
		n, err := syscall.ReadDirent(fd, buf)
		if err != syscall.EINTR {
			return n, err
		}
	}
}

func parseLinux(buf []byte) (int, string, os.FileMode) {
	reclen, ok := readU16(buf, unsafe.Offsetof(syscall.Dirent{}.Reclen))
	if !ok || int(reclen) == 0 || int(reclen) > len(buf) {
		return 0, "", UnknownType
	}
	ino, ok := readU64(buf, unsafe.Offsetof(syscall.Dirent{}.Ino))
	if !ok || ino == 0 {
		return int(reclen), "", UnknownType
	}
	typ := linuxType(buf)
	name := linuxName(buf[:reclen])
	return int(reclen), name, typ
}

func linuxName(rec []byte) string {
	off := unsafe.Offsetof(syscall.Dirent{}.Name)
	if off >= uintptr(len(rec)) {
		return ""
	}
	namebuf := rec[off:]
	for i, c := range namebuf {
		if c == 0 {
			return string(namebuf[:i])
		}
	}
	return string(namebuf)
}

func linuxType(buf []byte) os.FileMode {
	off := unsafe.Offsetof(syscall.Dirent{}.Type)
	if off >= uintptr(len(buf)) {
		return UnknownType
	}
	switch buf[off] {
	case syscall.DT_DIR:
		return os.ModeDir
	case syscall.DT_LNK:
		return os.ModeSymlink
	case syscall.DT_REG:
		return 0
	case syscall.DT_FIFO:
		return os.ModeNamedPipe
	case syscall.DT_SOCK:
		return os.ModeSocket
	case syscall.DT_CHR:
		return os.ModeDevice | os.ModeCharDevice
	case syscall.DT_BLK:
		return os.ModeDevice
	default:
		return UnknownType
	}
}

func readU16(b []byte, off uintptr) (uint16, bool) {
	if off+2 > uintptr(len(b)) {
		return 0, false
	}
	return uint16(b[off]) | uint16(b[off+1])<<8, true
}

func readU64(b []byte, off uintptr) (uint64, bool) {
	if off+8 > uintptr(len(b)) {
		return 0, false
	}
	var v uint64
	for i := uintptr(0); i < 8; i++ {
		v |= uint64(b[off+i]) << (8 * i)
	}
	return v, true
}
