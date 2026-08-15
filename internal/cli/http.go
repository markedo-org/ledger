package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/markedo-org/ledger/internal/cliconfig"
)

type apiClient struct {
	base   string
	token  string
	http   *http.Client
	owner  string
	ledger string
}

func newAPI(profile string) (*apiClient, error) {
	_, p, err := cliconfig.Resolve(profile)
	if err != nil {
		return nil, err
	}
	if p.URL == "" || p.Token == "" {
		return nil, fmt.Errorf("url and token missing; run ledger init or ledger config set")
	}
	return &apiClient{
		base:   strings.TrimRight(p.URL, "/"),
		token:  p.Token,
		owner:  p.Owner,
		ledger: p.Ledger,
		http:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *apiClient) do(method, path string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("%s %s: %s", method, path, msg)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

func printJSON(v any) int {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(string(raw))
	return 0
}
