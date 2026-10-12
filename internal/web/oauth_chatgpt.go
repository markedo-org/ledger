package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

// ChatGPT publishes its OAuth client as a URL. We fetch only those URLs.
// An open client_id would be a server-side request, and an unchecked
// redirect_uri would be an open redirect carrying an authorization code.
func chatgptClientID(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "chatgpt.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return false
	}
	if u.Path == "/oauth/client.json" {
		return true
	}
	rest, ok := strings.CutPrefix(u.Path, "/oauth/")
	if !ok || !strings.HasSuffix(rest, "/client.json") {
		return false
	}
	callback := strings.TrimSuffix(rest, "/client.json")
	if callback == "" || strings.Contains(callback, "/") {
		return false
	}
	for _, r := range callback {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func chatgptRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "chatgpt.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return false
	}
	if u.Path == "/connector_platform_oauth_redirect" {
		return true
	}
	rest, ok := strings.CutPrefix(u.Path, "/connector/oauth/")
	if !ok || rest == "" || strings.Contains(rest, "/") {
		return false
	}
	for _, r := range rest {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Server) oauthRedirects(ctx context.Context, clientID string) ([]string, error) {
	if !chatgptClientID(clientID) {
		return nil, app.ErrForbidden
	}
	if s.OAuthClients != nil {
		return s.OAuthClients(ctx, clientID)
	}
	return fetchClientRedirects(ctx, clientID)
}

