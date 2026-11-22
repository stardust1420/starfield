package src

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stardust1420/starfield/internal/sessions"
)

// Sessions whose history has no directory get an "(unknown directory)"
// card; opening it must show them, not leave the view half switched.
func TestUnknownDirectoryProjectOpensAndBacksOut(t *testing.T) {
	now := time.Now()
	f := &fakeOps{}
	m := homeLoaded(t, f, 120, 40, []sessions.Session{
		{ID: "a", Dir: "/w/alpha", Title: "known", State: sessions.Stopped, LastActive: now},
		{ID: "u", Dir: "", Title: "lost", State: sessions.Stopped, LastActive: now.Add(-time.Hour)},
	})
	m = send(t, m, "/", "u", "n", "k", "enter")
	if p := m.selectedProject(); p == nil || p.Dir != "" {
		t.Fatalf("selected = %+v; want the unknown-directory card", p)
	}
	m = send(t, m, "enter")
	if m.project == "" {
		t.Fatal("enter on the unknown-directory card stayed on the projects view")
	}
	if len(m.items) != 1 || m.items[0].ID != "u" {
		t.Errorf("sessions = %v; want [u]", m.items)
	}
	if v := plain(m.View()); !strings.Contains(v, "projects › (unknown directory)") {
		t.Errorf("breadcrumb:\n%s", strings.Split(v, "\n")[0])
	}
	next, cmd := m.Update(keyMsg("n"))
	m = next.(homeModel)
	runAll(cmd)
	if len(f.names) != 0 || m.status.kind != statusError {
		t.Errorf("n with no directory: NewSession %v, status %q", f.names, m.status.text)
	}
	m = send(t, m, "esc")
	if m.project != "" || m.filter.query() != "unk" {
		t.Errorf("esc: project %q, filter %q; want projects view with filter unk", m.project, m.filter.query())
	}
	if p := m.selectedProject(); p == nil || p.Dir != "" {
		t.Errorf("selection after back = %+v; want the unknown-directory card", p)
	}
}

// A start in a path with a symlink in it runs in the real path, which is
// where Claude records the session, and the view follows it there.
func TestStartResolvesSymlinks(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	link := filepath.Join(base, "link")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	f := &fakeOps{}
	m := homeLoaded(t, f, 120, 40, nil).openProject(link)
	m, cmd := m.startIn(link)
	msgs := runAll(cmd)
	if len(f.dirs) != 1 || f.dirs[0] != realPath(t, real) {
		t.Fatalf("NewSession dirs = %v; want [%s]", f.dirs, realPath(t, real))
	}
	next, _ := m.Update(msgs[0])
	if got := next.(homeModel).project; got != realPath(t, real) {
		t.Errorf("project after start = %q; want the real path %q", got, realPath(t, real))
	}
}

// A second n before the first session is up must not start another.
func TestDoubleNewStartsOneSession(t *testing.T) {
	dir := t.TempDir()
	f := &fakeOps{}
	m := homeLoaded(t, f, 120, 40, []sessions.Session{{ID: "a", Dir: dir, State: sessions.Stopped, LastActive: time.Now()}})
	m = send(t, m, "enter")
	m, c1 := press(m, "n")
	m, c2 := press(m, "n")
	runAll(c1)
	runAll(c2)
	if len(f.names) != 1 {
		t.Errorf("NewSession calls = %v; want one", f.names)
	}
	if m.status.text != "starting…" {
		t.Errorf("status = %q; want starting…", m.status.text)
	}
}

// A success status set before a resume (back from the picker or an
// attach, which changes the tick id) must still clear.
func TestSuccessStatusClearsAfterResume(t *testing.T) {
	fastStatus(t)
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	cmd := m.setStatus(statusSuccess, "stopped sessions from the last 7 days")
	m = m.resume(120, 30)
	for _, msg := range runAll(cmd) {
		next, _ := m.Update(msg)
		m = next.(homeModel)
	}
	if m.status.text != "" {
		t.Errorf("status = %q; want it cleared", m.status.text)
	}
}

// Widening the window while a discovery with the old one runs fetches the
// wider window as soon as that discovery returns.
func TestWideningDuringDiscoveryRefetches(t *testing.T) {
	countTicks(t)
	fastStatus(t)
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	m.historyDays = 3
	next, _ := m.Update(tickMsg{id: m.tickID})
	m = next.(homeModel)
	m, _ = m.cycleHistory() // 3 → 7 while the 3-day discovery runs
	next, cmd := m.Update(discoveredMsg{id: m.tickID, fromTick: true, days: 3})
	m = next.(homeModel)
	var days []int
	for _, msg := range runAll(cmd) {
		if d, ok := msg.(discoveredMsg); ok {
			days = append(days, d.days)
		}
	}
	if len(days) != 1 || days[0] != 7 {
		t.Errorf("discoveries after the stale result = %v; want one with 7 days", days)
	}
	// A result that covers the window starts nothing more.
	m.inFlight = false
	_, cmd = m.Update(discoveredMsg{id: m.tickID, days: 7})
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(discoveredMsg); ok {
			t.Error("a covering result started another discovery")
		}
	}
}

func TestWindowCovers(t *testing.T) {
	for _, c := range []struct {
		had, want int
		ok        bool
	}{{0, 0, true}, {0, 7, true}, {14, 7, true}, {7, 7, true}, {3, 7, false}, {14, 0, false}} {
		if got := windowCovers(c.had, c.want); got != c.ok {
			t.Errorf("windowCovers(%d, %d) = %v; want %v", c.had, c.want, got, c.ok)
		}
	}
}

// Typed paths are checked in a command, not in Update.
func TestPickerChecksTypedPathsInACommand(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p := typeInto(t, testPicker(t, &fakeOps{}), file[:len(file)-1])
	next, cmd := p.Update(pickerKey(file[len(file)-1:]))
	p = next.(pickerModel)
	if !p.rows[0].unchecked || p.rows[0].bad {
		t.Fatalf("before its check the row is %+v; want unchecked", p.rows[0])
	}
	p = deliverChecks(p, cmd)
	if !p.rows[0].bad {
		t.Errorf("after its check the row is %+v; want bad", p.rows[0])
	}
}
