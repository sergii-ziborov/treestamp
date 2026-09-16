package ignore

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
)

var gitModulePath = regexp.MustCompile(`^\s*path\s*=\s*(.*)`)

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
	paths := extractGitModuleFolders(string(body))
	if len(paths) > 0 {
		rules, _ := parseFile(strings.Join(paths, "\n"), e.caseInsensitive)
		if len(rules) > 0 {
			e.layers[rankGitModules] = &layer{base: base, rules: rules, parent: e.layers[rankGitModules]}
		}
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
		matches := gitModulePath.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		path := strings.ReplaceAll(strings.TrimSpace(matches[1]), "\\", "/")
		if path == "" || path == "." || path == ".." {
			continue
		}
		out = append(out, path)
	}
	return out
}
