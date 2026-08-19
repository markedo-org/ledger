package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/githuboauth"
	"github.com/markedo-org/ledger/internal/types"
)

const (
	sessionCookie = "ledger_session"
	stateCookie   = "ledger_oauth_state"
	nextCookie    = "ledger_oauth_next"
	csrfCookie    = "ledger_csrf"
)

type AuthConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Allowlist    []string
	SecureCookie bool
	RequireHTML  bool
}

func (a AuthConfig) Enabled() bool {
	return a.ClientID != "" && a.ClientSecret != "" && a.CallbackURL != ""
}

func (a AuthConfig) HTMLLocked() bool {
	return a.RequireHTML || a.Enabled()
}

func (a AuthConfig) allowed(login string) bool {
	if len(a.Allowlist) == 0 {
		return false
	}
	login = strings.ToLower(login)
	for _, n := range a.Allowlist {
		if strings.ToLower(strings.TrimSpace(n)) == login {
			return true
		}
	}
	return false
}

type GitHub interface {
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (string, error)
	User(ctx context.Context, accessToken string) (githuboauth.User, error)
}

func (s *Server) htmlGate(c *gin.Context) {
	if !s.Auth.HTMLLocked() {
		c.Next()
		return
	}
	sess, ok := s.sessionFrom(c)
	if !ok {
		next := c.Request.URL.RequestURI()
		c.Redirect(http.StatusFound, "/login?next="+url.QueryEscape(next))
		c.Abort()
		return
	}
	owner := c.Param("owner")
	ledger := strings.TrimSuffix(c.Param("ledger"), ".md")
	if !sess.Covers(owner, ledger) {
		s.htmlErr(c, app.ErrForbidden)
		c.Abort()
		return
	}
	c.Next()
}

func (s *Server) sessionFrom(c *gin.Context) (types.Session, bool) {
	plain, err := c.Cookie(sessionCookie)
	if err != nil || plain == "" {
		return types.Session{}, false
	}
	sess, err := s.App.Session(c.Request.Context(), plain)
	if err != nil {
		return types.Session{}, false
	}
	return sess, true
}

func (s *Server) setCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", s.Auth.SecureCookie, true)
}

func (s *Server) page(c *gin.Context, data gin.H) gin.H {
	if data == nil {
		data = gin.H{}
	}
	data["HTMLLocked"] = s.Auth.HTMLLocked()
	data["GitHub"] = s.Auth.Enabled()
	data["CSRF"] = s.issueCSRF(c)
	if sess, ok := s.sessionFrom(c); ok {
		data["User"] = sess.Actor
		data["Operator"] = sess.IsOperator()
	}
	data["Magic"] = s.App.MailEnabled()
	data["SiteURL"] = strings.TrimRight(s.SiteURL, "/")
	return data
}

func (s *Server) login(c *gin.Context) {
	if sess, ok := s.sessionFrom(c); ok {
		c.Redirect(http.StatusFound, homePath(sess, c.Query("next")))
		return
	}
	if next := c.Query("next"); next != "" {
		s.setCookie(c, nextCookie, safeNext(next), 600)
	}
	s.renderLogin(c, "")
}

func (s *Server) loginPost(c *gin.Context) {
	if !s.checkCSRF(c) {
		s.renderLogin(c, "That sign-in could not be completed. Reload the page and try again.")
		return
	}
	token := strings.TrimSpace(c.PostForm("token"))
	if token == "" {
		s.renderLogin(c, "Paste a bearer token issued by this ledger.")
		return
	}
	_, plain, err := s.App.SessionFromAPIToken(c.Request.Context(), token)
	if err != nil {
		s.renderLogin(c, "That token was not accepted.")
		return
	}
	s.setCookie(c, sessionCookie, plain, int(app.SessionTTL.Seconds()))
	next := ""
	if n, err := c.Cookie(nextCookie); err == nil && n != "" {
		next = n
		s.setCookie(c, nextCookie, "", -1)
	}
	sess, err := s.App.Session(c.Request.Context(), plain)
	if err != nil {
		c.Redirect(http.StatusFound, homePath(types.Session{}, next))
		return
	}
	c.Redirect(http.StatusFound, homePath(sess, next))
}

