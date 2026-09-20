//go:build !linux

package dirread

import (
	"io"
	"os"
)

// Read appends unsorted directory records. It does not call os.ReadDir,
// which sorts names and is slower than godirwalk on wide directories.
func Read(dir string, buf []Record) ([]Record, error) {
	dents, err := OSEntries(dir)
	if err != nil {
		return buf, err
	}
	for _, dent := range dents {
		info, _ := dent.Info()
		buf = append(buf, Record{Name: dent.Name(), Type: dent.Type(), Info: info})
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

// Visit streams children in ReadDir batches so SkipAll can close early.
func Visit(dir string, _ []byte, fn func(name string, typ os.FileMode, dent os.DirEntry) error) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	for {
		dents, err := file.ReadDir(256)
		for _, dent := range dents {
			if cbErr := fn(dent.Name(), dent.Type(), dent); cbErr != nil {
				return cbErr
			}
		}
		if err == io.EOF || (err == nil && len(dents) == 0) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func OSEntriesScratch(dir string, _ []byte) ([]os.DirEntry, error) {
	return unsortedEntries(dir)
}

func Names(dir string, _ []byte) ([]string, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	names, err := file.Readdirnames(-1)
	closeErr := file.Close()
	if err != nil {
		return names, err
	}
	return names, closeErr
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
