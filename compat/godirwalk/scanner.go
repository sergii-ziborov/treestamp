package godirwalk

import "github.com/sergii-ziborov/treestamp"

// MinimumScratchBufferSize is the getdents floor on Unix and 0 on Windows.
var MinimumScratchBufferSize = treestamp.MinimumScratchBufferSize()

// ReadDirents lists one directory as Dirent values.
func ReadDirents(osDirname string, scratchBuffer []byte) (Dirents, error) {
	ents, err := treestamp.ReadDirentsScratch(osDirname, scratchBuffer)
	if err != nil {
		return nil, err
	}
	out := make(Dirents, 0, len(ents))
	for _, ent := range ents {
		out = append(out, &Dirent{name: ent.Name(), path: osDirname, modeType: ent.Type()})
	}
	return out, nil
}

// ReadDirnames lists child names.
func ReadDirnames(osDirname string, scratchBuffer []byte) ([]string, error) {
	return treestamp.ReadDirnames(osDirname, scratchBuffer)
}

// Scanner is a lazy single-directory enumerator.
type Scanner struct {
	inner *treestamp.DirScanner
	dir   string
}

// NewScanner opens dirname.
func NewScanner(osDirname string) (*Scanner, error) {
	return NewScannerWithScratchBuffer(osDirname, nil)
}

// NewScannerWithScratchBuffer opens dirname with a reusable buffer.
func NewScannerWithScratchBuffer(osDirname string, scratchBuffer []byte) (*Scanner, error) {
	inner, err := treestamp.NewDirScannerScratch(osDirname, scratchBuffer)
	if err != nil {
		return nil, err
	}
	return &Scanner{inner: inner, dir: osDirname}, nil
}

func (s *Scanner) Scan() bool {
	return s != nil && s.inner != nil && s.inner.Scan()
}

func (s *Scanner) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *Scanner) Dirent() (*Dirent, error) {
	if s == nil || s.inner == nil {
		return nil, errClosed
	}
	d, err := s.inner.Dirent()
	if err != nil {
		return nil, err
	}
	return &Dirent{name: d.Name(), path: s.dir, modeType: d.Type()}, nil
}

func (s *Scanner) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Err()
}

var errClosed = errText("scanner closed")

type errText string

func (e errText) Error() string { return string(e) }
