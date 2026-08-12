package web

import (
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/types"
)

type Server struct {
	App    *app.App
	tmpl   *template.Template
	static fs.FS
}

func New(a *app.App) (*Server, error) {
	sub, err := fs.Sub(assets, "templates")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.ParseFS(sub, "*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	return &Server{App: a, tmpl: tmpl, static: static}, nil
}

func (s *Server) Engine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, "task-ledger\n\nLive view: /{owner}/{ledger}\nSnapshot:  /{owner}/{ledger}.md\nAPI:       /v1/{owner}/{ledger}/tasks\n")
	})
	r.GET("/static/*filepath", func(c *gin.Context) {
		p := strings.TrimPrefix(c.Param("filepath"), "/")
		http.ServeFileFS(c.Writer, c.Request, s.static, p)
	})

	v1 := r.Group("/v1")
	v1.Use(s.auth)
	v1.POST("/:owner/:ledger/tasks", s.create)
	v1.GET("/:owner/:ledger/tasks", s.list)
	v1.GET("/:owner/:ledger/tasks/:handle", s.get)
	v1.POST("/:owner/:ledger/tasks/:handle/claim", s.claim)
	v1.POST("/:owner/:ledger/tasks/:handle/heartbeat", s.heartbeat)
	v1.POST("/:owner/:ledger/tasks/:handle/release", s.release)
	v1.POST("/:owner/:ledger/tasks/:handle/phase", s.phase)
	v1.POST("/:owner/:ledger/tasks/:handle/notes", s.note)
	v1.POST("/:owner/:ledger/tasks/:handle/close", s.close)
	v1.POST("/:owner/:ledger/tasks/:handle/verify", s.verify)
	v1.POST("/:owner/:ledger/next", s.next)

	r.GET("/:owner/:ledger", s.htmlLedger)
	r.GET("/:owner/:ledger/:handle", s.htmlTask)
	return r
}

func (s *Server) auth(c *gin.Context) {
	h := c.GetHeader("Authorization")
	token := strings.TrimPrefix(h, "Bearer ")
	if token == h {
		token = ""
	}
	tok, err := s.App.Auth(c.Request.Context(), strings.TrimSpace(token))
	if err != nil {
		s.fail(c, err)
		c.Abort()
		return
	}
	c.Set("token", tok)
	c.Next()
}

func tokenFrom(c *gin.Context) types.Token {
	v, _ := c.Get("token")
	t, _ := v.(types.Token)
	return t
}

func (s *Server) fail(c *gin.Context, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		code = http.StatusUnauthorized
	case errors.Is(err, app.ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, app.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, app.ErrConflict):
		code = http.StatusConflict
	case errors.Is(err, app.ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, app.ErrPolicy):
		code = http.StatusUnprocessableEntity
	}
	c.JSON(code, gin.H{"error": err.Error()})
}

