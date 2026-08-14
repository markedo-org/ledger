package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	RootLogin = "login"
	RootURL   = "url"
	RootFile  = "file"
)

// RootConfig is GET / with no extra path. Env, not a UI.
type RootConfig struct {
	Mode string
	URL  string
	File string
}

func ParseRoot(mode, rawURL, file string) (RootConfig, error) {
	r := RootConfig{
		Mode: strings.ToLower(strings.TrimSpace(mode)),
		URL:  strings.TrimSpace(rawURL),
		File: strings.TrimSpace(file),
	}
	if r.Mode == "" {
		r.Mode = RootLogin
	}
	switch r.Mode {
	case RootLogin:
		return r, nil
	case RootURL:
		u, err := url.Parse(r.URL)
		if err != nil || r.URL == "" || u.Host == "" || u.User != nil {
			return r, fmt.Errorf("LEDGER_ROOT=url needs LEDGER_ROOT_URL as an absolute http(s) URL")
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return r, fmt.Errorf("LEDGER_ROOT_URL scheme must be http or https")
		}
		if u.Fragment != "" {
			return r, fmt.Errorf("LEDGER_ROOT_URL must not include a fragment")
		}
		return r, nil
	case RootFile:
		if r.File == "" {
			return r, fmt.Errorf("LEDGER_ROOT=file needs LEDGER_ROOT_FILE")
		}
		st, err := os.Stat(r.File)
		if err != nil {
			return r, fmt.Errorf("LEDGER_ROOT_FILE: %w", err)
		}
		if st.IsDir() {
			return r, fmt.Errorf("LEDGER_ROOT_FILE must be a file")
		}
		return r, nil
	default:
		return r, fmt.Errorf("LEDGER_ROOT must be login, url, or file")
	}
}

func (s *Server) root(c *gin.Context) {
	if sess, ok := s.sessionFrom(c); ok {
		c.Redirect(http.StatusFound, homePath(sess, ""))
		return
	}
	switch s.Root.Mode {
	case RootURL:
		c.Redirect(http.StatusMovedPermanently, s.Root.URL)
	case RootFile:
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.File(s.Root.File)
	default:
		c.Redirect(http.StatusFound, "/login")
	}
}
