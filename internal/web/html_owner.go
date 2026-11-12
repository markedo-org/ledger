package web

import (
	"errors"
	"log"
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
	tok.Role = sess.Role
	if tok.Role == "" {
		tok.Role = types.RoleWrite
	}
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

// canManageLedger is true for an operator, an owner-wide admin session, or an
// admin session bound to that ledger. An empty ledger means owner-wide only.
// Settings change retention, which deletes work, so a write session is refused
// here exactly as it is on the REST route.
func canManageLedger(sess types.Session, ok bool, owner, ledger string) bool {
	if !ok {
		return false
	}
	// No operator branch: a ledger's title and retention belong to its owner.
	// Rendering the form for a host session only invited a 403 on submit.
	if sess.Role != types.RoleAdmin {
		return false
	}
	if sess.OwnerSlug != owner {
		return false
	}
	if sess.LedgerSlug == "" {
		return true
	}
	return ledger != "" && sess.LedgerSlug == ledger
}

func (s *Server) htmlOwnerPost(c *gin.Context) {
	owner := c.Param("owner")
	slug := strings.ToLower(strings.TrimSpace(c.PostForm("slug")))
	title := strings.TrimSpace(c.PostForm("title"))
	if !s.checkCSRF(c) {
		s.renderOwnerMsg(c, owner, "That form could not be completed. Reload the page and try again.", slug, title)
		return
	}
	sess, ok := s.sessionFrom(c)
	if !canManageLedger(sess, ok, owner, "") {
		s.htmlErr(c, app.ErrForbidden)
		return
	}
	tok, err := s.tokenFromSession(c, sess)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	l, err := s.App.CreateLedger(c.Request.Context(), tok, owner, app.CreateLedgerInput{Slug: slug, Title: title})
	if err != nil {
		s.renderOwnerMsg(c, owner, createLedgerMessage(err), slug, title)
		return
	}
	issued, err := s.App.MintProjectWrite(c.Request.Context(), tok, owner, l.Slug, app.ProjectActor(tok, ""))
	if err != nil {
		log.Printf("mint project write for %s/%s: %v", owner, l.Slug, err)
	}
	plain := ""
	if issued != nil {
		plain = issued.Plain
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(c.Writer, "created", s.page(c, gin.H{
		"Title":      owner + "/" + l.Slug,
		"Owner":      owner,
		"Ledger":     l.Slug,
		"WriteToken": plain,
	})); err != nil {
		log.Printf("template: %v", err)
	}
}

func createLedgerMessage(err error) string {
	switch {
	case errors.Is(err, app.ErrConflict):
		return "That ledger name is taken."
	case errors.Is(err, app.ErrPolicy):
		return "The cap is full."
	case errors.Is(err, app.ErrInvalid):
		return "Ledger must be lowercase letters, digits, or hyphens."
	default:
		return "Could not create the ledger."
	}
}

func (s *Server) renderOwnerMsg(c *gin.Context, owner, message, slug, title string) {
	o, ledgers, err := s.App.PublicOwner(c.Request.Context(), owner)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	s.renderOwner(c, o, ledgers, message, slug, title)
}

func (s *Server) renderOwner(c *gin.Context, o types.Owner, ledgers []app.LedgerInfo, message, formSlug, formTitle string) {
	owner := o.Slug
	used := len(ledgers)
	sess, signedIn := s.sessionFrom(c)
	if signedIn && sess.LedgerSlug != "" {
		filtered := ledgers[:0]
		for _, l := range ledgers {
			if l.Slug == sess.LedgerSlug {
				filtered = append(filtered, l)
			}
		}
		ledgers = filtered
	}
	unused := 0
	if o.MaxLedgers > 0 && o.MaxLedgers > used {
		unused = o.MaxLedgers - used
	}
	// Decided per row. An owner-wide admin manages all of them, an admin bound
	// to one ledger manages that one, and everybody else manages none.
	type ledgerRow struct {
		app.LedgerInfo
		CanManage bool
	}
	rows := make([]ledgerRow, 0, len(ledgers))
	ownerWide := canManageLedger(sess, signedIn, owner, "")
	for _, l := range ledgers {
		rows = append(rows, ledgerRow{LedgerInfo: l, CanManage: canManageLedger(sess, signedIn, owner, l.Slug)})
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(c.Writer, "owner", s.page(c, gin.H{
		"Title":         owner,
		"Owner":         owner,
		"Ledgers":       rows,
		"MaxLedgers":    o.MaxLedgers,
		"Used":          used,
		"Unused":        unused,
		"CanCreate":     ownerWide,
		"CanManage":     ownerWide,
		"SignedIn":      signedIn,
		"SessionLedger": sess.LedgerSlug,
		"Message":       message,
		"FormSlug":      formSlug,
		"FormTitle":     formTitle,
	})); err != nil {
		log.Printf("template: %v", err)
	}
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
