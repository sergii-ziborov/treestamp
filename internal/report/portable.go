package report

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"unicode"
)

func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func RepoRelativeLocation(location string) string {
	if strings.HasPrefix(location, "<") && strings.HasSuffix(location, ">") {
		return location
	}
	if LooksAbsolute(location) {
		return ""
	}
	return strings.ReplaceAll(location, "\\", "/")
}

func LooksAbsolute(location string) bool {
	if strings.HasPrefix(location, "/") || strings.HasPrefix(location, "\\") {
		return true
	}
	return (len(location) > 1 && location[1] == ':' && unicode.IsLetter(rune(location[0]))) || filepath.IsAbs(location)
}
