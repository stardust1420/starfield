package src

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/dirs"
)

// testPicker opens a picker over cands from a home screen with fake ops.
func testPicker(t *testing.T, ops sessionOps, cands ...dirs.Candidate) pickerModel {
	t.Helper()
	back := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)
	back.ops = ops
	p := initPickerModel(back, cands, 100, 30)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(pickerModel)
}

func pickerKey(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// typeInto sends each rune of s as its own key press.
func typeInto(t *testing.T, p pickerModel, s string) pickerModel {
	t.Helper()
	for _, r := range s {
		next, _ := p.Update(pickerKey(string(r)))
		var ok bool
		if p, ok = next.(pickerModel); !ok {
			t.Fatalf("typing %q left the picker (%T)", r, next)
		}
	}
	return p
}

// runAll runs cmd and every command a tea.Batch inside it holds, and
// returns their messages. Ticks (frames) are skipped by the caller's choice
// of config; anything else just runs.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func rowPaths(p pickerModel) []string {
	out := make([]string, len(p.rows))
	for i, r := range p.rows {
		out[i] = r.cand.Path
	}
	return out
}

func TestPickerFiltersAsYouType(t *testing.T) {
	p := testPicker(t, &fakeOps{},
		dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent, LastUsed: time.Now()},
		dirs.Candidate{Path: "/w/acme-co", Source: dirs.Scanned},
		dirs.Candidate{Path: "/w/dotfiles", Source: dirs.Scanned},
		dirs.Candidate{Path: "/w/console", Source: dirs.Scanned},
	)
	if got := len(p.rows); got != 4 {
		t.Fatalf("empty query rows = %v; want all 4", rowPaths(p))
	}
	p = typeInto(t, p, "co")
	want := []string{"/w/console", "/w/acme-co"}
	if got := rowPaths(p); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v; want %v", got, want)
	}
	if p.cursor != 0 {
		t.Errorf("cursor = %d; want 0", p.cursor)
	}
}

func TestPickerEnterStartsInSelectedDir(t *testing.T) {
	f := &fakeOps{}
	dir := t.TempDir()
	p := testPicker(t, f,
		dirs.Candidate{Path: "/w/other", Source: dirs.Scanned},
		dirs.Candidate{Path: dir, Source: dirs.Recent, LastUsed: time.Now()},
	)
	next, _ := p.Update(pickerKey("down"))
	next, cmd := next.(pickerModel).Update(pickerKey("enter"))
	home, ok := next.(homeModel)
	if !ok {
		t.Fatalf("enter returned %T; want homeModel", next)
	}
	if len(home.starting) != 1 {
		t.Errorf("starting = %v; want the new session", home.starting)
	}
	var started *startedMsg
	for _, msg := range runAll(cmd) {
		if s, ok := msg.(startedMsg); ok {
			started = &s
		}
	}
	if started == nil {
		t.Fatal("no startedMsg")
	}
	if len(f.dirs) != 1 || f.dirs[0] != dir {
		t.Errorf("NewSession dirs = %v; want [%s]", f.dirs, dir)
	}
	if !home.starting[started.name] {
		t.Errorf("starting %v lacks %s", home.starting, started.name)
	}
}

func TestPickerTypedPathRowPinned(t *testing.T) {
	dir := t.TempDir()
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p = typeInto(t, p, dir)
	if len(p.rows) == 0 || p.rows[0].cand.Source != dirs.Typed || p.rows[0].cand.Path != dir {
		t.Fatalf("rows = %+v; want typed %s first", p.rows, dir)
	}
	if p.rows[0].bad {
		t.Errorf("existing dir marked bad")
	}
}

