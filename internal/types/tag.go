package types

import (
	"fmt"
	"strings"
)

const MaxTags = 3

func NormalizeTags(raw []string) ([]string, error) {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		s := strings.ToLower(strings.TrimSpace(r))
		if s == "" {
			continue
		}
		if !ValidSlug(s) {
			return nil, fmt.Errorf("invalid tag %q", s)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) > MaxTags {
		return nil, fmt.Errorf("at most %d tags", MaxTags)
	}
	return out, nil
}
