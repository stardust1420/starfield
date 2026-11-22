package src

import (
	"errors"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/sessions"
)

type fakeOps struct {
	names  []string
	dirs   []string
	argvs  [][]string
	killed []string
	newErr error

	capture      string
	captureErr   error
	captureLines int
}

func (f *fakeOps) AttachCmd(name string) *exec.Cmd { return exec.Command("true") }
func (f *fakeOps) NewSession(name, dir, id string, argv []string, w, h int) error {
	f.names = append(f.names, name)
	f.dirs = append(f.dirs, dir)
	f.argvs = append(f.argvs, argv)
	return f.newErr
}
func (f *fakeOps) Capture(name string, lines int) (string, error) {
	f.captureLines = lines
	return f.capture, f.captureErr
}
func (f *fakeOps) Kill(name string) error { f.killed = append(f.killed, name); return nil }

// homeWith builds a home screen showing ss with the cursor on the first
// session and fake ops.
func homeWith(t *testing.T, ops sessionOps, ss ...sessions.Session) homeModel {
	t.Helper()
	m := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)
	m.ops = ops
	next, _ := m.Update(discoveredMsg{id: m.tickID, sessions: ss})
	m = next.(homeModel)
	if m.selected() == nil {
		t.Fatalf("no session selected")
	}
	return m
}

func press(m homeModel, k string) (homeModel, tea.Cmd) {
	var msg tea.KeyMsg
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	next, cmd := m.Update(msg)
	return next.(homeModel), cmd
}

func testCfg() config.Config {
	return config.Config{ClaudeCmd: []string{"caffeinate", "-is", "claude"}}
}

const testID = "12345678-aaaa-4bbb-8ccc-123456789abc"

func TestResumeArgv(t *testing.T) {
	f := &fakeOps{}
	cfg := testCfg()
	msg := resumeCmd(f, cfg, sessions.Session{ID: testID, Dir: "/x"}, 80, 24)()
	if got, ok := msg.(startedMsg); !ok || got.name != "sf-12345678" {
		t.Fatalf("msg = %#v; want startedMsg sf-12345678", msg)
	}
	want := []string{"caffeinate", "-is", "claude", "--resume", testID}
	if len(f.argvs) != 1 || !reflect.DeepEqual(f.argvs[0], want) {
		t.Errorf("argv = %v; want %v", f.argvs, want)
	}
	if len(cfg.ClaudeCmd) != 3 {
		t.Errorf("ClaudeCmd mutated: %v", cfg.ClaudeCmd)
	}
}

func TestStartArgvUsesFreshUUID(t *testing.T) {
	f := &fakeOps{}
	msg := startCmd(f, testCfg(), "/x", 80, 24)()
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	argv := f.argvs[0]
	if len(argv) != 5 || argv[3] != "--session-id" || !re.MatchString(argv[4]) {
		t.Fatalf("argv = %v", argv)
	}
	if got := msg.(startedMsg).name; got != "sf-"+argv[4][:8] {
		t.Errorf("name = %q", got)
	}
	if newUUID() == newUUID() {
		t.Errorf("uuids repeat")
	}
}

func TestStartFailureIsReported(t *testing.T) {
	f := &fakeOps{newErr: errors.New("boom")}
	msg := startCmd(f, testCfg(), "/x", 80, 24)()
	m := homeWith(t, f, sessions.Session{ID: "a", Dir: "/x", State: sessions.Stopped})
	next, cmd := m.Update(msg)
	m = next.(homeModel)
	if cmd != nil || m.status.kind != statusError || m.status.text != "boom" {
		t.Errorf("status = %q, cmd = %v", m.status.text, cmd)
	}
}

func TestResumeMissingDirShowsError(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: "/definitely/not/here", Title: "t", State: sessions.Stopped})
	m, cmd := press(m, "enter")
	if m.status.kind != statusError || !regexp.MustCompile("directory gone").MatchString(m.status.text) {
		t.Errorf("status = %q", m.status.text)
	}
	if cmd != nil {
		cmd()
	}
	if len(f.names) != 0 {
		t.Errorf("NewSession called: %v", f.names)
	}
}

func TestResumeStoppedStartsSession(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: t.TempDir(), Title: "t", State: sessions.Stopped})
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("no cmd")
	}
	msg := cmd()
	if len(f.names) != 1 || f.names[0] != "sf-12345678" {
		t.Fatalf("calls = %v", f.names)
	}
	_, cmd = m.Update(msg)
	if cmd == nil {
		t.Errorf("startedMsg should return an attach cmd")
	}
}

func TestEnterOnExternalDoesNotAttach(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: "/x", State: sessions.External, PID: 42})
	m, cmd := press(m, "enter")
	if cmd != nil {
		t.Errorf("enter on external returned a cmd")
	}
	if !regexp.MustCompile(`pid 42`).MatchString(m.status.text) {
		t.Errorf("status = %q", m.status.text)
	}
	if len(f.names) != 0 {
		t.Errorf("NewSession called")
	}
}

