package src

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
		next, cmd := p.Update(pickerKey(string(r)))
		var ok bool
		if p, ok = next.(pickerModel); !ok {
			t.Fatalf("typing %q left the picker (%T)", r, next)
		}
		p = deliverChecks(p, cmd)
	}
	return p
}

// deliverChecks runs cmd and feeds the path checks it made back to p, as
// the Bubble Tea runtime would.
func deliverChecks(p pickerModel, cmd tea.Cmd) pickerModel {
	for _, msg := range runAll(cmd) {
		if c, ok := msg.(pathCheckedMsg); ok {
			next, _ := p.Update(c)
			p = next.(pickerModel)
		}
	}
	return p
}

// realPath is p with its symlinks resolved (t.TempDir() is under a
// symlink on macOS: /tmp and /var/folders both live in /private).
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
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
	// The matches, best first, then the offer to create "co" (this config
	// has no roots, so under the directory starfield was launched from).
	want := []string{"/w/console", "/w/acme-co", filepath.Join(p.cwd, "co")}
	if got := rowPaths(p); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v; want %v", got, want)
	}
	if !p.rows[2].create {
		t.Errorf("last row should offer to create: %+v", p.rows[2])
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
	if len(f.dirs) != 1 || f.dirs[0] != realPath(t, dir) {
		t.Errorf("NewSession dirs = %v; want [%s]", f.dirs, realPath(t, dir))
	}
	if !home.starting[started.guard] {
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

func TestPickerTypedFileShowsError(t *testing.T) {
	f := &fakeOps{}
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := testPicker(t, f, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p = typeInto(t, p, file)
	if !p.rows[0].bad || p.rows[0].create {
		t.Fatalf("row = %+v; want bad, not create", p.rows[0])
	}
	next, cmd := p.Update(pickerKey("enter"))
	p, ok := next.(pickerModel)
	if !ok {
		t.Fatalf("enter on a file left the picker (%T)", next)
	}
	runAll(cmd)
	if len(f.names) != 0 {
		t.Errorf("NewSession called: %v", f.names)
	}
	if v := plain(p.View()); !strings.Contains(v, "not a directory:") {
		t.Errorf("view lacks the enter error:\n%s", v)
	}
}

func TestPickerTypedNewPathOffersCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "brand", "new")
	p := testPicker(t, &fakeOps{}, dirs.Candidate{Path: "/w/alpha", Source: dirs.Recent})
	p = typeInto(t, p, dir)
	r := p.rows[0]
	if r.cand.Path != dir || !r.create || r.bad {
		t.Fatalf("row = %+v; want create row for %s", r, dir)
	}
	v := plain(p.View())
	if !strings.Contains(v, "will be created") || !strings.Contains(v, "+ new") {
		t.Errorf("view doesn't offer to create:\n%s", v)
	}
	if !strings.Contains(v, "enter create & start") {
		t.Errorf("hint doesn't say enter creates:\n%s", v)
	}
}

// startedFrom runs a picker enter and returns home and the startedMsg.
func startedFrom(t *testing.T, p pickerModel) (homeModel, startedMsg, []tea.Msg) {
	t.Helper()
	next, cmd := p.Update(pickerKey("enter"))
	home, ok := next.(homeModel)
	if !ok {
		t.Fatalf("enter returned %T; want homeModel", next)
	}
	msgs := runAll(cmd)
	for _, msg := range msgs {
		if s, ok := msg.(startedMsg); ok {
			return home, s, msgs
		}
	}
	t.Fatalf("no startedMsg in %#v", msgs)
	return home, startedMsg{}, nil
}

func TestPickerEnterCreatesDirAndStarts(t *testing.T) {
	f := &fakeOps{}
	dir := filepath.Join(t.TempDir(), "brand", "new")
	p := typeInto(t, testPicker(t, f), dir)
	home, started, _ := startedFrom(t, p)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if len(f.dirs) != 1 || f.dirs[0] != realPath(t, dir) {
		t.Errorf("NewSession dirs = %v; want [%s]", f.dirs, realPath(t, dir))
	}
	if !home.starting[started.guard] {
		t.Errorf("starting %v lacks %s", home.starting, started.name)
	}
}

func TestPickerCreateFailureIsReported(t *testing.T) {
	f := &fakeOps{}
	parent := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o700) })
	dir := filepath.Join(parent, "new")
	p := typeInto(t, testPicker(t, f), dir)
	next, cmd := p.Update(pickerKey("enter"))
	var failed *startFailedMsg
	for _, msg := range runAll(cmd) {
		if s, ok := msg.(startFailedMsg); ok {
			failed = &s
		}
	}
	if failed == nil || !strings.Contains(failed.err.Error(), "create "+dir) {
		t.Fatalf("want startFailedMsg naming the dir, got %#v", failed)
	}
	if len(f.names) != 0 {
		t.Errorf("NewSession called after mkdir failed: %v", f.names)
	}
	_ = next
}

func TestPickerPlainNameOffersNewUnderFirstRoot(t *testing.T) {
	root := t.TempDir()
	back := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)
	back.deps.Cfg.Roots = []string{root, "/elsewhere"}
	back.ops = &fakeOps{}
	p := initPickerModel(back, []dirs.Candidate{{Path: "/w/myproject-api", Source: dirs.Recent}}, 100, 30)
	p = typeInto(t, p, "myproj")
	last := p.rows[len(p.rows)-1]
	if last.cand.Path != filepath.Join(root, "myproj") || !last.create {
		t.Fatalf("last row = %+v; want create %s/myproj", last, root)
	}
	if p.rows[0].cand.Path != "/w/myproject-api" {
		t.Errorf("fuzzy match should stay first: %+v", p.rows[0])
	}

	// A name that already exists under the root is offered as is.
	if err := os.Mkdir(filepath.Join(root, "made"), 0o755); err != nil {
		t.Fatal(err)
	}
	p = typeInto(t, initPickerModel(back, nil, 100, 30), "made")
	if r := p.rows[len(p.rows)-1]; r.cand.Path != filepath.Join(root, "made") || r.create || r.bad {
		t.Errorf("existing dir row = %+v; want plain path row", r)
	}

	// Names that aren't a single path element get no create row.
	for _, q := range []string{"..", "a b/c"} {
		p = typeInto(t, initPickerModel(back, nil, 100, 30), q)
		for _, r := range p.rows {
			if r.create {
				t.Errorf("query %q offered to create %s", q, r.cand.Path)
			}
		}
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

func TestPickerLongQueryStaysOnOneLine(t *testing.T) {
	long := "/var/folders/fd/jbqyk_0d5k5g7dw7jzlqq6z80000gn/T/TestPickerSomethingLong2182259913/001/brand/new-project"
	p := typeInto(t, testPicker(t, &fakeOps{}), long)
	v := plain(p.View())
	if !strings.Contains(v, "new-project▏") {
		t.Errorf("prompt should end with the end of the query:\n%s", v)
	}
	if !strings.Contains(v, "will be created") {
		t.Errorf("a long query pushed the rows out of the panel:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line wider than the window (%d): %q", w, line)
		}
	}
}
