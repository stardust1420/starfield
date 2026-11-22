package src

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/sessions"
)

// twoProjects is a discovery across two directories, in display order: the
// live session first, as Merge sorts them.
func twoProjects(now time.Time) []sessions.Session {
	return []sessions.Session{
		{ID: "r1", Dir: "/w/alpha", Title: "fix parser", Branch: "main", State: sessions.Running, Status: "idle", CostUSD: 2.5, LastActive: now.Add(-time.Hour)},
		{ID: "s1", Dir: "/w/beta", Title: "write docs", State: sessions.Stopped, CostUSD: 1, LastActive: now.Add(-2 * time.Hour)},
		{ID: "s2", Dir: "/w/alpha", Title: "old idea", State: sessions.Stopped, CostUSD: 0.5, LastActive: now.Add(-3 * time.Hour)},
	}
}

// homeLoaded is the home screen after one discovery of ss, on the projects view.
func homeLoaded(t *testing.T, ops sessionOps, w, h int, ss []sessions.Session) homeModel {
	t.Helper()
	m := InitHomeModel(Deps{Cfg: testCfg()}, w, h)
	m.historyDays = 0
	m.ops = ops
	next, _ := m.Update(discoveredMsg{id: m.tickID, sessions: ss})
	return next.(homeModel)
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func send(t *testing.T, m homeModel, keys ...string) homeModel {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		var ok bool
		if m, ok = next.(homeModel); !ok {
			t.Fatalf("key %q left the home screen (%T)", k, next)
		}
	}
	return m
}

func TestHomeListsProjectsNotSessions(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	if m.project != "" {
		t.Fatalf("home should open on the projects view, got project %q", m.project)
	}
	var titles []string
	for _, e := range m.grid.entries {
		if e.heading {
			t.Errorf("projects view has a heading: %+v", e)
		}
		titles = append(titles, e.title)
	}
	if strings.Join(titles, ",") != "alpha,beta" {
		t.Errorf("project cards = %v; want [alpha beta] (live first)", titles)
	}
	if p := m.selectedProject(); p == nil || p.Dir != "/w/alpha" {
		t.Errorf("selected project = %+v; want /w/alpha", p)
	}
	if m.selected() != nil {
		t.Error("no session should be selected on the projects view")
	}
}

