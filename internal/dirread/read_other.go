//go:build !linux

package dirread

import "os"

// Read appends unsorted directory records. It does not call os.ReadDir,
// which sorts names and is slower than godirwalk on wide directories.
func Read(dir string, buf []Record) ([]Record, error) {
	dents, err := OSEntries(dir)
	if err != nil {
		return buf, err
	}
	for _, dent := range dents {
		buf = append(buf, Record{Name: dent.Name(), Type: dent.Type()})
	}
	return buf, nil
}

// ReadScratch ignores scratch; the OS read does not use a getdents buffer.
func ReadScratch(dir string, buf []Record, _ []byte) ([]Record, error) {
	return Read(dir, buf)
}

func OSEntries(dir string) ([]os.DirEntry, error) {
	return OSEntriesScratch(dir, nil)
}

func OSEntriesScratch(dir string, _ []byte) ([]os.DirEntry, error) {
	return unsortedEntries(dir)
}

func Names(dir string, _ []byte) ([]string, error) {
	dents, err := unsortedEntries(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(dents))
	for i, dent := range dents {
		out[i] = dent.Name()
	}
	return out, nil
}

func unsortedEntries(dir string) ([]os.DirEntry, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	dents, err := file.ReadDir(-1)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	return dents, closeErr
}
