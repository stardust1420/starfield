package src

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stardust1420/starfield/internal/sessions"
)

var cardNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func plain(s string) string { return sgr.ReplaceAllString(s, "") }

func TestSessionEntriesOneCardPerSession(t *testing.T) {
	ss := []sessions.Session{
		{ID: "a", Dir: "/home/me/Developer/starfield", Title: "A", State: sessions.Running},
		{ID: "c", Dir: "/home/me/Developer/starfield", Title: "C", State: sessions.Stopped},
	}
	entries, items := sessionEntries(ss, cardNow, "/home/me")
	if len(entries) != 2 || len(items) != 2 {
		t.Fatalf("got %d entries, %d items; want 2 of each", len(entries), len(items))
	}
	for i, e := range entries {
		if e.heading || !e.selectable {
			t.Errorf("entry %d = %+v; want a selectable card", i, e)
		}
		if items[i].ID != ss[i].ID {
			t.Errorf("items[%d] = %s; want %s", i, items[i].ID, ss[i].ID)
		}
	}
}

func TestCardBodyPerState(t *testing.T) {
	cases := []struct {
		name string
		s    sessions.Session
		want string
	}{
		{"idle", sessions.Session{State: sessions.Running, Status: "idle", Started: cardNow.Add(-2 * time.Hour)}, "● idle · up 2h"},
		{"busy", sessions.Session{State: sessions.Running, Status: "busy", Started: cardNow.Add(-2 * time.Hour)}, "◐ working · up 2h"},
		{"starting", sessions.Session{State: sessions.Starting}, "◔ starting…"},
		{"external", sessions.Session{State: sessions.External, PID: 11435}, "◌ external · pid 11435"},
		{"stopped", sessions.Session{State: sessions.Stopped, LastActive: cardNow.Add(-3 * time.Hour)}, "○ 3h ago"},
		{"stopped just now", sessions.Session{State: sessions.Stopped, LastActive: cardNow.Add(-10 * time.Second)}, "○ just now"},
	}
	for _, c := range cases {
		c.s.ID, c.s.Dir, c.s.Title = "id", "/x", "T"
		c.s.Branch, c.s.Model, c.s.LastPrompt = "main", "opus-5-5", "fix the bug"
		c.s.CostUSD, c.s.UserMessages = 10.22, 133
		body := sessionCard(c.s, cardNow, "", glyphBusy).body
		if len(body) != 4 {
			t.Fatalf("%s: %d body lines; want 4: %q", c.name, len(body), body)
		}
		if got := plain(body[3]); got != c.want {
			t.Errorf("%s: line 4 = %q; want %q", c.name, got, c.want)
		}
	}

	s := sessions.Session{ID: "id", Title: "T", Branch: "main", Model: "opus-5-5", LastPrompt: "fix the bug", CostUSD: 10.22, UserMessages: 133, State: sessions.Stopped, LastActive: cardNow}
	body := sessionCard(s, cardNow, "", glyphBusy).body
	for i, want := range []string{"⎇ main · opus-5-5", `"fix the bug"`, "$10.22 · 133 msgs"} {
		if got := plain(body[i]); got != want {
			t.Errorf("line %d = %q; want %q", i+1, got, want)
		}
	}
}

func TestCardCostLine(t *testing.T) {
	cases := []struct {
		cost      float64
		msgs      int
		truncated bool
		want      string // "" means the line is left out
	}{
		{10.22, 133, false, "$10.22 · 133 msgs"},
		{10.22, 133, true, "$10.22"},
		{0, 1, false, "1 msg"},
		{0, 5, true, ""},
		{0, 0, false, ""},
	}
	for _, c := range cases {
		s := sessions.Session{ID: "id", Title: "T", State: sessions.Starting, CostUSD: c.cost, UserMessages: c.msgs, Truncated: c.truncated}
		body := sessionCard(s, cardNow, "", glyphBusy).body
		got := ""
		if len(body) == 2 {
			got = plain(body[0])
		}
		if got != c.want || (c.want == "" && len(body) != 1) {
			t.Errorf("cost %v msgs %d truncated %v: body %q; want cost line %q", c.cost, c.msgs, c.truncated, body, c.want)
		}
	}
}