func TestProjectCardSummarisesItsSessions(t *testing.T) {
	now := time.Now()
	ps := groupProjects(twoProjects(now))
	if len(ps) != 2 {
		t.Fatalf("projects = %+v", ps)
	}
	a := ps[0]
	if a.Running != 1 || a.Stopped != 1 || a.External != 0 || a.CostUSD != 3 || a.Branch != "main" {
		t.Errorf("alpha = %+v", a)
	}
	body := plain(strings.Join(projectCard(a, now, "/home/u").body, "\n"))
	for _, want := range []string{"/w/alpha", "⎇ main", "● 1 running", "○ 1 stopped", "$3.00", "1h ago"} {
		if !strings.Contains(body, want) {
			t.Errorf("alpha card lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "external") {
		t.Errorf("zero counts should be left out:\n%s", body)
	}
}

func TestEnterOpensProjectSessions(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	m = send(t, m, "enter")
	if m.project != "/w/alpha" {
		t.Fatalf("project = %q; want /w/alpha", m.project)
	}
	var ids []string
	for i, e := range m.grid.entries {
		if e.heading {
			t.Errorf("sessions view has a heading: %+v", e)
		}
		ids = append(ids, m.items[i].ID)
	}
	if strings.Join(ids, ",") != "r1,s2" {
		t.Errorf("sessions = %v; want [r1 s2]", ids)
	}
	if v := plain(m.View()); !strings.Contains(v, "✦ starfield › alpha") {
		t.Errorf("header should name the project:\n%s", strings.Split(v, "\n")[0])
	}
}

func TestEscReturnsToProjectsWithSelectionAndFilter(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	// Filter the projects to beta, open it, filter its sessions, then back out.
	m = send(t, m, "/", "b", "e", "enter")
	if p := m.selectedProject(); p == nil || p.Dir != "/w/beta" {
		t.Fatalf("filtered selection = %+v; want beta", p)
	}
	m = send(t, m, "enter")
	if m.project != "/w/beta" {
		t.Fatalf("project = %q", m.project)
	}
	if m.filter.query() != "" {
		t.Errorf("the sessions view should start unfiltered, got %q", m.filter.query())
	}
	m = send(t, m, "/", "z", "enter")
	m = send(t, m, "esc") // clears the session filter first
	if m.project != "/w/beta" || m.filter.query() != "" {
		t.Fatalf("first esc should clear the filter and stay: project %q filter %q", m.project, m.filter.query())
	}
	m = send(t, m, "esc")
	if m.project != "" {
		t.Fatal("second esc should go back to the projects view")
	}
	if m.filter.query() != "be" {
		t.Errorf("projects filter = %q; want it restored to be", m.filter.query())
	}
	if p := m.selectedProject(); p == nil || p.Dir != "/w/beta" {
		t.Errorf("selection after back = %+v; want beta", p)
	}
}

func TestProjectsFilterAndRunningOnly(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	m = send(t, m, "/", "w", "/", "b", "enter") // matches the path /w/beta
	if len(m.grid.entries) != 1 || m.grid.entries[0].title != "beta" {
		t.Errorf("filter by path: %v", m.grid.entries)
	}
	m = send(t, m, "esc", "a")
	if len(m.grid.entries) != 1 || m.grid.entries[0].title != "alpha" {
		t.Errorf("running only should keep projects with a live session: %v", m.grid.entries)
	}
}

func TestHistoryWindowAppliesInBothViews(t *testing.T) {
	now := time.Now()
	ss := twoProjects(now)
	ss[1].LastActive = now.Add(-10 * 24 * time.Hour) // beta's only session is old
	ss[2].LastActive = now.Add(-10 * 24 * time.Hour) // and alpha's stopped one
	m := homeLoaded(t, &fakeOps{}, 120, 40, ss)
	m.historyDays = 3
	m.rebuild()
	if len(m.grid.entries) != 1 || m.grid.entries[0].title != "alpha" {
		t.Errorf("3d window projects = %v; want only alpha", m.grid.entries)
	}
	m = send(t, m, "enter")
	if len(m.items) != 1 || m.items[0].ID != "r1" {
		t.Errorf("3d window sessions = %v; want only the running r1", m.items)
	}
	m = send(t, m, "t") // 3d → 7d, still inside the project
	if m.project != "/w/alpha" || m.historyDays != 7 {
		t.Errorf("t in the sessions view: project %q, days %d", m.project, m.historyDays)
	}
}

func TestNewInProjectStartsThereWithoutPicker(t *testing.T) {
	f := &fakeOps{}
	m := homeLoaded(t, f, 120, 40, twoProjects(time.Now()))
	m = send(t, m, "enter")
	next, cmd := m.Update(keyMsg("n"))
	if _, ok := next.(homeModel); !ok {
		t.Fatalf("n in a project returned %T; want the same screen", next)
	}
	runAll(cmd)
	if len(f.dirs) != 1 || f.dirs[0] != "/w/alpha" {
		t.Errorf("NewSession dirs = %v; want [/w/alpha]", f.dirs)
	}
}

func TestNewOnProjectsOpensPicker(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	next, _ := m.Update(keyMsg("n"))
	if _, ok := next.(pickerModel); !ok {
		t.Fatalf("n on the projects view returned %T; want the picker", next)
	}
}

func TestPickerStartLandsInThatProject(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	next, _ := m.Update(keyMsg("n"))
	p := next.(pickerModel)
	p = typeInto(t, p, t.TempDir())
	dir := p.rows[0].cand.Path
	next, _ = p.Update(pickerKey("enter"))
	home, ok := next.(homeModel)
	if !ok {
		t.Fatalf("enter returned %T", next)
	}
	if home.project != dir {
		t.Errorf("after starting in %s, home shows project %q; want that project", dir, home.project)
	}
}

func TestBannerOnProjectsViewWhenThereIsRoom(t *testing.T) {
	m := homeLoaded(t, &fakeOps{}, 120, 40, twoProjects(time.Now()))
	v := plain(m.View())
	if !strings.Contains(v, "███████╗████████╗ █████╗ ██████╗") {
		t.Errorf("projects view lacks the banner:\n%s", v)
	}
	if lines := strings.Split(v, "\n"); len(lines) != 40 {
		t.Errorf("page is %d lines; want exactly the window (40)", len(lines))
	}
	if strings.Contains(plain(send(t, m, "enter").View()), "████████╗") {
		t.Error("the sessions view shouldn't show the banner")
	}
	small := homeLoaded(t, &fakeOps{}, 120, 22, twoProjects(time.Now()))
	if strings.Contains(plain(small.View()), "████████╗") {
		t.Error("a short window should drop the banner to leave room for cards")
	}
}

func TestBannerSpellsStarfield(t *testing.T) {
	lines := bannerLines()
	if len(lines) != 6 {
		t.Fatalf("banner is %d lines; want 6", len(lines))
	}
	w := len([]rune(lines[0]))
	for i, l := range lines {
		if n := len([]rune(l)); n != w {
			t.Errorf("banner line %d is %d wide; want %d (lines must line up)", i, n, w)
		}
	}
}
