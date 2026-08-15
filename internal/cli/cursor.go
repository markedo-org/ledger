package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func resolveCursorMCP(projectDir string, force, noWrite bool) (string, bool, error) {
	if noWrite {
		return "", false, nil
	}
	root := projectDir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", false, err
		}
		root = wd
	}
	dir := root
	if filepath.Base(filepath.Clean(root)) != ".cursor" {
		dir = filepath.Join(root, ".cursor")
	}
	path := filepath.Join(dir, "mcp.json")
	if force || projectDir != "" {
		return path, true, nil
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", false, nil
	}
	return path, true, nil
}

func maybeWriteCursor(projectDir string, force, noWrite bool, snippet []byte) (string, error) {
	path, ok, err := resolveCursorMCP(projectDir, force, noWrite)
	if err != nil || !ok {
		return "", err
	}
	if err := writeCursorMCP(path, snippet); err != nil {
		return "", err
	}
	return path, nil
}

func writeCursorMCP(path string, snippet []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var incoming struct {
		Servers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(snippet, &incoming); err != nil {
		return err
	}
	existing := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		var cur struct {
			Servers map[string]any `json:"mcpServers"`
		}
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if cur.Servers != nil {
			existing = cur.Servers
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for k, v := range incoming.Servers {
		existing[k] = v
	}
	out, err := json.MarshalIndent(map[string]any{"mcpServers": existing}, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o600)
}