func TestPickerTypedMissingPathShowsError(t *testing.T) {
	f := &fakeOps{}
	p := testPicker(t, f, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p = typeInto(t, p, "/nope/zzz")
	next, cmd := p.Update(pickerKey("enter"))
	p, ok := next.(pickerModel)
	if !ok {
		t.Fatalf("enter on a missing dir left the picker (%T)", next)
	}
	runAll(cmd)
	if len(f.names) != 0 {
		t.Errorf("NewSession called: %v", f.names)
	}
	if v := plain(p.View()); !strings.Contains(v, "not a directory") {
		t.Errorf("view lacks error:\n%s", v)
	}
}

func TestPickerEscReturnsHome(t *testing.T) {
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p = typeInto(t, p, "q") // q is part of the query, not quit
	next, cmd := p.Update(pickerKey("esc"))
	if _, ok := next.(homeModel); !ok {
		t.Fatalf("esc returned %T; want homeModel", next)
	}
	if cmd == nil {
		t.Errorf("esc should restart the home screen's loops")
	}
}

func TestPickerTabCompletesSelected(t *testing.T) {
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/console", Source: dirs.Scanned})
	p = typeInto(t, p, "cns")
	next, _ := p.Update(pickerKey("tab"))
	p = next.(pickerModel)
	if p.field.value != "/w/console" {
		t.Errorf("query = %q; want /w/console", p.field.value)
	}
}

func TestPickerScanMergeKeepsSelection(t *testing.T) {
	p := testPicker(t, &fakeOps{},
		dirs.Candidate{Path: "/w/a", Source: dirs.Recent},
		dirs.Candidate{Path: "/w/b", Source: dirs.Recent},
	)
	next, _ := p.Update(pickerKey("down"))
	p = next.(pickerModel)
	next, _ = p.Update(scannedMsg{id: p.id, cands: []dirs.Candidate{
		{Path: "/w/new", Source: dirs.Recent},
		{Path: "/w/a", Source: dirs.Recent},
		{Path: "/w/b", Source: dirs.Recent},
		{Path: "/w/c", Source: dirs.Scanned},
	}})
	p = next.(pickerModel)
	if len(p.rows) != 4 || p.rows[p.cursor].cand.Path != "/w/b" {
		t.Errorf("rows = %v cursor = %d; want 4 rows on /w/b", rowPaths(p), p.cursor)
	}
}

func TestHomeNOpensPicker(t *testing.T) {
	m := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)
	next, cmd := m.Update(pickerKey("n"))
	if _, ok := next.(pickerModel); !ok {
		t.Fatalf("n returned %T; want pickerModel", next)
	}
	if cmd == nil {
		t.Errorf("n should start the scan")
	}
}

// A start the home screen launched can finish while the picker is open (the
// picker was reopened before startedMsg arrived): it must still attach.
func TestPickerForwardsStartedMsgToHome(t *testing.T) {
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p.back.markStarting("sf-12345678")
	next, cmd := p.Update(startedMsg{name: "sf-12345678", guard: "sf-12345678"})
	home, ok := next.(homeModel)
	if !ok {
		t.Fatalf("startedMsg returned %T; want homeModel", next)
	}
	if len(home.starting) != 0 {
		t.Errorf("starting = %v; want cleared", home.starting)
	}
	attached := false
	for _, msg := range runAll(cmd) {
		if strings.Contains(fmt.Sprintf("%T", msg), "execMsg") {
			attached = true
		}
	}
	if !attached {
		t.Error("no attach command returned")
	}
}

func TestPickerForwardsResultsToBack(t *testing.T) {
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p.back.markStarting("sf-12345678")
	next, _ := p.Update(startFailedMsg{name: "sf-12345678", err: errors.New("boom")})
	p, ok := next.(pickerModel)
	if !ok {
		t.Fatalf("startFailedMsg left the picker (%T)", next)
	}
	if len(p.back.starting) != 0 {
		t.Errorf("guard not cleared: %v", p.back.starting)
	}
	if p.back.status.text != "boom" {
		t.Errorf("back status = %q; want boom", p.back.status.text)
	}
	next, _ = p.Update(actionMsg{done: "stopped x"})
	if p = next.(pickerModel); p.back.status.text != "stopped x" {
		t.Errorf("actionMsg not forwarded: status %q", p.back.status.text)
	}
}

func TestPickerRowStripsControlSequences(t *testing.T) {
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/ev\x1b[2Jil\x07", Source: dirs.Scanned})
	row := p.renderRow(p.rows[0], true, 60)
	if strings.Contains(row, "\x1b[2J") || strings.Contains(row, "\x07") {
		t.Errorf("row keeps control sequences: %q", row)
	}
	if !strings.Contains(plain(row), "/w/evil") {
		t.Errorf("row = %q", plain(row))
	}
}
