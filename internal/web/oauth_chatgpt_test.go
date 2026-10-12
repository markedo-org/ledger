package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/markedo-org/ledger/internal/app"
	"github.com/markedo-org/ledger/internal/store"
)

func TestChatGPTOAuthStaysReadOnlyUntilRevoked(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	adminPlain, err := store.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Bootstrap(context.Background(), "markedo", "meta", "maria", adminPlain); err != nil {
		t.Fatal(err)
	}
	a := app.New(st)
	srv, err := New(a)
	if err != nil {
		t.Fatal(err)
	}
	const redirectURI = "https://chatgpt.com/connector_platform_oauth_redirect"
	const clientID = "https://chatgpt.com/oauth/client.json"
	srv.OAuthClients = func(context.Context, string) ([]string, error) {
		return []string{redirectURI}, nil
	}
	httpSrv := httptest.NewServer(srv.Handler())
	t.Cleanup(httpSrv.Close)
	a.PublicURL = httpSrv.URL

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	meta, err := client.Get(httpSrv.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	body := mustBody(t, meta)
	if meta.StatusCode != http.StatusOK || !strings.Contains(body, httpSrv.URL+"/oauth/token") {
		t.Fatalf("metadata %d %s", meta.StatusCode, body)
	}

	loginPage, err := client.Get(httpSrv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	loginHTML := mustBody(t, loginPage)
	csrf := attr(loginHTML, `name="csrf" value="`)
	form := url.Values{"csrf": {csrf}, "token": {adminPlain}}
	res, err := client.PostForm(httpSrv.URL+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	mustBody(t, res)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login %d", res.StatusCode)
	}

	verifier := strings.Repeat("a", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"resource":              {httpSrv.URL + "/mcp"},
		"state":                 {"xyz"},
		"scope":                 {"ledger:read offline_access"},
	}
	consent, err := client.Get(httpSrv.URL + "/oauth/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	consentHTML := mustBody(t, consent)
	if consent.StatusCode != http.StatusOK || !strings.Contains(consentHTML, "Allow read access") {
		t.Fatalf("consent %d %s", consent.StatusCode, consentHTML)
	}
	pending := attr(consentHTML, `name="pending" value="`)
	csrf = attr(consentHTML, `name="csrf" value="`)
	approve := url.Values{"csrf": {csrf}, "pending": {pending}, "ledger": {"meta"}}
	back, err := client.PostForm(httpSrv.URL+"/oauth/authorize", approve)
	if err != nil {
		t.Fatal(err)
	}
	mustBody(t, back)
	loc := back.Header.Get("Location")
	if back.StatusCode != http.StatusFound || !strings.HasPrefix(loc, redirectURI) {
		t.Fatalf("redirect %d %s", back.StatusCode, loc)
	}
	locURL, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	if locURL.Query().Get("state") != "xyz" || locURL.Query().Get("iss") != httpSrv.URL {
		t.Fatalf("callback query %s", locURL.RawQuery)
	}
	code := locURL.Query().Get("code")
	if code == "" {
		t.Fatal("missing code")
	}

	tokenBody := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"resource":      {httpSrv.URL + "/mcp"},
	}
	tokRes, err := http.PostForm(httpSrv.URL+"/oauth/token", tokenBody)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustBody(t, tokRes)
	if tokRes.StatusCode != http.StatusOK {
		t.Fatalf("token %d %s", tokRes.StatusCode, raw)
	}
	var issued struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Scope   string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(raw), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Access == "" || issued.Refresh == "" || !strings.Contains(issued.Scope, "offline_access") {
		t.Fatalf("issued %#v", issued)
	}
	readReq, err := http.NewRequest(http.MethodGet, httpSrv.URL+"/v1/markedo/meta/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	readReq.Header.Set("Authorization", "Bearer "+issued.Access)
	readRes, err := http.DefaultClient.Do(readReq)
	if err != nil {
		t.Fatal(err)
	}
	readBody := mustBody(t, readRes)
	if readRes.StatusCode != http.StatusOK {
		t.Fatalf("read %d %s", readRes.StatusCode, readBody)
	}

	list := mcpCall(t, httpSrv.URL, issued.Access, "tools/list", nil)
	if !strings.Contains(list, "list_tasks") || strings.Contains(list, "create_task") {
		t.Fatalf("tool list leaked a write tool: %s", list)
	}
	createReq, err := http.NewRequest(http.MethodPost, httpSrv.URL+"/v1/markedo/meta/tasks", strings.NewReader(`{"title":"nope","idempotency_key":"oauth-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Authorization", "Bearer "+issued.Access)
	createReq.Header.Set("Content-Type", "application/json")
	createRes, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	createBody := mustBody(t, createRes)
	if createRes.StatusCode != http.StatusForbidden {
		t.Fatalf("create with access token %d %s", createRes.StatusCode, createBody)
	}

	refreshBody := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {issued.Refresh},
		"client_id":     {clientID},
		"resource":      {httpSrv.URL + "/mcp"},
	}
	refreshed, err := http.PostForm(httpSrv.URL+"/oauth/token", refreshBody)
	if err != nil {
		t.Fatal(err)
	}
	refreshRaw := mustBody(t, refreshed)
	if refreshed.StatusCode != http.StatusOK {
		t.Fatalf("refresh %d %s", refreshed.StatusCode, refreshRaw)
	}
	again, err := http.PostForm(httpSrv.URL+"/oauth/token", refreshBody)
	if err != nil {
		t.Fatal(err)
	}
	againRaw := mustBody(t, again)
	if again.StatusCode == http.StatusOK {
		t.Fatalf("spent refresh token still worked: %s", againRaw)
	}

	listReq, err := http.NewRequest(http.MethodGet, httpSrv.URL+"/v1/markedo/tokens", nil)
	if err != nil {
		t.Fatal(err)
	}
	listReq.Header.Set("Authorization", "Bearer "+adminPlain)
	listRes, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	listed := mustBody(t, listRes)
	var tokens struct {
		Tokens []struct {
			ID    string `json:"id"`
			Actor string `json:"actor"`
			Role  string `json:"role"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(listed), &tokens); err != nil {
		t.Fatal(err)
	}
	var readID string
	for _, tok := range tokens.Tokens {
		if tok.Actor == "chatgpt" && tok.Role == "read" {
			readID = tok.ID
		}
	}
	if readID == "" {
		t.Fatalf("read token not listed: %s", listed)
	}
	revReq, err := http.NewRequest(http.MethodDelete, httpSrv.URL+"/v1/markedo/tokens/"+readID, nil)
	if err != nil {
		t.Fatal(err)
	}
	revReq.Header.Set("Authorization", "Bearer "+adminPlain)
	revRes, err := http.DefaultClient.Do(revReq)
	if err != nil {
		t.Fatal(err)
	}
	if revRes.StatusCode != http.StatusOK {
		t.Fatalf("revoke %d %s", revRes.StatusCode, mustBody(t, revRes))
	}
	_ = mustBody(t, revRes)

	var renewed struct {
		Access string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(refreshRaw), &renewed); err != nil {
		t.Fatal(err)
	}
	dead, err := http.NewRequest(http.MethodGet, httpSrv.URL+"/v1/markedo/meta/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	dead.Header.Set("Authorization", "Bearer "+renewed.Access)
	deadRes, err := http.DefaultClient.Do(dead)
	if err != nil {
		t.Fatal(err)
	}
	if deadRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked grant still read %d %s", deadRes.StatusCode, mustBody(t, deadRes))
	}
	_ = mustBody(t, deadRes)
}

func mcpCall(t *testing.T, origin, token, method string, params any) string {
	t.Helper()
	payload := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		payload["params"] = params
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, origin+"/mcp", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := mustBody(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("mcp %s %d %s", method, res.StatusCode, body)
	}
	return body
}

func mustBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func attr(html, key string) string {
	i := strings.Index(html, key)
	if i < 0 {
		return ""
	}
	rest := html[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