func TestCardOmitsMissingFields(t *testing.T) {
	s := sessions.Session{ID: "id", Title: "T", LastPrompt: "hello", State: sessions.Stopped, LastActive: cardNow.Add(-time.Hour)}
	body := sessionCard(s, cardNow, "", glyphBusy).body
	want := []string{`"hello"`, "○ 1h ago"}
	if len(body) != len(want) {
		t.Fatalf("body = %q; want %q", body, want)
	}
	for i := range want {
		if got := plain(body[i]); got != want[i] {
			t.Errorf("line %d = %q; want %q", i+1, got, want[i])
		}
	}

	// A prompt that is already the title isn't repeated.
	s = sessions.Session{ID: "id", Title: "hello", LastPrompt: "hello", State: sessions.Starting}
	if body := sessionCard(s, cardNow, "", glyphBusy).body; len(body) != 1 {
		t.Errorf("body = %q; want only the state line", body)
	}
}

func TestRunningOnlyFilter(t *testing.T) {
	ss := []sessions.Session{
		{ID: "r", Title: "run", State: sessions.Running},
		{ID: "s", Title: "start", State: sessions.Starting},
		{ID: "e", Title: "ext", State: sessions.External},
		{ID: "x", Title: "old", State: sessions.Stopped},
	}
	ids := func(ss []sessions.Session) string {
		var b strings.Builder
		for _, s := range ss {
			b.WriteString(s.ID)
		}
		return b.String()
	}
	if got := ids(filterSessions(ss, false, "")); got != "rsex" {
		t.Errorf("all = %q; want rsex", got)
	}
	if got := ids(filterSessions(ss, true, "")); got != "rse" {
		t.Errorf("running only = %q; want rse", got)
	}
}

func TestFilterMatchesTitleDirBranchPrompt(t *testing.T) {
	ss := []sessions.Session{
		{ID: "1", Title: "Refactor parser"},
		{ID: "2", Dir: "/src/parser"},
		{ID: "3", Branch: "feat/parser"},
		{ID: "4", LastPrompt: "the PARSER breaks"},
		{ID: "5", Title: "other", Model: "parser"}, // model isn't searched
	}
	if got := len(filterSessions(ss, false, "parser")); got != 4 {
		t.Errorf("matched %d; want 4", got)
	}
}

func TestRelTime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-time.Minute, "just now"},
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m"},
		{3*time.Hour + 59*time.Minute, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{3 * 7 * 24 * time.Hour, "3w"},
		{125 * 24 * time.Hour, "4mo"},
		{800 * 24 * time.Hour, "2y"},
	}
	for _, c := range cases {
		if got := relTime(c.d); got != c.want {
			t.Errorf("relTime(%v) = %q; want %q", c.d, got, c.want)
		}
	}
}

func TestTildePath(t *testing.T) {
	cases := map[string]string{
		"/home/me":              "~",
		"/home/me/Developer/sf": "~/Developer/sf",
		"/home/meow":            "/home/meow",
		"/tmp":                  "/tmp",
	}
	for in, want := range cases {
		if got := tildePath(in, "/home/me"); got != want {
			t.Errorf("tildePath(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestDiscoveryErrorKeepsPreviousSessions(t *testing.T) {
	ss := []sessions.Session{{ID: "a", Dir: "/x", Title: "A", State: sessions.Stopped}}
	m := InitHomeModel(Deps{}, 100, 30)
	update := func(msg discoveredMsg) {
		next, _ := m.Update(msg)
		m = next.(homeModel)
	}
	update(discoveredMsg{id: m.tickID, sessions: ss})
	m = m.openProject("/x")
	update(discoveredMsg{id: m.tickID, err: errors.New("tmux broke")})

	if len(m.all) != 1 || m.emptyPage() || m.selected() == nil || m.selected().ID != "a" {
		t.Fatalf("after a failed discovery: all = %v, empty page = %v; want the previous card kept", m.all, m.emptyPage())
	}
	if !strings.Contains(m.status.text, "tmux broke") || m.status.kind != statusError {
		t.Errorf("status = %q; want the discovery error", m.status.text)
	}
	if !strings.Contains(plain(m.View()), "tmux broke") {
		t.Errorf("view doesn't show the error")
	}

	update(discoveredMsg{id: m.tickID, sessions: ss})
	if m.status.text != "" {
		t.Errorf("status after a clean discovery = %q; want cleared", m.status.text)
	}
}

func TestProjectCardStripsControlSequences(t *testing.T) {
	e := projectCard(project{Dir: "/w/ev\x1b[2Jil\x07", Branch: "ma\x1b]52;c;eA==\x07in"}, cardNow, "/home/me")
	for _, s := range append([]string{e.title}, e.body...) {
		if strings.Contains(s, "\x1b[2J") || strings.Contains(s, "\x07") || strings.Contains(s, "]52;") {
			t.Errorf("card keeps control sequences: %q", s)
		}
	}
	if e.title != "evil" {
		t.Errorf("title = %q", e.title)
	}
}
