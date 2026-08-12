package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	slugRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	actorRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	handleRe = regexp.MustCompile(`^([A-Za-z])-(\d{1,9})$`)
)

func ValidSlug(s string) bool {
	return slugRe.MatchString(s)
}

func ValidActor(s string) bool {
	return actorRe.MatchString(strings.ToLower(s))
}

func ValidPrefix(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return c >= 'A' && c <= 'Z'
}

func FormatHandle(prefix string, n int) string {
	return fmt.Sprintf("%s-%03d", prefix, n)
}

func ParseHandle(s string) (prefix string, n int, err error) {
	m := handleRe.FindStringSubmatch(s)
	if m == nil {
		return "", 0, fmt.Errorf("invalid handle %q", s)
	}
	n, err = strconv.Atoi(m[2])
	if err != nil || n < 1 {
		return "", 0, fmt.Errorf("invalid handle %q", s)
	}
	return strings.ToUpper(m[1]), n, nil
}
