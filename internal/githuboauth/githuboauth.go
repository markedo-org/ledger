package githuboauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthorizeURL string
	TokenURL     string
	UserURL      string
	HTTP         *http.Client
}

type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type Client struct {
	cfg Config
}

func New(cfg Config) *Client {
	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = "https://github.com/login/oauth/authorize"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if cfg.UserURL == "" {
		cfg.UserURL = "https://api.github.com/user"
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{cfg: cfg}
}

func (c *Client) AuthURL(state string) string {
	q := url.Values{
		"client_id":    {c.cfg.ClientID},
		"redirect_uri": {c.cfg.RedirectURI},
		"scope":        {"read:user"},
		"state":        {state},
	}
	return c.cfg.AuthorizeURL + "?" + q.Encode()
}

func (c *Client) Exchange(ctx context.Context, code string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     c.cfg.ClientID,
		"client_secret": c.cfg.ClientSecret,
		"code":          code,
		"redirect_uri":  c.cfg.RedirectURI,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github token: status %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Error != "" || out.AccessToken == "" {
		return "", fmt.Errorf("github token: %s", out.Error)
	}
	return out.AccessToken, nil
}

func (c *Client) User(ctx context.Context, accessToken string) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.UserURL, nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "task-ledger")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("github user: status %d", resp.StatusCode)
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return User{}, err
	}
	if u.Login == "" {
		return User{}, fmt.Errorf("github user: empty login")
	}
	return u, nil
}
