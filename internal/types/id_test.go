package types

import "testing"

func TestHandleRoundTrip(t *testing.T) {
	h := FormatHandle("T", 1)
	if h != "T-001" {
		t.Fatalf("%s", h)
	}
	p, n, err := ParseHandle("t-1")
	if err != nil || p != "T" || n != 1 {
		t.Fatalf("%s %d %v", p, n, err)
	}
	if FormatHandle(p, n) != "T-001" {
		t.Fatal("canonical")
	}
}

func TestValidSlug(t *testing.T) {
	ok := []string{"acme", "a", "repo-name", "x12", "2027", "1acme"}
	bad := []string{"", "Acme", "-no", "has_underscore", "Has-Dash"}
	for _, s := range ok {
		if !ValidSlug(s) {
			t.Fatalf("expected valid %q", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Fatalf("expected invalid %q", s)
		}
	}
}
