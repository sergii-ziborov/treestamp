package ignore

import (
	"fmt"
	"strings"
)

type ruleAction int

const (
	actionIgnore ruleAction = iota
	actionInclude
)

type ruleTarget int

const (
	targetAny ruleTarget = iota
	targetDirectory
)

type ruleScope int

const (
	scopeAnywhere ruleScope = iota
	scopePath
	scopeAnchored
)

type ignoreRule struct {
	pattern string
	action  ruleAction
	target  ruleTarget
	scope   ruleScope
	ci      bool
}

func parseFile(text string, caseInsensitive bool) ([]ignoreRule, []string) {
	text = strings.TrimPrefix(text, "\ufeff")
	var rules []ignoreRule
	var errs []string
	for i, raw := range strings.Split(text, "\n") {
		rule, err := parseRule(raw, caseInsensitive)
		if err != "" {
			errs = append(errs, fmt.Sprintf("line %d: %s", i+1, err))
			continue
		}
		if rule != nil {
			rules = append(rules, *rule)
		}
	}
	return rules, errs
}

func parseRule(raw string, caseInsensitive bool) (*ignoreRule, string) {
	line := trimUnescapedTrailingSpaces(strings.TrimRight(raw, "\r"))
	if line == "" {
		return nil, ""
	}
	escapedPrefix := strings.HasPrefix(line, "\\#") || strings.HasPrefix(line, "\\!")
	if escapedPrefix {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return nil, ""
	}
	negated := !escapedPrefix && strings.HasPrefix(line, "!")
	if negated {
		line = line[1:]
	}
	escapedDirectory := strings.HasSuffix(line, "\\/")
	target := targetAny
	if strings.HasSuffix(line, "/") {
		line = strings.TrimSuffix(line, "/")
		if escapedDirectory {
			line = strings.TrimSuffix(line, "\\")
		}
		target = targetDirectory
	}
	anchored := strings.HasPrefix(line, "/")
	if anchored {
		line = line[1:]
	}
	if line == "" {
		return nil, ""
	}
	if err := validatePattern(line); err != "" {
		return nil, err
	}
	if caseInsensitive {
		line = strings.ToLower(line)
	}
	scope := scopeAnywhere
	if anchored {
		scope = scopeAnchored
	} else if strings.Contains(line, "/") {
		scope = scopePath
	}
	action := actionIgnore
	if negated {
		action = actionInclude
	}
	return &ignoreRule{
		pattern: line,
		action:  action,
		target:  target,
		scope:   scope,
		ci:      caseInsensitive,
	}, ""
}

func validatePattern(pattern string) string {
	escaped := false
	bracket := false
	brace := 0
	for i := 0; i < len(pattern); i++ {
		b := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '[':
			if !bracket {
				bracket = true
			}
		case ']':
			if bracket {
				bracket = false
			}
		case '{':
			if !bracket {
				brace++
			}
		case '}':
			if !bracket {
				brace--
				if brace < 0 {
					return "unmatched closing brace"
				}
			}
		}
	}
	if brace != 0 {
		return "unclosed alternation"
	}
	return ""
}

func trimUnescapedTrailingSpaces(line string) string {
	for strings.HasSuffix(line, " ") {
		without := strings.TrimSuffix(line, " ")
		slashes := 0
		for i := len(without) - 1; i >= 0 && without[i] == '\\'; i-- {
			slashes++
		}
		if slashes%2 == 1 {
			break
		}
		line = without
	}
	return line
}

func (r ignoreRule) matchesExact(path string, isDir bool) bool {
	if r.target == targetDirectory && !isDir {
		return false
	}
	value := path
	if r.scope == scopeAnywhere {
		if i := strings.LastIndexByte(path, '/'); i >= 0 {
			value = path[i+1:]
		}
	}
	if r.ci {
		value = asciiLower(value)
	}
	return matchPattern(r.pattern, value, r.ci)
}

func matchPattern(pattern, value string, ci bool) bool {
	if !hasMeta(pattern) {
		if ci {
			return strings.EqualFold(pattern, value)
		}
		return pattern == value
	}
	if strings.HasPrefix(pattern, "*") && !hasMeta(pattern[1:]) {
		suf := pattern[1:]
		if ci {
			return strings.HasSuffix(asciiLower(value), suf)
		}
		return strings.HasSuffix(value, suf)
	}
	if strings.HasSuffix(pattern, "*") && !hasMeta(pattern[:len(pattern)-1]) {
		pre := pattern[:len(pattern)-1]
		if ci {
			return strings.HasPrefix(asciiLower(value), pre)
		}
		return strings.HasPrefix(value, pre)
	}
	return globMatch(pattern, value)
}

func hasMeta(value string) bool {
	return strings.ContainsAny(value, "*?[{{\\")
}

func asciiLower(value string) string {
	b := []byte(value)
	for i, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			b[i] = ch + 32
		}
	}
	return string(b)
}
