package types

import "testing"

func TestNormalizeTags(t *testing.T) {
	got, err := NormalizeTags([]string{" Ledger ", "SITE", "ledger", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "ledger" || got[1] != "site" {
		t.Fatalf("%v", got)
	}
	if _, err := NormalizeTags([]string{"Bad Tag"}); err == nil {
		t.Fatal("want invalid")
	}
	if _, err := NormalizeTags([]string{"a", "b", "c", "d"}); err == nil {
		t.Fatal("want max 3")
	}
	empty, err := NormalizeTags(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty %v %v", empty, err)
	}
}
