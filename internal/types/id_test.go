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
