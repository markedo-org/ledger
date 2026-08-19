package web

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
)

func (s *Server) loginEmailPost(c *gin.Context) {
	if !s.checkCSRF(c) {
		s.renderLogin(c, "That sign-in could not be completed. Reload the page and try again.")
		return
	}
	err := s.App.RequestMagicLink(c.Request.Context(), c.PostForm("email"))
	if errors.Is(err, app.ErrNotFound) {
		s.renderLogin(c, "Magic-link email is off on this process. Paste a bearer token.")
		return
	}
	if errors.Is(err, app.ErrInvalid) {
		s.renderLogin(c, "That does not look like an email address.")
		return
	}
	if err != nil {
		log.Printf("magic-link: %v", err)
	}
	s.renderLogin(c, "If that address has a token on this ledger, we sent a sign-in link.")
}

func (s *Server) loginEmailGet(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		s.renderLogin(c, "That sign-in link is missing its code.")
		return
	}
	_, plain, err := s.App.ConsumeMagicLink(c.Request.Context(), code)
	if err != nil {
		s.renderLogin(c, "That sign-in link is invalid or has expired.")
		return
	}
	s.setCookie(c, sessionCookie, plain, int(app.SessionTTL.Seconds()))
	sess, err := s.App.Session(c.Request.Context(), plain)
	if err != nil {
		c.Redirect(http.StatusFound, "/owners")
		return
	}
	c.Redirect(http.StatusFound, homePath(sess, ""))
}

func (s *Server) loginReviewGet(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		s.renderLogin(c, "That review link is missing its code.")
		return
	}
	_, plain, err := s.App.ConsumeReviewLink(c.Request.Context(), code)
	if err != nil {
		s.renderLogin(c, "That review link is invalid or has expired.")
		return
	}
	s.setCookie(c, sessionCookie, plain, int(app.SessionTTL.Seconds()))
	sess, err := s.App.Session(c.Request.Context(), plain)
	if err != nil {
		c.Redirect(http.StatusFound, "/owners")
		return
	}
	c.Redirect(http.StatusFound, homePath(sess, ""))
}

func (s *Server) reviewPost(c *gin.Context) {
	url, exp, err := s.App.MintReviewURL(c.Request.Context(), tokenFrom(c))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"url":                url,
		"expires_in_seconds": exp,
	})
}
