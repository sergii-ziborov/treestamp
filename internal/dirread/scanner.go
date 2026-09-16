package dirread

import "os"

// Scanner yields one child at a time from a single directory.
type Scanner struct {
	dir     string
	scratch []byte
	work    []byte
	file    *os.File
	name    string
	typ     os.FileMode
	info    os.FileInfo
	pending []os.DirEntry
	err     error
}

// NewScanner opens dirname. Pass a reusable scratch buffer on Linux.
func NewScanner(dir string, scratch []byte) (*Scanner, error) {
	file, err := openDir(dir)
	if err != nil {
		return nil, err
	}
	return &Scanner{dir: dir, scratch: EnsureScratch(scratch), file: file}, nil
}

func (s *Scanner) Name() string { return s.name }

func (s *Scanner) Dent() Dent {
	return Dent{dir: s.dir, name: s.name, typ: s.typ}
}

func (s *Scanner) Record() Record {
	return Record{Name: s.name, Type: s.typ, Info: s.info}
}

func (s *Scanner) Err() error {
	s.finish(s.err)
	return s.err
}

func (s *Scanner) Close() error { return s.Err() }

func (s *Scanner) finish(err error) {
	if s.file == nil {
		if s.err == nil {
			s.err = err
		}
		return
	}
	closeErr := s.file.Close()
	s.file = nil
	s.work = nil
	s.pending = nil
	if err != nil {
		s.err = err
		return
	}
	s.err = closeErr
}

// EnsureScratch grows a too-small Linux getdents buffer.
func EnsureScratch(buf []byte) []byte {
	need := MinimumScratch()
	if need == 0 || len(buf) >= need {
		return buf
	}
	return make([]byte, need)
}
