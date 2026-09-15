package ignore

// MatchGlob is the gitignore-compatible glob used by ignore rules and named types.
func MatchGlob(pattern, value string) bool {
	return globMatch(pattern, value)
}

func globMatch(pattern, value string) bool {
	if result, ok := matchBraces(pattern, value); ok {
		return result
	}
	pb := []byte(pattern)
	vb := []byte(value)
	width := len(vb) + 1
	memo := make([]byte, (len(pb)+1)*width)
	return matchesAt(pb, vb, 0, 0, width, memo)
}

func matchBraces(pattern, value string) (bool, bool) {
	start := braceStart(pattern)
	if start < 0 {
		return false, false
	}
	alts, end := braceAlts(pattern, start)
	if end < 0 {
		return false, false
	}
	for _, alt := range alts {
		if globMatch(pattern[:start]+alt+pattern[end+1:], value) {
			return true, true
		}
	}
	return false, true
}

func braceStart(pattern string) int {
	escaped, class := false, false
	for index, b := range []byte(pattern) {
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '[':
			class = true
		case ']':
			class = false
		case '{':
			if !class {
				return index
			}
		}
	}
	return -1
}

func braceAlts(pattern string, start int) ([]string, int) {
	bytes := []byte(pattern)
	depth, escaped, class := 1, false, false
	altStart := start + 1
	var alts []string
	end := -1
	for index := start + 1; index < len(bytes); index++ {
		b := bytes[index]
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '[':
			class = true
		case ']':
			class = false
		case '{':
			if !class {
				depth++
			}
		case '}':
			if !class {
				depth--
				if depth == 0 {
					alts = append(alts, pattern[altStart:index])
					end = index
				}
			}
		case ',':
			if !class && depth == 1 {
				alts = append(alts, pattern[altStart:index])
				altStart = index + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	return alts, end
}

func matchesAt(pattern, value []byte, pi, vi, width int, memo []byte) bool {
	slot := pi*width + vi
	switch memo[slot] {
	case 1:
		return false
	case 2:
		return true
	}
	var result bool
	if pi == len(pattern) {
		result = vi == len(value)
	} else if pattern[pi] == '*' && pi+1 < len(pattern) && pattern[pi+1] == '*' {
		result = doubleStar(pattern, value, pi, vi, width, memo)
	} else if pattern[pi] == '*' {
		result = matchesAt(pattern, value, pi+1, vi, width, memo) ||
			(vi < len(value) && value[vi] != '/' && matchesAt(pattern, value, pi, vi+1, width, memo))
	} else if pattern[pi] == '?' {
		result = vi < len(value) && value[vi] != '/' && matchesAt(pattern, value, pi+1, vi+1, width, memo)
	} else if pattern[pi] == '[' {
		if ok, next, classOK := characterClass(pattern, value, pi, vi); classOK {
			result = ok && matchesAt(pattern, value, next, vi+1, width, memo)
		} else {
			result = vi < len(value) && value[vi] == '[' && matchesAt(pattern, value, pi+1, vi+1, width, memo)
		}
	} else if pattern[pi] == '\\' && pi+1 < len(pattern) {
		result = vi < len(value) && value[vi] == pattern[pi+1] && matchesAt(pattern, value, pi+2, vi+1, width, memo)
	} else {
		result = vi < len(value) && value[vi] == pattern[pi] && matchesAt(pattern, value, pi+1, vi+1, width, memo)
	}
	if result {
		memo[slot] = 2
	} else {
		memo[slot] = 1
	}
	return result
}

func doubleStar(pattern, value []byte, pi, vi, width int, memo []byte) bool {
	next := pi + 2
	for next < len(pattern) && pattern[next] == '*' {
		next++
	}
	var skip bool
	if next < len(pattern) && pattern[next] == '/' {
		skip = matchesAt(pattern, value, next+1, vi, width, memo)
	} else {
		skip = matchesAt(pattern, value, next, vi, width, memo)
	}
	return skip || (vi < len(value) && matchesAt(pattern, value, pi, vi+1, width, memo))
}

func characterClass(pattern, value []byte, pi, vi int) (bool, int, bool) {
	if vi >= len(value) || value[vi] == '/' {
		return false, 0, false
	}
	candidate := value[vi]
	index := pi + 1
	negated := false
	if index < len(pattern) && (pattern[index] == '!' || pattern[index] == '^') {
		negated = true
		index++
	}
	matched := false
	hasMember := false
	for index < len(pattern) {
		if pattern[index] == ']' && hasMember {
			return matched != negated, index + 1, true
		}
		hasMember = true
		start, consumed, ok := escapedMember(pattern, index)
		if !ok {
			return false, 0, false
		}
		index += consumed
		if index < len(pattern) && pattern[index] == '-' && index+1 < len(pattern) && pattern[index+1] != ']' {
			end, endConsumed, endOK := escapedMember(pattern, index+1)
			if !endOK {
				return false, 0, false
			}
			matched = matched || (start <= candidate && candidate <= end)
			index += 1 + endConsumed
		} else {
			matched = matched || candidate == start
		}
	}
	return false, 0, false
}

func escapedMember(pattern []byte, index int) (byte, int, bool) {
	if index >= len(pattern) {
		return 0, 0, false
	}
	if pattern[index] == '\\' {
		if index+1 >= len(pattern) {
			return 0, 0, false
		}
		return pattern[index+1], 2, true
	}
	return pattern[index], 1, true
}
