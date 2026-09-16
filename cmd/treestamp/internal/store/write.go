package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func InsideRoot(root, output string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absOut, err := filepath.Abs(output)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absRoot, absOut)
	if err != nil {
		return false, err
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../"), nil
}

func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".treestamp-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := replaceFile(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}
