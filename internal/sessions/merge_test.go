package sessions

import (
	"errors"
	"testing"
	"time"

	"github.com/stardust1420/starfield/internal/claude"
	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/procs"
	"github.com/stardust1420/starfield/internal/tmux"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// table: tmux pane pid 10 -> caffeinate 11 -> claude 12; claude 20 unrelated.
func testTable() procs.Table {
	return procs.Table{
		10: {PID: 10, PPID: 1, Comm: "zsh"},
		11: {PID: 11, PPID: 10, Comm: "caffeinate"},
		12: {PID: 12, PPID: 11, Comm: "claude"},
		20: {PID: 20, PPID: 1, Comm: "claude"},
	}
}

func byID(ss []Session, id string) (Session, bool) {
	for _, s := range ss {
		if s.ID == id {
			return s, true
		}
	}
	return Session{}, false
}

func TestMergeManagedRunning(t *testing.T) {
	hist := []claude.HistorySession{{ID: "a", Dir: "/h", Title: "T", LastPrompt: "p", Branch: "main", Model: "opus", CostUSD: 1.5, UserMessages: 3, Truncated: true, Started: t0, LastActive: t0.Add(time.Hour)}}
	live := []claude.LiveSession{{PID: 12, SessionID: "a", Dir: "/l", Status: "busy", StartedAt: t0.Add(time.Minute)}}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 10, ClaudeID: "a", Dir: "/p"}}
	got := Merge(hist, live, panes, testTable())
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	s := got[0]
	if s.State != Running || s.TmuxName != "sf-a" || s.PID != 12 || s.Status != "busy" {
		t.Fatalf("bad session %+v", s)
	}
	if s.Dir != "/h" || s.Title != "T" || s.Branch != "main" || s.Model != "opus" || s.CostUSD != 1.5 || s.UserMessages != 3 || !s.Truncated || s.LastPrompt != "p" {
		t.Fatalf("history not merged %+v", s)
	}
	if !s.Started.Equal(t0.Add(time.Minute)) || !s.LastActive.Equal(t0.Add(time.Hour)) {
		t.Fatalf("times %v %v", s.Started, s.LastActive)
	}
}

