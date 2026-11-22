package dirs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stardust1420/starfield/internal/claude"
)

func mk(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScanFindsReposAndChildren(t *testing.T) {
	root := t.TempDir()
	mk(t,
		filepath.Join(root, "a", ".git"),
		filepath.Join(root, "b", "c", ".git"),
		filepath.Join(root, "plain", "deep"),
		filepath.Join(root, ".hidden", ".git"),
		filepath.Join(root, "node_modules", "x", ".git"),
		filepath.Join(root, "a", "sub", ".git"),
	)
	// worktree-style .git file
	mk(t, filepath.Join(root, "w", "wt"))
	if err := os.WriteFile(filepath.Join(root, "w", "wt", ".git"), []byte("gitdir: x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Scan(root, 2)
	want := []string{
		filepath.Join(root, "a"),
		filepath.Join(root, "b"),
		filepath.Join(root, "b", "c"),
		filepath.Join(root, "plain"),
		filepath.Join(root, "w"),
		filepath.Join(root, "w", "wt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if Scan(filepath.Join(root, "missing"), 2) != nil {
		t.Fatal("missing root should return nil")
	}
}

func TestCandidatesRecentFirstDeduped(t *testing.T) {
	root := t.TempDir()
	mk(t, filepath.Join(root, "a", ".git"), filepath.Join(root, "b", ".git"))
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	now := time.Now()
	hist := []claude.HistorySession{
		{Dir: b, LastActive: now.Add(-2 * time.Hour)},
		{Dir: a + "/", LastActive: now.Add(-time.Hour)},
		{Dir: b, LastActive: now.Add(-3 * time.Hour)},
	}
	got := Candidates(hist, []string{root}, 2)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Path != a || got[0].Source != Recent || !got[0].LastUsed.Equal(now.Add(-time.Hour)) {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].Path != b || got[1].Source != Recent {
		t.Fatalf("second %+v", got[1])
	}
}

func TestCandidatesScannedAfterRecent(t *testing.T) {
	root := t.TempDir()
	mk(t, filepath.Join(root, "a", ".git"), filepath.Join(root, "b", ".git"))
	hist := []claude.HistorySession{{Dir: filepath.Join(root, "b"), LastActive: time.Now()}}
	got := Candidates(hist, []string{root}, 2)
	if len(got) != 2 || got[0].Source != Recent || got[1].Source != Scanned || got[1].Path != filepath.Join(root, "a") {
		t.Fatalf("got %+v", got)
	}
}

func TestCandidatesSkipsMissingDirs(t *testing.T) {
	root := t.TempDir()
	hist := []claude.HistorySession{{Dir: filepath.Join(root, "gone"), LastActive: time.Now()}}
	if got := Candidates(hist, nil, 2); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestExpandTilde(t *testing.T) {
	cases := map[string]string{
		"~":       "/h",
		"~/x/y":   "/h/x/y",
		"/abs":    "/abs",
		"~user/x": "~user/x",
		"rel/~/x": "rel/~/x",
	}
	for in, want := range cases {
		if got := Expand(in, "/h"); got != want {
			t.Errorf("Expand(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRecentsDoesNotCheckTheDisk(t *testing.T) {
	now := time.Now()
	got := Recents([]claude.HistorySession{
		{Dir: "/gone/old", LastActive: now.Add(-time.Hour)},
		{Dir: "/gone/new", LastActive: now},
		{Dir: ""},
	})
	var paths []string
	for _, c := range got {
		paths = append(paths, c.Path)
		if c.Source != Recent {
			t.Errorf("%s source = %v; want Recent", c.Path, c.Source)
		}
	}
	if !reflect.DeepEqual(paths, []string{"/gone/new", "/gone/old"}) {
		t.Errorf("Recents = %v; want both, newest first, unchecked", paths)
	}
}
