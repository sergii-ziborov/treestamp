//go:build !linux

package dirread

import (
	"io"
	"os"
	"runtime"
)

// MinimumScratch is 0 on Windows and the page size on other non-Linux hosts.
func MinimumScratch() int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return os.Getpagesize()
}

func (s *Scanner) Scan() bool {
	if s.file == nil {
		return false
	}
	s.name, s.typ, s.info = "", 0, nil
	entries, err := s.file.ReadDir(1)
	if err != nil || len(entries) == 0 {
		if err == io.EOF {
			err = nil
		}
		s.finish(err)
		return false
	}
	dent := entries[0]
	s.name = dent.Name()
	s.typ = dent.Type()
	s.info, _ = dent.Info()
	return true
}
