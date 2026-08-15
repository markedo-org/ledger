package cliconfig

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	URL    string
	Token  string
	Owner  string
	Ledger string
}

type File struct {
	Profiles map[string]Profile
	Order    []string
}

func Path() (string, error) {
	if p := strings.TrimSpace(os.Getenv("LEDGER_CONFIG")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ledger", "config"), nil
}

func Load(path string) (File, error) {
	out := File{Profiles: map[string]Profile{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	var name string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name = strings.TrimSpace(line[1 : len(line)-1])
			if name == "" {
				return out, fmt.Errorf("empty profile name")
			}
			if _, ok := out.Profiles[name]; !ok {
				out.Order = append(out.Order, name)
				out.Profiles[name] = Profile{}
			}
			continue
		}
		if name == "" {
			return out, fmt.Errorf("key outside a profile: %s", line)
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return out, fmt.Errorf("expected key = value: %s", line)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		p := out.Profiles[name]
		switch k {
		case "url":
			p.URL = v
		case "token":
			p.Token = v
		case "owner":
			p.Owner = v
		case "ledger":
			p.Ledger = v
		default:
			return out, fmt.Errorf("unknown key %q in [%s]", k, name)
		}
		out.Profiles[name] = p
	}
	return out, sc.Err()
}

func Save(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# task-ledger CLI. Mode 0600. Do not commit.\n")
	seen := map[string]bool{}
	for _, name := range f.Order {
		p, ok := f.Profiles[name]
		if !ok {
			continue
		}
		writeProfile(&b, name, p)
		seen[name] = true
	}
	for name, p := range f.Profiles {
		if seen[name] {
			continue
		}
		writeProfile(&b, name, p)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func writeProfile(b *strings.Builder, name string, p Profile) {
	b.WriteString("\n[" + name + "]\n")
	if p.URL != "" {
		b.WriteString("url = " + p.URL + "\n")
	}
	if p.Token != "" {
		b.WriteString("token = " + p.Token + "\n")
	}
	if p.Owner != "" {
		b.WriteString("owner = " + p.Owner + "\n")
	}
	if p.Ledger != "" {
		b.WriteString("ledger = " + p.Ledger + "\n")
	}
}

func (f File) Get(name string) (Profile, bool) {
	if name == "" {
		name = "default"
	}
	p, ok := f.Profiles[name]
	return p, ok
}

func (f *File) Put(name string, p Profile) {
	if name == "" {
		name = "default"
	}
	if _, ok := f.Profiles[name]; !ok {
		f.Order = append(f.Order, name)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	f.Profiles[name] = p
}

func Resolve(name string) (string, Profile, error) {
	if name == "" {
		name = strings.TrimSpace(os.Getenv("LEDGER_PROFILE"))
	}
	if name == "" {
		name = "default"
	}
	path, err := Path()
	if err != nil {
		return name, Profile{}, err
	}
	f, err := Load(path)
	if err != nil {
		return name, Profile{}, err
	}
	p, _ := f.Get(name)
	if u := strings.TrimSpace(os.Getenv("LEDGER_URL")); u != "" {
		p.URL = u
	}
	if t := strings.TrimSpace(os.Getenv("LEDGER_TOKEN")); t != "" {
		p.Token = t
	}
	return name, p, nil
}
