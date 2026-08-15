package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

func (s *Server) ownerJSON(o types.Owner, ledgers []app.LedgerInfo) gin.H {
	out := gin.H{
		"slug":        o.Slug,
		"max_ledgers": o.MaxLedgers,
		"created_at":  o.CreatedAt.UTC().Format(time.RFC3339),
	}
	if ledgers != nil {
		ls := make([]gin.H, 0, len(ledgers))
		for _, l := range ledgers {
			item := s.ledgerJSON(l.Ledger)
			item["frozen"] = l.Frozen
			ls = append(ls, item)
		}
		out["ledgers"] = ls
	}
	return out
}

func (s *Server) createOwner(c *gin.Context) {
	var in struct {
		Slug       string `json:"slug"`
		MaxLedgers int    `json:"max_ledgers"`
		Ledger     string `json:"ledger"`
		Title      string `json:"title"`
		Actor      string `json:"actor"`
		Email      string `json:"email"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	created, err := s.App.CreateOwner(c.Request.Context(), tokenFrom(c), app.CreateOwnerInput{
		Slug: in.Slug, MaxLedgers: in.MaxLedgers, Ledger: in.Ledger, Title: in.Title, Actor: in.Actor, Email: in.Email,
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	body := s.ownerJSON(created.Owner, nil)
	if created.Ledger != nil {
		body["ledger"] = gin.H{
			"owner": created.Ledger.OwnerSlug,
			"slug":  created.Ledger.Slug,
			"title": created.Ledger.Title,
		}
	}
	if created.Token != nil {
		body["token"] = created.Token.Plain
		body["actor"] = created.Token.Token.Actor
		body["role"] = created.Token.Token.Role
		body["note"] = "Owner admin, not bound to the first ledger. Name its MCP server task-ledger-admin."
		body["mcp"] = s.App.AgentMCPConfig("task-ledger-admin", created.Token.Plain)
	}
	if created.WriteToken != nil && created.Ledger != nil {
		body["write_token"] = created.WriteToken.Plain
		body["write_role"] = created.WriteToken.Token.Role
		body["write_ledger"] = created.Ledger.Slug
		body["write_mcp"] = s.App.AgentMCPConfig("task-ledger-"+created.Ledger.Slug, created.WriteToken.Plain)
	}
	c.JSON(http.StatusCreated, body)
}

func (s *Server) listOwners(c *gin.Context) {
	owners, err := s.App.ListOwners(c.Request.Context(), tokenFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]gin.H, 0, len(owners))
	for _, o := range owners {
		out = append(out, s.ownerJSON(o, nil))
	}
	c.JSON(http.StatusOK, gin.H{"owners": out})
}

func (s *Server) getOwner(c *gin.Context) {
	o, ledgers, err := s.App.GetOwner(c.Request.Context(), tokenFrom(c), c.Param("owner"))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, s.ownerJSON(o, ledgers))
}

func (s *Server) patchOwner(c *gin.Context) {
	var in struct {
		MaxLedgers *int `json:"max_ledgers"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	if in.MaxLedgers == nil {
		s.fail(c, fmt.Errorf("%w: max_ledgers required", app.ErrInvalid))
		return
	}
	o, err := s.App.SetMaxLedgers(c.Request.Context(), tokenFrom(c), c.Param("owner"), *in.MaxLedgers)
	if err != nil {
		s.fail(c, err)
		return
	}
	_, ledgers, err := s.App.GetOwner(c.Request.Context(), tokenFrom(c), o.Slug)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, s.ownerJSON(o, ledgers))
}

func (s *Server) operatorToken() types.Token {
	return types.Token{Actor: "operator", Role: types.RoleOperator}
}

func (s *Server) operatorGate(c *gin.Context) {
	if !s.App.OperatorConfigured() {
		c.Status(http.StatusNotFound)
		c.Abort()
		return
	}
	sess, ok := s.sessionFrom(c)
	if !ok {
		c.Redirect(http.StatusFound, "/login?next="+url.QueryEscape("/admin"))
		c.Abort()
		return
	}
	if !sess.IsOperator() {
		c.Redirect(http.StatusFound, homePath(sess, ""))
		c.Abort()
		return
	}
	c.Next()
}

func (s *Server) htmlAdmin(c *gin.Context) {
	s.renderAdmin(c, "", "")
}

