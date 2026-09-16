package selection

import (
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
)

type Contain int

const (
	ContainMaybe Contain = iota
	ContainNo
)

// CompiledSelection is an immutable query-scope plan shared by Full, Changed, and Watch.
type CompiledSelection struct {
	prefixes []string
	open     bool
}

func Compile(cfg Config) CompiledSelection {
	prefixes, open := ignore.ScopePrefixes(cfg.OverrideRules, cfg.IgnoreCase)
	return CompiledSelection{prefixes: prefixes, open: open}
}

func (c CompiledSelection) MayContain(rel string) Contain {
	if c.open || len(c.prefixes) == 0 {
		return ContainMaybe
	}
	rel = pathx.Slash(rel)
	if rel == "" {
		return ContainMaybe
	}
	for _, prefix := range c.prefixes {
		if rel == prefix || strings.HasPrefix(prefix, rel+"/") || strings.HasPrefix(rel, prefix+"/") {
			return ContainMaybe
		}
	}
	return ContainNo
}

// Explanation is a diagnostic for one path using the same selection rules as Scan.
type Explanation struct {
	Relative, Outcome, Reason, Source, Pattern string
	Line                                       int
}
