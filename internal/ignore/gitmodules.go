package ignore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
)

func (e *Engine) loadGitModules(directory, base string) []string {
	path := filepath.Join(directory, ".gitmodules")
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []string{path + ": ignore file is a symbolic link"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	return e.applyGitModules(base, body)
}

func (e *Engine) applyGitModules(base string, body []byte) []string {
	for _, path := range extractGitModuleFolders(string(body)) {
		if base != "" {
			path = base + "/" + path
		}
		e.modules = append(append([]string(nil), e.modules...), path)
	}
	location := ".gitmodules"
	if base != "" {
		location = base + "/.gitmodules"
	}
	e.sources = append(e.sources, Source{
		Kind: SourceGitModules, Location: location, ContentHash: hashx.SHA256Prefix(body),
	})
	return nil
}

func extractGitModuleFolders(input string) []string {
	var out []string
	for _, line := range strings.Split(input, "\n") {
		path, ok := gitModulePathLine(line)
		if !ok {
			continue
		}
		out = append(out, path)
	}
	return out
}

func gitModulePathLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
		return "", false
	}
	key, raw, ok := splitGitAssign(line)
	if !ok || key != "path" {
		return "", false
	}
	path := strings.Trim(strings.ReplaceAll(decodeGitConfigValue(raw), "\\", "/"), "/")
	if path == "" || path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return "", false
	}
	return path, true
}

func splitGitAssign(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	return key, strings.TrimSpace(line[i+1:]), key != ""
}

func decodeGitConfigValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if v[0] == '"' {
		return unquoteGit(v)
	}
	if i := strings.IndexAny(v, "#;"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

func unquoteGit(v string) string {
	var b strings.Builder
	escaped := false
	for i := 1; i < len(v); i++ {
		c := v[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			return b.String()
		}
		b.WriteByte(c)
	}
	return b.String()
}
