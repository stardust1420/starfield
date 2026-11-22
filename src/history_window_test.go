package src

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/sessions"
)

func tKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")} }

// fastStatus makes success and error statuses clear at once, so runAll doesn't wait
// out the real delay.
func fastStatus(t *testing.T) {
	orig, origErr := statusClearDelay, errorClearDelay
	statusClearDelay, errorClearDelay = time.Millisecond, time.Millisecond
	t.Cleanup(func() { statusClearDelay, errorClearDelay = orig, origErr })
}

func TestHistoryKeyCyclesTheWindow(t *testing.T) {
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	m.historyDays = 14
	var got []int
	for range 7 {
		next, _ := m.Update(tKey())
		m = next.(homeModel)
		got = append(got, m.historyDays)
	}
	want := []int{30, 90, 0, 1, 3, 7, 14}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("t presses gave %v; want %v", got, want)
		}
	}

	// A value that isn't a step (from STARFIELD_HISTORY_DAYS=4) moves to
	// the next step up.
	m.historyDays = 4
	next, _ := m.Update(tKey())
	if d := next.(homeModel).historyDays; d != 7 {
		t.Errorf("from 4 days, t gave %d; want 7", d)
	}
}

func TestHistoryWindowShowsInHeader(t *testing.T) {
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	next, _ := m.Update(discoveredMsg{id: m.tickID, sessions: []sessions.Session{
		{ID: "a", Dir: "/a", State: sessions.Stopped, LastActive: time.Now()},
	}})
	m = next.(homeModel)
	m.historyDays = 14
	if v := plain(m.View()); !strings.Contains(v, "◷ 14d") {
		t.Errorf("header lacks the window:\n%s", v)
	}
	m.historyDays = 0
	if v := plain(m.View()); !strings.Contains(v, "◷ all time") {
		t.Errorf("header lacks 'all time':\n%s", v)
	}
}

func TestEmptyCopyPointsAtTheWindow(t *testing.T) {
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	m.historyDays = 3
	// Stopped sessions exist but all are older than the window.
	next, _ := m.Update(discoveredMsg{id: m.tickID, sessions: []sessions.Session{
		{ID: "old", Dir: "/b", State: sessions.Stopped, LastActive: time.Now().Add(-10 * 24 * time.Hour)},
	}})
	m = next.(homeModel)
	if v := plain(m.View()); !strings.Contains(v, "No sessions from the last 3 days · t to look further back") {
		t.Errorf("hidden-by-window copy missing:\n%s", v)
	}
	// Discovery found nothing in the window at all.
	next, _ = m.Update(discoveredMsg{id: m.tickID})
	m = next.(homeModel)
	if v := plain(m.View()); !strings.Contains(v, "No sessions from the last 3 days") || !strings.Contains(v, "to look further back") {
		t.Errorf("empty-page copy should point at t:\n%s", v)
	}
}

func TestNarrowingTheWindowHidesOlderStoppedAtOnce(t *testing.T) {
	now := time.Now()
	m := InitHomeModel(Deps{Cfg: testCfg()}, 120, 30)
	m.historyDays = 0
	next, _ := m.Update(discoveredMsg{id: m.tickID, sessions: []sessions.Session{
		{ID: "run", Dir: "/a", State: sessions.Running, LastActive: now.Add(-40 * 24 * time.Hour)},
		{ID: "new", Dir: "/a", State: sessions.Stopped, LastActive: now.Add(-2 * time.Hour)},
		{ID: "old", Dir: "/b", State: sessions.Stopped, LastActive: now.Add(-10 * 24 * time.Hour)},
	}})
	m = next.(homeModel)
	if len(m.projItems) != 2 {
		t.Fatalf("all time shows %d projects; want 2", len(m.projItems))
	}
	next, _ = m.Update(tKey()) // all time → 1 day
	m = next.(homeModel)
	if len(m.projItems) != 1 || m.projItems[0].Dir != "/a" {
		t.Errorf("1 day shows projects %v; want only /a (b's only session is old)", m.projItems)
	}
	if !strings.Contains(m.status.text, "1 day") {
		t.Errorf("status = %q; want it to name the new window", m.status.text)
	}
	m = m.openProject("/a")
	var ids []string
	for _, s := range m.shown {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "run,new" {
		t.Errorf("1 day shows %v; want [run new] (running sessions ignore the window)", ids)
	}
}

func TestHistoryKeyRefreshesAndSaves(t *testing.T) {
	fastStatus(t)
	cfg := testCfg()
	cfg.StateDir = filepath.Join(t.TempDir(), "starfield")
	m := InitHomeModel(Deps{Cfg: cfg}, 120, 30)
	m.historyDays = 14
	next, cmd := m.Update(tKey())
	m = next.(homeModel)
	var refreshed bool
	for _, msg := range runAll(cmd) {
		if d, ok := msg.(discoveredMsg); ok && !d.fromTick {
			refreshed = true
		}
	}
	if !refreshed {
		t.Error("changing the window should refresh at once")
	}
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "settings.json"))
	if err != nil {
		t.Fatalf("settings not saved: %v", err)
	}
	var saved struct {
		HistoryDays *int `json:"history_days"`
	}
	if json.Unmarshal(b, &saved); saved.HistoryDays == nil || *saved.HistoryDays != 30 {
		t.Errorf("saved %s; want history_days 30", b)
	}
}

func TestHistoryKeySaveFailureIsShown(t *testing.T) {
	fastStatus(t)
	cfg := testCfg()
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o644)
	cfg.StateDir = filepath.Join(file, "starfield") // can't be created
	m := InitHomeModel(Deps{Cfg: cfg}, 120, 30)
	m.historyDays = 14
	_, cmd := m.Update(tKey())
	var failed bool
	for _, msg := range runAll(cmd) {
		if s, ok := msg.(settingsSavedMsg); ok && s.err != nil {
			failed = true
			next, _ := m.Update(s)
			if st := next.(homeModel).status; st.kind != statusError || !strings.Contains(st.text, "save") {
				t.Errorf("status = %+v; want a save error", st)
			}
		}
	}
	if !failed {
		t.Error("no failed settingsSavedMsg")
	}
}
