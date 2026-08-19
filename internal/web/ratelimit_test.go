package web

import (
	"testing"
	"time"
)

func TestSignInGateWindow(t *testing.T) {
	g := newSignInGate(3, time.Minute)
	base := time.Date(2026, 8, 19, 22, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if !g.allow("10.0.0.1", base) {
			t.Fatalf("attempt %d refused inside the limit", i+1)
		}
	}
	if g.allow("10.0.0.1", base) {
		t.Fatal("fourth attempt allowed")
	}
	if !g.allow("10.0.0.2", base) {
		t.Fatal("a different address must have its own budget")
	}
	if !g.allow("10.0.0.1", base.Add(61*time.Second)) {
		t.Fatal("window did not roll over")
	}
}

func TestSignInGateSweepsIdleAddresses(t *testing.T) {
	g := newSignInGate(1, time.Minute)
	base := time.Date(2026, 8, 19, 22, 0, 0, 0, time.UTC)
	for i := 0; i < 10001; i++ {
		g.allow(string(rune(i))+"x", base)
	}
	g.allow("last", base.Add(2*time.Minute))
	if len(g.hits) != 1 {
		t.Fatalf("idle addresses were not swept: %d left", len(g.hits))
	}
}
