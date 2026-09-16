package render

import (
	"io"
	"os"
	"strings"
)

type Palette struct {
	Bold, Dim, Green, Yellow, Red, Cyan, Reset string
}

func Detect(w io.Writer, mode string) Palette {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "never" || os.Getenv("NO_COLOR") != "" {
		return Palette{}
	}
	if mode != "always" && !isTTY(w) {
		return Palette{}
	}
	return Palette{
		Bold: "\x1b[1m", Dim: "\x1b[2m", Green: "\x1b[32m",
		Yellow: "\x1b[33m", Red: "\x1b[31m", Cyan: "\x1b[36m", Reset: "\x1b[0m",
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (p Palette) Paint(code, text string) string {
	if code == "" {
		return text
	}
	return code + text + p.Reset
}
