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

func openDir(path string) (*os.File, error) { return os.Open(path) }

func (s *Scanner) Scan() bool {
	if s.file == nil {
		return false
	}
	s.name, s.typ, s.info = "", 0, nil
	if len(s.pending) == 0 {
		entries, err := s.file.ReadDir(256)
		if err != nil || len(entries) == 0 {
			if err == io.EOF {
				err = nil
			}
			s.finish(err)
			return false
		}
		s.pending = entries
	}
	dent := s.pending[0]
	s.pending = s.pending[1:]
	s.name = dent.Name()
	s.typ = dent.Type()
	s.info, _ = dent.Info()
	return true
}