func fetchClientRedirects(ctx context.Context, clientID string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 2 || req.URL.Host != "chatgpt.com" || req.URL.Scheme != "https" {
			return app.ErrForbidden
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, app.ErrForbidden
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	var doc struct {
		ClientID     string   `json:"client_id"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc.ClientID != clientID {
		return nil, app.ErrForbidden
	}
	return doc.RedirectURIs, nil
}

func redirectAllowed(uri string, published []string) bool {
	if !chatgptRedirect(uri) {
		return false
	}
	for _, got := range published {
		if got == uri {
			return true
		}
	}
	return false
}

func (s *Server) oauthProtectedResource(c *gin.Context) {
	res := s.App.OAuthResource()
	if res == "" {
		c.String(http.StatusNotFound, "OAuth is not configured")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"resource":                 res,
		"authorization_servers":    []string{s.App.Issuer()},
		"scopes_supported":         []string{"ledger:read", "offline_access"},
		"bearer_methods_supported": []string{"header"},
	})
}

func (s *Server) oauthAuthorizationServer(c *gin.Context) {
	iss := s.App.Issuer()
	if iss == "" {
		c.String(http.StatusNotFound, "OAuth is not configured")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                         iss,
		"authorization_endpoint":                         iss + "/oauth/authorize",
		"token_endpoint":                                 iss + "/oauth/token",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"client_id_metadata_document_supported":          true,
		"scopes_supported":                               []string{"ledger:read", "offline_access"},
		"authorization_response_iss_parameter_supported": true,
	})
}

func (s *Server) oauthAuthorize(c *gin.Context) {
	if s.App.OAuthResource() == "" {
		c.String(http.StatusNotFound, "OAuth is not configured")
		return
	}
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	published, err := s.oauthRedirects(c.Request.Context(), clientID)
	if err != nil || !redirectAllowed(redirectURI, published) {
		s.renderOAuth(c, "That client cannot be connected.", nil, "")
		return
	}
	if c.Query("response_type") != "code" || c.Query("code_challenge_method") != "S256" {
		s.oauthErrorRedirect(c, redirectURI, c.Query("state"), "invalid_request")
		return
	}
	sess, ok := s.sessionFrom(c)
	if !ok {
		next := c.Request.URL.RequestURI()
		c.Redirect(http.StatusFound, "/login?next="+url.QueryEscape(next))
		return
	}
	id, err := s.App.BeginOAuth(c.Request.Context(), app.OAuthStart{
		ClientID: clientID, RedirectURI: redirectURI,
		CodeChallenge: c.Query("code_challenge"), Resource: c.Query("resource"),
		Scope: c.Query("scope"), State: c.Query("state"),
	})
	if err != nil {
		s.oauthErrorRedirect(c, redirectURI, c.Query("state"), "invalid_request")
		return
	}
	tok, err := s.tokenFromSession(c, sess)
	if err != nil || tok.IsOperator() || tok.Role != types.RoleAdmin {
		s.renderOAuth(c, "Sign in with the owner admin token for the ledger you want ChatGPT to read. A write token cannot approve this.", nil, "")
		return
	}
	ledgers, err := s.App.ListLedgers(c.Request.Context(), tok, tok.OwnerSlug)
	if err != nil || len(ledgers) == 0 {
		s.renderOAuth(c, "This sign-in has no ledger to share.", nil, "")
		return
	}
	if sess.LedgerSlug != "" {
		var filtered []types.Ledger
		for _, l := range ledgers {
			if l.Slug == sess.LedgerSlug {
				filtered = append(filtered, l)
			}
		}
		ledgers = filtered
	}
	if len(ledgers) == 0 {
		s.renderOAuth(c, "This sign-in has no ledger to share.", nil, "")
		return
	}
	s.renderOAuth(c, "", ledgers, id)
}

func (s *Server) oauthAuthorizePost(c *gin.Context) {
	s.allowChatGPTReturn(c)
	if !s.checkCSRF(c) {
		s.renderOAuth(c, "That approval could not be completed. Start the connection again.", nil, "")
		return
	}
	pendingID := c.PostForm("pending")
	if c.PostForm("deny") != "" {
		loc, err := s.App.DenyOAuth(c.Request.Context(), pendingID)
		if err != nil {
			s.renderOAuth(c, "That connection request has expired. Start it again from ChatGPT.", nil, "")
			return
		}
		c.Redirect(http.StatusFound, loc)
		return
	}
	sess, ok := s.sessionFrom(c)
	if !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	tok, err := s.tokenFromSession(c, sess)
	if err != nil {
		s.renderOAuth(c, "Sign in with the owner admin token.", nil, "")
		return
	}
	loc, err := s.App.ConsentOAuth(c.Request.Context(), tok, pendingID, c.PostForm("ledger"))
	if err != nil {
		// A second click lands here after the first one already minted the
		// code and deleted the pending row. Saying the ledger could not be
		// shared sent people back to the start of a grant that had worked.
		msg := "That ledger could not be shared. Start the connection again."
		if errors.Is(err, app.ErrUnauthorized) {
			msg = "That approval was already sent to ChatGPT. If it did not connect, start the connection again from ChatGPT."
		}
		log.Printf("oauth consent: %v", err)
		s.renderOAuth(c, msg, nil, "")
		return
	}
	c.Redirect(http.StatusFound, loc)
}

func (s *Server) oauthErrorRedirect(c *gin.Context, redirectURI, state, code string) {
	u, err := url.Parse(redirectURI)
	if err != nil || !chatgptRedirect(redirectURI) {
		s.renderOAuth(c, "That client cannot be connected.", nil, "")
		return
	}
	q := u.Query()
	q.Set("error", code)
	if state != "" {
		q.Set("state", state)
	}
	q.Set("iss", s.App.Issuer())
	u.RawQuery = q.Encode()
	c.Redirect(http.StatusFound, u.String())
}

func (s *Server) oauthToken(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		oauthTokenError(c, "invalid_request")
		return
	}
	resource := c.Request.PostForm.Get("resource")
	clientID := c.Request.PostForm.Get("client_id")
	var out app.OAuthTokens
	var err error
	switch c.Request.PostForm.Get("grant_type") {
	case "authorization_code":
		out, err = s.App.ExchangeOAuthCode(c.Request.Context(),
			c.Request.PostForm.Get("code"),
			c.Request.PostForm.Get("code_verifier"),
			clientID,
			c.Request.PostForm.Get("redirect_uri"),
			resource)
	case "refresh_token":
		out, err = s.App.RefreshOAuth(c.Request.Context(),
			c.Request.PostForm.Get("refresh_token"),
			clientID,
			resource)
	default:
		oauthTokenError(c, "unsupported_grant_type")
		return
	}
	if err != nil {
		oauthTokenError(c, "invalid_grant")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token":  out.Access,
		"token_type":    "Bearer",
		"expires_in":    out.Expires,
		"refresh_token": out.Refresh,
		"scope":         out.Scope,
	})
}

func oauthTokenError(c *gin.Context, code string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusBadRequest, gin.H{"error": code})
}

// allowChatGPTReturn widens form-action on the consent page only. Chrome
// applies that directive to the whole redirect chain after a form submit, so
// form-action 'self' drops the 302 to chatgpt.com on the floor. The code is
// minted, the browser never delivers it, and the next click finds no pending
// grant. The global policy stays 'self'.
func (s *Server) allowChatGPTReturn(c *gin.Context) {
	h := c.Writer.Header()
	csp := h.Get("Content-Security-Policy")
	const from = "form-action 'self'"
	const to = "form-action 'self' https://chatgpt.com"
	if strings.Contains(csp, to) {
		return
	}
	h.Set("Content-Security-Policy", strings.Replace(csp, from, to, 1))
}

func (s *Server) renderOAuth(c *gin.Context, message string, ledgers []types.Ledger, pending string) {
	s.allowChatGPTReturn(c)
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(c.Writer, "oauth", s.page(c, gin.H{
		"Title":   "Share a ledger · task-ledger",
		"Message": message,
		"Ledgers": ledgers,
		"Pending": pending,
		"Gate":    true,
	}))
}
