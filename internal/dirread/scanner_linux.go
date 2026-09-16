//go:build linux

package dirread

import "os"

// MinimumScratch is os.Getpagesize(), matching godirwalk on Unix.
func MinimumScratch() int { return os.Getpagesize() }

func (s *Scanner) Scan() bool {
	if s.file == nil {
		return false
	}
	s.name, s.typ, s.info = "", 0, nil
	if len(s.scratch) == 0 {
		s.scratch = make([]byte, blockSize)
	}
	for {
		if !s.fillWork() {
			return false
		}
		adv, name, typ := parseLinux(s.work)
		if adv == 0 {
			s.work = nil
			continue
		}
		s.work = s.work[adv:]
		if name == "" || name == "." || name == ".." {
			continue
		}
		if typ == UnknownType {
			resolved, err := resolveUnknown(s.dir, name)
			if err != nil {
				s.finish(err)
				return false
			}
			if resolved == UnknownType {
				continue
			}
			typ = resolved
		}
		s.name = name
		s.typ = typ
		return true
	}
}

func (s *Scanner) fillWork() bool {
	if len(s.work) > 0 {
		return true
	}
	n, err := readRetry(int(s.file.Fd()), s.scratch)
	if err != nil {
		s.finish(err)
		return false
	}
	if n <= 0 {
		s.finish(nil)
		return false
	}
	s.work = s.scratch[:n]
	return true
}
