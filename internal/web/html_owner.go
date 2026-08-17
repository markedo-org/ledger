package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

func (s *Server) tokenFromSession(c *gin.Context, sess types.Session) (types.Token, error) {
	tok := types.Token{
		Actor:      sess.Actor,
		OwnerSlug:  sess.OwnerSlug,
		LedgerSlug: sess.LedgerSlug,
	}
	if sess.IsOperator() {
		tok.Role = types.RoleOperator
		return tok, nil
	}
	tok.Role = types.RoleAdmin
	o, err := s.App.Store.OwnerBySlug(c.Request.Context(), sess.OwnerSlug)
	if err != nil {
		return tok, err
	}
	tok.OwnerID = o.ID
	if sess.LedgerSlug != "" {
		l, err := s.App.PublicLedger(c.Request.Context(), sess.OwnerSlug, sess.LedgerSlug)
		if err != nil {
			return tok, err
		}
		tok.LedgerID = l.ID
	}
	return tok, nil
}

// canManageLedger is true for an operator, an owner-wide session, or a
// session bound to that ledger. An empty ledger means owner-wide only.
func canManageLedger(sess types.Session, ok bool, owner, ledger string) bool {
	if !ok {
		return false
	}
	if sess.IsOperator() {
		return true
	}
	if sess.OwnerSlug != owner {
		return false
	}
	if sess.LedgerSlug == "" {
		return true
	}
	return ledger != "" && sess.LedgerSlug == ledger
}

func (s *Server) htmlLedgerSettings(c *gin.Context) {
	s.renderLedgerSettings(c, "")
}

func (s *Server) htmlLedgerSettingsPost(c *gin.Context) {
	if !s.checkCSRF(c) {
		s.renderLedgerSettings(c, "That save could not be completed. Reload the page and try again.")
		return
	}
	owner, ledger := c.Param("owner"), c.Param("ledger")
	sess, ok := s.sessionFrom(c)
	if !canManageLedger(sess, ok, owner, ledger) {
		s.htmlErr(c, app.ErrForbidden)
		return
	}
	title := strings.TrimSpace(c.PostForm("title"))
	archive, err := strconv.Atoi(strings.TrimSpace(c.PostForm("archive_done_after_days")))
	if err != nil {
		s.renderLedgerSettings(c, "Archive days must be a number. Use 0 to never hide DONE.")
		return
	}
	purge, err := strconv.Atoi(strings.TrimSpace(c.PostForm("purge_done_after_days")))
	if err != nil {
		s.renderLedgerSettings(c, "Purge days must be a number. Use 0 to never delete DONE.")
		return
	}
	tok, err := s.tokenFromSession(c, sess)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	if _, err := s.App.PatchLedger(c.Request.Context(), tok, owner, ledger, &title, &archive, &purge); err != nil {
		s.renderLedgerSettings(c, strings.TrimPrefix(err.Error(), "invalid: "))
		return
	}
	c.Redirect(http.StatusSeeOther, "/"+owner+"/"+ledger+"/settings?saved=1")
}

func (s *Server) renderLedgerSettings(c *gin.Context, message string) {
	owner, ledger := c.Param("owner"), c.Param("ledger")
	sess, ok := s.sessionFrom(c)
	if !canManageLedger(sess, ok, owner, ledger) {
		s.htmlErr(c, app.ErrForbidden)
		return
	}
	l, err := s.App.PublicLedger(c.Request.Context(), owner, ledger)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	archive, purge := s.App.EffectiveRetention(l)
	if message == "" && c.Query("saved") == "1" {
		message = "Saved."
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(c.Writer, "settings", s.page(c, gin.H{
		"Title":       owner + "/" + ledger + " settings",
		"Owner":       owner,
		"Ledger":      l.Slug,
		"LedgerTitle": l.Title,
		"Archive":     archive,
		"Purge":       purge,
		"Message":     message,
		"CanManage":   true,
	}))
}
