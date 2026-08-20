package web

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEveryResponseCarriesTheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityHeaders("default-src 'self'"))
	r.GET("/anything", func(c *gin.Context) { c.String(200, "hi") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/anything", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// The policy is built from the templates rather than written down, so that a
// change to the inline theme script cannot leave a stale hash behind. A stale
// hash looks fine to whoever made the change, because their browser has the
// theme cached, and drops the theme for everyone arriving fresh.
func TestPolicyHashesTheScriptTheTemplatesActuallyCarry(t *testing.T) {
	sub, err := fs.Sub(assets, "templates")
	if err != nil {
		t.Fatal(err)
	}
	csp, err := buildCSP(sub)
	if err != nil {
		t.Fatal(err)
	}

	layout, err := fs.ReadFile(sub, "layout.html")
	if err != nil {
		t.Fatal(err)
	}
	m := inlineScript.FindSubmatch(layout)
	if m == nil {
		t.Skip("no inline script in the layout any more")
	}
	sum := sha256.Sum256(m[1])
	want := fmt.Sprintf("'sha256-%s'", base64.StdEncoding.EncodeToString(sum[:]))

	if !strings.Contains(csp, want) {
		t.Fatalf("policy does not allow the layout's own inline script.\npolicy: %s\nwant:   %s", csp, want)
	}
	for _, must := range []string{
		"frame-ancestors 'none'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, must) {
			t.Errorf("policy is missing %q: %s", must, csp)
		}
	}
	// Allowing either of these back in would give up most of what the policy
	// is for, so say so out loud.
	for _, never := range []string{"unsafe-inline", "unsafe-eval"} {
		if strings.Contains(csp, never) {
			t.Errorf("policy allows %s: %s", never, csp)
		}
	}
}
