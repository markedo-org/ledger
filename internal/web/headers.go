package web

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// The board renders task text that people wrote, and html/template escapes it,
// so a content policy is a second wall rather than a patch over a known hole.
// It lives in the app and not in a proxy because a self-hoster running the
// binary gets nothing from our nginx, and because a template that gains a
// script and a policy that must allow it should move in the same commit.
func securityHeaders(csp string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", csp)
		c.Next()
	}
}

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// buildCSP hashes whatever inline script the templates carry rather than
// naming a hash in a constant. A constant would go stale the first time
// somebody edits the theme script, and the failure would be a page that looks
// fine to whoever changed it and loses its theme for everyone else.
func buildCSP(templates fs.FS) (string, error) {
	hashes, err := inlineScriptHashes(templates)
	if err != nil {
		return "", err
	}

	script := "'self'"
	if len(hashes) > 0 {
		script += " " + strings.Join(hashes, " ")
	}

	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
		"img-src 'self' data:",
		"style-src 'self' https://fonts.bunny.net",
		"font-src 'self' https://fonts.bunny.net",
		"connect-src 'self'",
		"script-src " + script,
	}, "; "), nil
}

func inlineScriptHashes(templates fs.FS) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	entries, err := fs.ReadDir(templates, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := fs.ReadFile(templates, e.Name())
		if err != nil {
			return nil, err
		}
		for _, m := range inlineScript.FindAllSubmatch(b, -1) {
			sum := sha256.Sum256(m[1])
			h := fmt.Sprintf("'sha256-%s'", base64.StdEncoding.EncodeToString(sum[:]))
			if !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	return out, nil
}
