package dirs

import (
	"reflect"
	"testing"
)

func TestFuzzyPrefersBasename(t *testing.T) {
	paths := []string{"/core/legacy/a", "/dev/core-tools/x", "/dev/core"}
	m := Fuzzy("core", paths)
	if len(m) != 3 || paths[m[0].Index] != "/dev/core" {
		t.Fatalf("got %+v", m)
	}
	if !reflect.DeepEqual(m[0].Positions, []int{5, 6, 7, 8}) {
		t.Fatalf("positions %v", m[0].Positions)
	}
	// tie between the other two keeps input order
	if m[1].Index != 0 || m[2].Index != 1 {
		t.Fatalf("tie order %+v", m)
	}
}

func TestFuzzyNoMatch(t *testing.T) {
	if m := Fuzzy("zzz", []string{"/a/b", "/c"}); len(m) != 0 {
		t.Fatalf("got %+v", m)
	}
}

func TestFuzzyEmptyQueryKeepsOrder(t *testing.T) {
	m := Fuzzy("", []string{"/b", "/a", "/c"})
	if len(m) != 3 || m[0].Index != 0 || m[1].Index != 1 || m[2].Index != 2 {
		t.Fatalf("got %+v", m)
	}
}

func TestFuzzyCaseInsensitiveAndAnchored(t *testing.T) {
	// greedy would grab the early "a" in "/a/x/app"; anchoring finds the basename.
	m := Fuzzy("APP", []string{"/a/p/p/app"})
	if len(m) != 1 || !reflect.DeepEqual(m[0].Positions, []int{7, 8, 9}) {
		t.Fatalf("got %+v", m)
	}
}