func TestMergeManagedWhenPanePIDIsClaude(t *testing.T) {
	live := []claude.LiveSession{{PID: 12, SessionID: "a", StartedAt: t0}}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 12, ClaudeID: "a"}}
	got := Merge(nil, live, panes, testTable())
	if len(got) != 1 || got[0].State != Running || got[0].TmuxName != "sf-a" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeExternal(t *testing.T) {
	live := []claude.LiveSession{{PID: 20, SessionID: "x", Dir: "/x", StartedAt: t0, Status: "idle"}}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 10, ClaudeID: "a"}}
	got := Merge(nil, live, panes, testTable())
	x, ok := byID(got, "x")
	if !ok || x.State != External || x.TmuxName != "" || x.PID != 20 || x.Dir != "/x" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeDedupesResumedHistory(t *testing.T) {
	hist := []claude.HistorySession{{ID: "a", LastActive: t0}}
	live := []claude.LiveSession{{PID: 20, SessionID: "a", StartedAt: t0}}
	got := Merge(hist, live, nil, testTable())
	if len(got) != 1 || got[0].State != External {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeStartingPane(t *testing.T) {
	hist := []claude.HistorySession{{ID: "a", Title: "T", Dir: "/h", LastActive: t0}}
	panes := []tmux.Pane{
		{Session: "sf-a", PanePID: 10, ClaudeID: "a", Dir: "/p"},
		{Session: "sf-b", PanePID: 30, ClaudeID: "b", Dir: "/pb", Created: t0},
	}
	got := Merge(hist, nil, panes, testTable())
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	a, _ := byID(got, "a")
	if a.State != Starting || a.TmuxName != "sf-a" || a.Title != "T" || a.Dir != "/h" || a.PID != 0 {
		t.Fatalf("a %+v", a)
	}
	b, _ := byID(got, "b")
	if b.State != Starting || b.Dir != "/pb" {
		t.Fatalf("b %+v", b)
	}
}

func TestMergeLiveWithoutHistoryUsesRegistryName(t *testing.T) {
	live := []claude.LiveSession{{PID: 20, SessionID: "x", Name: "my name", Dir: "/x", StartedAt: t0}}
	got := Merge(nil, live, nil, testTable())
	if len(got) != 1 || got[0].Title != "my name" || !got[0].LastActive.Equal(t0) {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeTitleFallbackOrder(t *testing.T) {
	hist := []claude.HistorySession{{ID: "a", LastPrompt: "prompt", LastActive: t0}}
	live := []claude.LiveSession{{PID: 20, SessionID: "a", Name: "reg", StartedAt: t0}}
	if got := Merge(hist, live, nil, testTable()); got[0].Title != "reg" {
		t.Fatalf("registry name should beat prompt: %q", got[0].Title)
	}
	if got := Merge(hist, nil, nil, testTable()); got[0].Title != "prompt" {
		t.Fatalf("prompt fallback: %q", got[0].Title)
	}
}

func TestMergeSortOrder(t *testing.T) {
	hist := []claude.HistorySession{
		{ID: "s-old", LastActive: t0},
		{ID: "s-new", LastActive: t0.Add(time.Hour)},
		{ID: "r1", LastActive: t0.Add(2 * time.Hour)},
		{ID: "e1", LastActive: t0.Add(3 * time.Hour)},
	}
	live := []claude.LiveSession{
		{PID: 12, SessionID: "r1", StartedAt: t0},
		{PID: 20, SessionID: "e1", StartedAt: t0},
	}
	panes := []tmux.Pane{
		{Session: "sf-r1", PanePID: 10, ClaudeID: "r1"},
		{Session: "sf-st", PanePID: 30, ClaudeID: "st"},
	}
	got := Merge(hist, live, panes, testTable())
	want := []string{"r1", "st", "e1", "s-new", "s-old"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("pos %d: got %s want %s (%+v)", i, got[i].ID, id, got)
		}
	}
}

func TestDiscoverReturnsPartialResultsOnSourceError(t *testing.T) {
	boom := errors.New("tmux down")
	d := NewDiscoverer(config.Config{ClaudeDir: "/nonexistent", HistoryDays: 7}, nil)
	d.scanHistory = func(string, time.Time, *claude.HistoryCache) ([]claude.HistorySession, error) {
		return []claude.HistorySession{{ID: "h", LastActive: t0}}, nil
	}
	d.snapshot = func() (procs.Table, error) { return testTable(), nil }
	d.readRegistry = func(string, procs.Table) ([]claude.LiveSession, error) {
		return []claude.LiveSession{{PID: 20, SessionID: "x", StartedAt: t0}}, nil
	}
	d.listPanes = func() ([]tmux.Pane, error) { return nil, boom }
	got, err := d.Discover()
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if x, _ := byID(got, "x"); x.State != External {
		t.Fatalf("x %+v", x)
	}
}

func TestMergeLiveDirFallsBackToPaneDir(t *testing.T) {
	live := []claude.LiveSession{{PID: 12, SessionID: "a", StartedAt: t0}}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 10, ClaudeID: "a", Dir: "/p"}}
	got := Merge(nil, live, panes, testTable())
	if len(got) != 1 || got[0].Dir != "/p" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeDedupesLiveSessionsWithSameID(t *testing.T) {
	// Managed entry wins even when older.
	live := []claude.LiveSession{
		{PID: 20, SessionID: "a", StartedAt: t0.Add(time.Hour)},
		{PID: 12, SessionID: "a", StartedAt: t0},
	}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 10, ClaudeID: "a"}}
	got := Merge(nil, live, panes, testTable())
	if len(got) != 1 || got[0].State != Running || got[0].PID != 12 {
		t.Fatalf("managed preference: %+v", got)
	}
	// Neither managed: newest StartedAt wins.
	live = []claude.LiveSession{
		{PID: 20, SessionID: "a", StartedAt: t0},
		{PID: 21, SessionID: "a", StartedAt: t0.Add(time.Hour)},
	}
	got = Merge(nil, live, nil, testTable())
	if len(got) != 1 || got[0].PID != 21 || got[0].State != External {
		t.Fatalf("newest preference: %+v", got)
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[2Jhi\x07\u009b31m":       "hi31m",
		"a\tb\nc\r":                    "a b c ",
		"plain ✦ text":                 "plain ✦ text",
		"del\x7fete\u0085x":            "deletex",
		"\x1b]0;title\x07after":        "after",
		"\x1b[31mred\x1b[0m and \x00z": "red and z",
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestMergeSanitizesText(t *testing.T) {
	evil := "\x1b[2Jhi\x07\u009b"
	hist := []claude.HistorySession{{ID: "a", Dir: "/d\x1b", Title: evil, LastPrompt: evil, Branch: "m\x07ain", Model: "op\x07us", LastActive: t0}}
	live := []claude.LiveSession{{PID: 20, SessionID: "x", Name: evil, Status: "id\x1ble", StartedAt: t0}}
	got := Merge(hist, live, nil, testTable())
	a, _ := byID(got, "a")
	if a.Title != "hi" || a.LastPrompt != "hi" || a.Branch != "main" || a.Model != "opus" {
		t.Errorf("history fields not sanitized: %+v", a)
	}
	if a.Dir != "/d\x1b" {
		t.Errorf("Dir must stay as is (used for tmux): %q", a.Dir)
	}
	x, _ := byID(got, "x")
	if x.Title != "hi" {
		t.Errorf("registry name not sanitized: %q", x.Title)
	}
}

// Two live sessions under one pane: the shallow one is the pane's own
// session (ClaudeID matches), the nested one is a child claude with another
// id. The matching one must get the pane whatever the registry order.
func TestMergePaneGoesToBestMatch(t *testing.T) {
	table := procs.Table{
		10: {PID: 10, PPID: 1, Comm: "zsh"},
		11: {PID: 11, PPID: 10, Comm: "caffeinate"},
		12: {PID: 12, PPID: 11, Comm: "claude"},
		13: {PID: 13, PPID: 12, Comm: "claude"},
	}
	panes := []tmux.Pane{{Session: "sf-a", PanePID: 10, ClaudeID: "a"}}
	shallow := claude.LiveSession{PID: 12, SessionID: "a", StartedAt: t0}
	nested := claude.LiveSession{PID: 13, SessionID: "b", StartedAt: t0}
	for _, live := range [][]claude.LiveSession{{nested, shallow}, {shallow, nested}} {
		got := Merge(nil, live, panes, table)
		a, _ := byID(got, "a")
		b, _ := byID(got, "b")
		if a.State != Running || a.TmuxName != "sf-a" {
			t.Errorf("order %v: a = %+v; want Running in sf-a", live, a)
		}
		if b.State != External || b.TmuxName != "" {
			t.Errorf("order %v: b = %+v; want External", live, b)
		}
		if len(got) != 2 {
			t.Errorf("got %+v", got)
		}
	}
}

// Without an id match, the pane's own pid wins, then the shallowest process.
func TestMergePaneFallsBackToPIDThenDepth(t *testing.T) {
	table := procs.Table{
		10: {PID: 10, PPID: 1, Comm: "claude"},
		11: {PID: 11, PPID: 10, Comm: "claude"},
		12: {PID: 12, PPID: 11, Comm: "claude"},
	}
	panes := []tmux.Pane{{Session: "sf-z", PanePID: 10, ClaudeID: "z"}}
	l10 := claude.LiveSession{PID: 10, SessionID: "p", StartedAt: t0}
	l11 := claude.LiveSession{PID: 11, SessionID: "q", StartedAt: t0}
	l12 := claude.LiveSession{PID: 12, SessionID: "r", StartedAt: t0}
	for _, live := range [][]claude.LiveSession{{l12, l11, l10}, {l10, l11, l12}} {
		got := Merge(nil, live, panes, table)
		if p, _ := byID(got, "p"); p.State != Running {
			t.Errorf("pane pid should win: %+v", got)
		}
	}
	for _, live := range [][]claude.LiveSession{{l12, l11}, {l11, l12}} {
		got := Merge(nil, live, panes, table)
		if q, _ := byID(got, "q"); q.State != Running {
			t.Errorf("shallowest should win: %+v", got)
		}
		if r, _ := byID(got, "r"); r.State != External {
			t.Errorf("deeper should be External: %+v", got)
		}
	}
}
