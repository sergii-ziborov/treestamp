//go:build !linux

package dirread

import "os"

// Read appends directory records using os.ReadDir.
func Read(dir string, buf []Record) ([]Record, error) {
	dents, err := os.ReadDir(dir)
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
	return os.ReadDir(dir)
}

func OSEntriesScratch(dir string, _ []byte) ([]os.DirEntry, error) {
	return os.ReadDir(dir)
}

func Names(dir string, _ []byte) ([]string, error) {
	dents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(dents))
	for i, dent := range dents {
		out[i] = dent.Name()
	}
	return out, nil
}