func taskJSON(t types.Task) gin.H {
	var until any
	if t.ClaimedUntil != nil {
		until = t.ClaimedUntil.UTC().Format(time.RFC3339)
	}
	notes := make([]gin.H, 0, len(t.Notes))
	for _, n := range t.Notes {
		notes = append(notes, gin.H{
			"actor": n.Actor,
			"body":  n.Body,
			"at":    n.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	checks := make([]gin.H, 0, len(t.Checks))
	for _, ch := range t.Checks {
		checks = append(checks, gin.H{"body": ch.Body, "done": ch.Done})
	}
	return gin.H{
		"id":            t.ID,
		"handle":        t.Handle,
		"title":         t.Title,
		"body":          t.Body,
		"phase":         t.Phase,
		"size":          t.Size,
		"rank":          t.Rank,
		"version":       t.Version,
		"pushed":        t.Pushed,
		"ref":           t.Ref,
		"claimed_by":    t.ClaimedBy,
		"claimed_until": until,
		"evidence":      t.Evidence,
		"notes":         notes,
		"checks":        checks,
		"depends_on":    t.DependsOn,
		"since":         t.Since.UTC().Format("2006-01-02"),
	}
}

func (s *Server) create(c *gin.Context) {
	var in struct {
		Title          string   `json:"title"`
		Body           string   `json:"body"`
		Phase          string   `json:"phase"`
		Size           string   `json:"size"`
		Prefix         string   `json:"prefix"`
		Ref            string   `json:"ref"`
		IdempotencyKey string   `json:"idempotency_key"`
		Checks         []string `json:"checks"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	if k := c.GetHeader("Idempotency-Key"); k != "" && in.IdempotencyKey == "" {
		in.IdempotencyKey = k
	}
	t, replay, err := s.App.Create(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), app.CreateInput{
		Prefix: in.Prefix, Title: in.Title, Body: in.Body, Phase: in.Phase, Size: in.Size,
		Ref: in.Ref, IdempotencyKey: in.IdempotencyKey, Checks: in.Checks,
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	code := http.StatusCreated
	if replay {
		code = http.StatusOK
	}
	c.JSON(code, taskJSON(t))
}

func (s *Server) list(c *gin.Context) {
	_, tasks, err := s.App.List(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"))
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskJSON(t))
	}
	c.JSON(http.StatusOK, gin.H{"tasks": out})
}

func (s *Server) get(c *gin.Context) {
	t, err := s.App.Get(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) claim(c *gin.Context) {
	var in struct {
		TTLSeconds int    `json:"ttl_seconds"`
		Steal      bool   `json:"steal"`
		Reason     string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&in)
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := s.App.Claim(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"), app.ClaimInput{
		TTL: ttl, Steal: in.Steal, Reason: in.Reason,
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) heartbeat(c *gin.Context) {
	var in struct {
		TTLSeconds int `json:"ttl_seconds"`
	}
	_ = c.ShouldBindJSON(&in)
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := s.App.Heartbeat(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"), ttl)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) release(c *gin.Context) {
	t, err := s.App.Release(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) phase(c *gin.Context) {
	var in struct {
		Phase  string `json:"phase"`
		Reason string `json:"reason"`
		Force  bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	t, err := s.App.SetPhase(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"), app.PhaseInput(in))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) note(c *gin.Context) {
	var in struct {
		Body string `json:"body"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	n, err := s.App.AddNote(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"), in.Body)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"actor": n.Actor, "body": n.Body, "at": n.CreatedAt.UTC().Format(time.RFC3339)})
}

func (s *Server) close(c *gin.Context) {
	var in struct {
		Evidence string `json:"evidence"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		s.fail(c, err)
		return
	}
	t, err := s.App.Close(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"), in.Evidence)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) verify(c *gin.Context) {
	t, err := s.App.Verify(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), c.Param("handle"))
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) next(c *gin.Context) {
	var in struct {
		Prefix     string `json:"prefix"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	_ = c.ShouldBindJSON(&in)
	var ttl time.Duration
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	t, err := s.App.Next(c.Request.Context(), tokenFrom(c), c.Param("owner"), c.Param("ledger"), in.Prefix, ttl)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

func (s *Server) htmlLedger(c *gin.Context) {
	owner, ledger := c.Param("owner"), c.Param("ledger")
	if strings.HasSuffix(ledger, ".md") {
		s.markdown(c, owner, strings.TrimSuffix(ledger, ".md"))
		return
	}
	l, tasks, err := s.App.ListPublic(c.Request.Context(), owner, ledger)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	type row struct {
		Handle, Title, Size, ClaimedBy, ClaimUntil string
		Claimed                                    bool
		Pushed                                     int
	}
	type phase struct {
		Name  string
		Tasks []row
	}
	order := []types.Phase{types.PhaseNOW, types.PhaseNEXT, types.PhaseLATER, types.PhaseGATED, types.PhasePARKED, types.PhaseDONE}
	by := map[types.Phase][]row{}
	now := time.Now().UTC()
	for _, t := range tasks {
		r := row{Handle: t.Handle, Title: t.Title, Size: string(t.Size), Pushed: t.Pushed}
		if t.ClaimedBy != "" && t.ClaimedUntil != nil && t.ClaimedUntil.After(now) {
			r.Claimed = true
			r.ClaimedBy = t.ClaimedBy
			r.ClaimUntil = t.ClaimedUntil.UTC().Format("15:04")
		}
		by[t.Phase] = append(by[t.Phase], r)
	}
	var phases []phase
	for _, p := range order {
		phases = append(phases, phase{Name: string(p), Tasks: by[p]})
	}
	data := gin.H{
		"Title":  l.OwnerSlug + "/" + l.Slug,
		"Owner":  l.OwnerSlug,
		"Ledger": l.Slug,
		"Phases": phases,
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(c.Writer, "ledger", data); err != nil {
		log.Printf("template: %v", err)
	}
}

func (s *Server) htmlTask(c *gin.Context) {
	l, t, err := s.App.GetPublic(c.Request.Context(), c.Param("owner"), c.Param("ledger"), c.Param("handle"))
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	until := ""
	if t.ClaimedUntil != nil {
		until = t.ClaimedUntil.UTC().Format(time.RFC3339)
	}
	data := gin.H{
		"Title":      t.Handle + " · " + l.OwnerSlug + "/" + l.Slug,
		"Owner":      l.OwnerSlug,
		"Ledger":     l.Slug,
		"Task":       t,
		"ClaimUntil": until,
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(c.Writer, "task", data); err != nil {
		log.Printf("template: %v", err)
	}
}

func (s *Server) htmlErr(c *gin.Context, err error) {
	code := http.StatusInternalServerError
	if errors.Is(err, app.ErrNotFound) {
		code = http.StatusNotFound
	}
	c.String(code, err.Error())
}

func (s *Server) markdown(c *gin.Context, owner, ledger string) {
	l, tasks, err := s.App.ListPublic(c.Request.Context(), owner, ledger)
	if err != nil {
		s.htmlErr(c, err)
		return
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, RenderMarkdown(l, tasks, c.Request.Host))
}