func (s *Server) renderLogin(c *gin.Context, message string) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(c.Writer, "login", s.page(c, gin.H{
		"Title":   "Sign in · task-ledger",
		"Message": message,
		"Gate":    true,
	}))
}

func (s *Server) loginGitHub(c *gin.Context) {
	if !s.Auth.Enabled() || s.GitHub == nil {
		s.renderLogin(c, "GitHub OAuth is not configured on this process.")
		return
	}
	if sess, ok := s.sessionFrom(c); ok {
		c.Redirect(http.StatusFound, homePath(sess, ""))
		return
	}
	state, err := randomHex(16)
	if err != nil {
		c.String(http.StatusInternalServerError, "could not start login")
		return
	}
	s.setCookie(c, stateCookie, state, 600)
	c.Redirect(http.StatusFound, s.GitHub.AuthURL(state))
}

func (s *Server) logout(c *gin.Context) {
	if !s.checkCSRF(c) {
		c.String(http.StatusForbidden, "invalid csrf")
		return
	}
	if plain, err := c.Cookie(sessionCookie); err == nil {
		_ = s.App.DeleteSession(c.Request.Context(), plain)
	}
	s.setCookie(c, sessionCookie, "", -1)
	c.Redirect(http.StatusFound, "/")
}

func (s *Server) issueCSRF(c *gin.Context) string {
	if v, err := c.Cookie(csrfCookie); err == nil && v != "" {
		return v
	}
	tok, err := randomHex(16)
	if err != nil {
		return ""
	}
	s.setCookie(c, csrfCookie, tok, int(app.SessionTTL.Seconds()))
	return tok
}

func (s *Server) checkCSRF(c *gin.Context) bool {
	want, err := c.Cookie(csrfCookie)
	got := c.PostForm("csrf")
	if err != nil || want == "" || got == "" || len(want) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func (s *Server) githubCallback(c *gin.Context) {
	if !s.Auth.Enabled() || s.GitHub == nil {
		c.String(http.StatusNotFound, "oauth is not configured")
		return
	}
	want, err := c.Cookie(stateCookie)
	got := c.Query("state")
	if err != nil || want == "" || got == "" || want != got {
		c.String(http.StatusBadRequest, "invalid oauth state")
		return
	}
	s.setCookie(c, stateCookie, "", -1)
	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "missing code")
		return
	}
	token, err := s.GitHub.Exchange(c.Request.Context(), code)
	if err != nil {
		c.String(http.StatusBadGateway, "github token exchange failed")
		return
	}
	u, err := s.GitHub.User(c.Request.Context(), token)
	if err != nil {
		c.String(http.StatusBadGateway, "github user lookup failed")
		return
	}
	if !s.Auth.allowed(u.Login) {
		c.Status(http.StatusForbidden)
		c.Header("Content-Type", "text/html; charset=utf-8")
		_ = s.tmpl.ExecuteTemplate(c.Writer, "login", s.page(c, gin.H{
			"Title":   "Not allowed · task-ledger",
			"Message": "GitHub user " + u.Login + " is not allowed to sign in here.",
			"Gate":    true,
		}))
		return
	}
	sess, plain, err := s.App.CreateSession(c.Request.Context(), u.Login, strconv.FormatInt(u.ID, 10), u.Login, "", "", types.RoleOperator, "")
	if err != nil {
		c.String(http.StatusInternalServerError, "could not create session")
		return
	}
	s.setCookie(c, sessionCookie, plain, int(app.SessionTTL.Seconds()))
	next := ""
	if n, err := c.Cookie(nextCookie); err == nil && n != "" {
		next = n
		s.setCookie(c, nextCookie, "", -1)
	}
	c.Redirect(http.StatusFound, homePath(sess, next))
}

func homePath(sess types.Session, next string) string {
	n := safeNext(next)
	if n == "/admin" && !sess.IsOperator() {
		n = "/"
	}
	if n != "/" {
		return n
	}
	if sess.IsOperator() {
		return "/admin"
	}
	if sess.OwnerSlug != "" && sess.LedgerSlug != "" {
		return "/" + sess.OwnerSlug + "/" + sess.LedgerSlug
	}
	if sess.OwnerSlug != "" {
		return "/" + sess.OwnerSlug
	}
	return "/owners"
}

func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" {
		return "/"
	}
	return next
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