func (s *Server) htmlAdminPost(c *gin.Context) {
	if !s.checkCSRF(c) {
		s.renderAdmin(c, "That form could not be completed. Reload and try again.", "")
		return
	}
	tok := s.operatorToken()
	ctx := c.Request.Context()
	action := c.PostForm("action")
	switch action {
	case "create_owner":
		max, _ := strconv.Atoi(strings.TrimSpace(c.PostForm("max_ledgers")))
		created, err := s.App.CreateOwner(ctx, tok, app.CreateOwnerInput{
			Slug:       c.PostForm("slug"),
			MaxLedgers: max,
			Ledger:     c.PostForm("ledger"),
			Title:      c.PostForm("title"),
			Actor:      c.PostForm("actor"),
			Email:      c.PostForm("email"),
		})
		if err != nil {
			s.renderAdmin(c, err.Error(), "")
			return
		}
		issued := ""
		if created.Token != nil {
			issued = created.Token.Plain
		}
		s.renderAdmin(c, "Created owner "+created.Owner.Slug+". Token is owner admin, not bound to the first ledger.", issued)
	case "set_max":
		n, err := strconv.Atoi(strings.TrimSpace(c.PostForm("max_ledgers")))
		if err != nil {
			s.renderAdmin(c, "max_ledgers must be an integer.", "")
			return
		}
		o, err := s.App.SetMaxLedgers(ctx, tok, c.PostForm("owner"), n)
		if err != nil {
			s.renderAdmin(c, err.Error(), "")
			return
		}
		s.renderAdmin(c, "Set "+o.Slug+" max_ledgers to "+strconv.Itoa(o.MaxLedgers)+".", "")
	case "create_ledger":
		owner := c.PostForm("owner")
		l, err := s.App.CreateLedger(ctx, tok, owner, app.CreateLedgerInput{
			Slug:  c.PostForm("slug"),
			Title: c.PostForm("title"),
		})
		if err != nil {
			s.renderAdmin(c, err.Error(), "")
			return
		}
		issued, err := s.App.MintProjectWrite(ctx, tok, owner, l.Slug, app.ProjectActor(tok, c.PostForm("actor")))
		if err != nil {
			s.renderAdmin(c, err.Error(), "")
			return
		}
		view := s.App.LedgerCreatedView(l, issued)
		mcp, _ := json.MarshalIndent(view["mcp"], "", "  ")
		plain := ""
		if issued != nil {
			plain = issued.Plain
		}
		note, _ := view["note"].(string)
		s.renderAdminMCP(c, note, plain, string(mcp))
	case "create_token":
		issued, err := s.App.CreateToken(ctx, tok, c.PostForm("owner"), app.CreateTokenInput{
			Actor:  c.PostForm("actor"),
			Ledger: c.PostForm("ledger"),
			Role:   c.PostForm("role"),
			Email:  c.PostForm("email"),
		})
		if err != nil {
			s.renderAdmin(c, err.Error(), "")
			return
		}
		s.renderAdmin(c, "Minted token for "+issued.Token.Actor+". Shown once.", issued.Plain)
	default:
		s.renderAdmin(c, "Unknown action.", "")
	}
}

func (s *Server) renderAdminMCP(c *gin.Context, message, issued, mcp string) {
	s.renderAdminPage(c, message, issued, mcp)
}

func (s *Server) renderAdmin(c *gin.Context, message, issued string) {
	s.renderAdminPage(c, message, issued, "")
}

func (s *Server) renderAdminPage(c *gin.Context, message, issued, mcp string) {
	owners, err := s.App.ListOwners(c.Request.Context(), s.operatorToken())
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	type ownerRow struct {
		Slug, Created string
		MaxLedgers    int
		Ledgers       []app.LedgerInfo
	}
	rows := make([]ownerRow, 0, len(owners))
	for _, o := range owners {
		_, ledgers, err := s.App.GetOwner(c.Request.Context(), s.operatorToken(), o.Slug)
		if err != nil {
			s.htmlErr(c, err)
			return
		}
		rows = append(rows, ownerRow{
			Slug:       o.Slug,
			MaxLedgers: o.MaxLedgers,
			Created:    o.CreatedAt.UTC().Format("2006-01-02"),
			Ledgers:    ledgers,
		})
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(c.Writer, "admin", s.page(c, gin.H{
		"Title":   "Admin · task-ledger",
		"Message": message,
		"Issued":  issued,
		"MCP":     mcp,
		"Owners":  rows,
	})); err != nil {
		log.Printf("template: %v", err)
	}
}
