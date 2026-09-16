package merkle

import (
	"strings"
	"testing"
)

func TestTreeRevisionCanonicalAndLocalUpdate(t *testing.T) {
	recs := []Record{
		{Path: "b.txt", Hash: "hb", Size: 2},
		{Path: "a.txt", Hash: "ha", Size: 1},
		{Path: "c.txt", Hash: "hc", Size: 3},
	}
	built := Build(recs)
	inc := New()
	for _, rec := range recs {
		inc = inc.Upsert(rec)
	}
	if built.Revision() != inc.Revision() || !strings.HasPrefix(built.Revision(), prefix) {
		t.Fatalf("order %s vs %s", built.Revision(), inc.Revision())
	}
	if built.Len() != 3 {
		t.Fatal(built.Len())
	}
	changed := Record{Path: "b.txt", Hash: "hb2", Size: 9}
	updated := built.Apply([]Record{changed}, nil)
	rebuilt := Build([]Record{recs[1], changed, recs[2]})
	if updated.Revision() != rebuilt.Revision() {
		t.Fatalf("apply %s vs %s", updated.Revision(), rebuilt.Revision())
	}
	if updated.Revision() == built.Revision() {
		t.Fatal("unchanged")
	}
	got, ok := updated.Get("b.txt")
	if !ok || got.Hash != "hb2" {
		t.Fatalf("%v %v", got, ok)
	}
	removed := updated.Delete("a.txt")
	if _, ok := removed.Get("a.txt"); ok || removed.Len() != 2 {
		t.Fatal(removed.Len())
	}
	empty := removed.Delete("b.txt").Delete("c.txt")
	if empty.Revision() != New().Revision() {
		t.Fatalf("empty %s", empty.Revision())
	}
}

func TestCoverageRecheck(t *testing.T) {
	if !(Coverage{Complete: false}).RequiresRecheck() {
		t.Fatal("incomplete")
	}
	if !(Coverage{Complete: true, Overflow: true}).RequiresRecheck() {
		t.Fatal("overflow")
	}
	if (Coverage{Complete: true}).RequiresRecheck() {
		t.Fatal("ok")
	}
}