func TestAttachDoneWithExitedSessionIsNotAnError(t *testing.T) {
	m := homeWith(t, &fakeOps{}, sessions.Session{ID: testID, Dir: "/x", State: sessions.Running})
	next, cmd := m.Update(attachDoneMsg{name: "sf-x", err: &exec.ExitError{}})
	m = next.(homeModel)
	if m.status.text != "" {
		t.Errorf("status = %q; want empty", m.status.text)
	}
	if cmd == nil {
		t.Errorf("want a refresh cmd")
	}
	next, _ = m.Update(attachDoneMsg{name: "sf-x", err: errors.New("no tty")})
	if next.(homeModel).status.kind != statusError {
		t.Errorf("other errors should show")
	}
}

func TestStopRequiresConfirm(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: "/x", Title: "my task", State: sessions.Running, TmuxName: "sf-12345678"})
	m, _ = press(m, "x")
	if m.confirm == nil || m.confirm.title != "stop my task?" || m.confirm.yes != "stop" {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	if len(f.killed) != 0 {
		t.Fatalf("killed before confirmation")
	}
	m, cmd := press(m, "y")
	if cmd == nil {
		t.Fatal("no cmd after y")
	}
	msg := cmd()
	if len(f.killed) != 1 || f.killed[0] != "sf-12345678" {
		t.Errorf("killed = %v", f.killed)
	}
	next, _ := m.Update(msg)
	if got := next.(homeModel).status.text; got != "stopped my task" {
		t.Errorf("status = %q", got)
	}
}

func TestStopCancelAndHints(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: "/x", Title: "t", State: sessions.Running})
	m, _ = press(m, "x")
	m, _ = press(m, "n")
	if m.confirm != nil || len(f.killed) != 0 {
		t.Errorf("n should cancel")
	}
	ext := homeWith(t, f, sessions.Session{ID: "e", Dir: "/x", State: sessions.External})
	ext, _ = press(ext, "x")
	if ext.confirm != nil || ext.status.text == "" {
		t.Errorf("external: confirm=%v status=%q", ext.confirm, ext.status.text)
	}
	st := homeWith(t, f, sessions.Session{ID: "s", Dir: "/x", State: sessions.Stopped})
	st, _ = press(st, "x")
	if st.confirm != nil || st.status.text != "already stopped" {
		t.Errorf("stopped: status=%q", st.status.text)
	}
}

func TestNilOpsShowsUnavailable(t *testing.T) {
	m := homeWith(t, nil, sessions.Session{ID: "a", Dir: "/x", State: sessions.Running})
	m, _ = press(m, "enter")
	if m.status.text != "tmux unavailable" {
		t.Errorf("status = %q", m.status.text)
	}
}

func TestDoubleEnterStartsOnce(t *testing.T) {
	f := &fakeOps{}
	m := homeWith(t, f, sessions.Session{ID: testID, Dir: t.TempDir(), Title: "t", State: sessions.Stopped})
	m, c1 := press(m, "enter")
	m, c2 := press(m, "enter")
	if c1 == nil || c2 != nil {
		t.Fatalf("first cmd nil=%v, second nil=%v; want only the first", c1 == nil, c2 == nil)
	}
	msg := c1()
	if len(f.names) != 1 {
		t.Errorf("NewSession calls = %v; want 1", f.names)
	}
	next, _ := m.Update(msg)
	if len(next.(homeModel).starting) != 0 {
		t.Errorf("starting not cleared")
	}
}

func TestDuplicateSessionErrorAttachesAnyway(t *testing.T) {
	f := &fakeOps{newErr: errors.New("new-session: exit 1: duplicate session: sf-12345678")}
	msg := resumeCmd(f, testCfg(), sessions.Session{ID: testID, Dir: "/x"}, 80, 24)()
	if got, ok := msg.(startedMsg); !ok || got.name != "sf-12345678" {
		t.Errorf("msg = %#v; want startedMsg", msg)
	}
}

func TestErrorStatusReadableAtNarrowWidths(t *testing.T) {
	const errText = "directory gone: /Users/me/Developer/some-project"
	for _, w := range []int{80, 100} {
		m := homeWith(t, &fakeOps{}, sessions.Session{ID: testID, Dir: "/Users/me/Developer/some-project", Title: "t", State: sessions.Stopped})
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m = next.(homeModel)
		m, _ = press(m, "enter")
		if v := plain(m.View()); !strings.Contains(v, errText) {
			t.Errorf("width %d: view lacks %q:\n%s", w, errText, v)
		}
	}
}

func TestConcurrentStartsTrackEachName(t *testing.T) {
	f := &fakeOps{}
	m := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)
	m.ops = f
	m, c1 := m.startIn(t.TempDir())
	m, c2 := m.startIn(t.TempDir())
	if len(m.starting) != 2 {
		t.Fatalf("starting = %v; want 2 names", m.starting)
	}
	msg1, msg2 := c1(), c2()
	next, _ := m.Update(msg1)
	m = next.(homeModel)
	if len(m.starting) != 1 || !m.starting[msg2.(startedMsg).name] {
		t.Errorf("after first start: starting = %v; want only %v", m.starting, msg2)
	}
	next, _ = m.Update(startFailedMsg{name: msg2.(startedMsg).name, err: errors.New("x")})
	if got := next.(homeModel).starting; len(got) != 0 {
		t.Errorf("after failure: starting = %v; want empty", got)
	}
}
