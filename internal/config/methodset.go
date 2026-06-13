package config

import "strings"

// MethodSet matches RPC method names against a pattern list: a plain entry
// matches exactly, an entry ending in '*' matches every method with that
// prefix ("execute*" covers execute, execute2, execute.raw.bg, …). Prefixes
// are plain string prefixes, not dot-segment aware — patterns should be
// written against the daemon's command families.
type MethodSet struct {
	exact    map[string]bool
	prefixes []string
}

func NewMethodSet(patterns []string) MethodSet {
	ms := MethodSet{exact: make(map[string]bool, len(patterns))}
	for _, p := range patterns {
		if name, ok := strings.CutSuffix(p, "*"); ok {
			ms.prefixes = append(ms.prefixes, name)
		} else {
			ms.exact[p] = true
		}
	}
	return ms
}

func (m MethodSet) Matches(method string) bool {
	if m.exact[method] {
		return true
	}
	for _, p := range m.prefixes {
		if strings.HasPrefix(method, p) {
			return true
		}
	}
	return false
}

func (m MethodSet) Empty() bool { return len(m.exact) == 0 && len(m.prefixes) == 0 }
