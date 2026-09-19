//go:build linux

package dirread

import (
	"encoding/binary"
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

// Visit calls fn for each child from getdents blocks. dent is nil; Info
// should use the constructed path. fn may stop the listing with an error.
func Visit(dir string, scratch []byte, fn func(name string, typ os.FileMode, dent os.DirEntry) error) error {
	if len(scratch) < MinimumScratch() {
		var stack [blockSize]byte
		scratch = stack[:]
	}
	fd, err := openRetry(dir)
	if err != nil {
		return &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	for {
		n, err := readRetry(fd, scratch)
		if err != nil {
			return os.NewSyscallError("readdirent", err)
		}
		if n <= 0 {
			return nil
		}
		if err := visitBlock(dir, scratch[:n], fn); err != nil {
			return err
		}
	}
}

func visitBlock(dir string, block []byte, fn func(string, os.FileMode, os.DirEntry) error) error {
	for consumed := 0; consumed < len(block); {
		adv, name, typ := parseLinux(block[consumed:])
		if adv == 0 {
			if consumed < len(block) {
				return ErrTruncatedRecord
			}
			break
		}
		consumed += adv
		if name == "" || name == "." || name == ".." {
			continue
		}
		if typ == UnknownType {
			resolved, err := resolveUnknown(dir, name)
			if err != nil {
				return err
			}
			if resolved == UnknownType {
				continue
			}
			typ = resolved
		}
		if err := fn(name, typ, nil); err != nil {
			return err
		}
	}
	return nil
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
	tmp := scratch
	if len(tmp) < MinimumScratch() {
		var stack [blockSize]byte
		tmp = stack[:]
	}
	return readNames(dir, tmp)
}

func readNames(dir string, tmp []byte) ([]string, error) {
	fd, err := openRetry(dir)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	var out []string
	for {
		n, err := readRetry(fd, tmp)
		if err != nil {
			return out, os.NewSyscallError("readdirent", err)
		}
		if n <= 0 {
			return out, nil
		}
		next, err := appendNames(tmp[:n], out)
		if err != nil {
			return out, err
		}
		out = next
	}
}

func appendNames(block []byte, buf []string) ([]string, error) {
	for consumed := 0; consumed < len(block); {
		adv, name, _ := parseLinux(block[consumed:])
		if adv == 0 {
			if consumed < len(block) {
				return buf, ErrTruncatedRecord
			}
			break
		}
		consumed += adv
		if name != "" && name != "." && name != ".." {
			buf = append(buf, name)
		}
	}
	return buf, nil
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

func ParseRecords(block []byte) ([]Record, error) {
	return appendRecords(".", block, nil)
}

func appendRecords(dir string, block []byte, buf []Record) ([]Record, error) {
	for consumed := 0; consumed < len(block); {
		adv, name, typ := parseLinux(block[consumed:])
		if adv == 0 {
			if consumed < len(block) {
				return buf, ErrTruncatedRecord
			}
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
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_DIRECTORY, 0)
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
	return binary.NativeEndian.Uint16(b[off : off+2]), true
}

func readU64(b []byte, off uintptr) (uint64, bool) {
	if off+8 > uintptr(len(b)) {
		return 0, false
	}
	return binary.NativeEndian.Uint64(b[off : off+8]), true
}
