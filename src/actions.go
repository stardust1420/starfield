package src

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/sessions"
	"github.com/stardust1420/starfield/internal/tmux"
)

// sessionOps is the part of the tmux client the actions use. *tmux.Client
// satisfies it; tests use a fake.
type sessionOps interface {
	AttachCmd(name string) *exec.Cmd
	NewSession(name, dir, id string, argv []string, w, h int) error
	Kill(name string) error
	Capture(name string, lines int) (string, error)
}

// attachDoneMsg is sent when the attached tmux client returns (detached, or
// the session ended).
type attachDoneMsg struct {
	name string
	err  error
}

// startedMsg means the tmux session name now exists and can be attached.
// guard is the key the starting guard was set under (the base name a
// suffixed resume retry replaced, or "new:<dir>" for a new session). For a
// new session, asked is the directory picked and dir the one it runs in:
// asked with its symlinks resolved, as Claude records it.
type startedMsg struct {
	name, guard string
	dir, asked  string
}

// startFailedMsg means creating the tmux session failed.
type startFailedMsg struct {
	name, guard string
	err         error
}

// attachCmd hands the terminal to tmux until the user detaches.
func attachCmd(ops sessionOps, name string) tea.Cmd {
	return tea.ExecProcess(ops.AttachCmd(name), func(err error) tea.Msg {
		return attachDoneMsg{name: name, err: err}
	})
}

// claudeArgv is the Claude command plus extra arguments, on a fresh slice so
// the configured command's backing array is never written to.
func claudeArgv(cfg config.Config, extra ...string) []string {
	return append(append([]string{}, cfg.ClaudeCmd...), extra...)
}

// resumeCmd starts a detached tmux session running `claude --resume <id>`.
// If the name is taken (after /clear, the pane sf-<id8> carries a newer
// id), it retries once under the name plus a random suffix rather than
// attach to a different conversation.
func resumeCmd(ops sessionOps, cfg config.Config, s sessions.Session, w, h int) tea.Cmd {
	name, argv := tmux.SessionName(s.ID), claudeArgv(cfg, "--resume", s.ID)
	return func() tea.Msg {
		if fi, err := os.Stat(s.Dir); err != nil || !fi.IsDir() {
			return startFailedMsg{name: name, guard: name, err: errors.New("directory gone: " + s.Dir)}
		}
		err := ops.NewSession(name, s.Dir, s.ID, argv, w, h)
		if err != nil && strings.Contains(err.Error(), "duplicate session") {
			alt := name + "-" + randomHex(2)
			if err = ops.NewSession(alt, s.Dir, s.ID, argv, w, h); err == nil {
				return startedMsg{name: alt, guard: name}
			}
		}
		if err != nil {
			return startFailedMsg{name: name, guard: name, err: err}
		}
		return startedMsg{name: name, guard: name}
	}
}

// randomHex returns n random bytes as hex.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

// startCmd starts a new Claude session with a fresh id in dir.
func startCmd(ops sessionOps, cfg config.Config, dir string, w, h int) tea.Cmd {
	return startWithID(ops, cfg, dir, newUUID(), false, w, h)
}

// startWithID starts a new Claude session with the given id in dir, making
// dir (and any missing parents) first when create is set. Everything that
// touches the disk happens in the command, off the UI goroutine; a failure
// comes back as startFailedMsg, so the starting guard is cleared.
func startWithID(ops sessionOps, cfg config.Config, dir, id string, create bool, w, h int) tea.Cmd {
	name, guard := tmux.SessionName(id), newGuard(dir)
	argv := claudeArgv(cfg, "--session-id", id)
	return func() tea.Msg {
		if create {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return startFailedMsg{name: name, guard: guard, err: fmt.Errorf("create %s: %w", dir, err)}
			}
		}
		real := resolveDir(dir)
		if err := ops.NewSession(name, real, id, argv, w, h); err != nil {
			return startFailedMsg{name: name, guard: guard, err: err}
		}
		return startedMsg{name: name, guard: guard, dir: real, asked: dir}
	}
}

// newGuard is the starting-guard key for a new session in dir: one at a
// time per directory, so a second n before the first is up does nothing.
func newGuard(dir string) string { return "new:" + dir }

// resolveDir is dir with its symlinks resolved (/tmp/x is /private/tmp/x
// on macOS). Claude records its working directory that way, so the new
// session lands in the same project as its history. dir itself if it
// can't be resolved.
func resolveDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// newUUID returns a random RFC 4122 version 4 UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// windowSize is the size given to new tmux sessions.
func (m homeModel) windowSize() (int, int) {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func (m *homeModel) setStatus(kind int, text string) tea.Cmd {
	return m.status.set(kind, text)
}

// refreshCmd starts a discovery now, unless one is already in flight (its
// result will serve). Its result doesn't arm a tick: the refresh loop keeps
// running on its own timer, so a refresh never adds a second loop.
func (m *homeModel) refreshCmd() tea.Cmd {
	if m.inFlight {
		return nil
	}
	m.inFlight = true
	return discoverCmd(m.deps.Disc, m.tickID, false, m.historyDays)
}

// enterKey attaches to or resumes the selected session.
func (m homeModel) enterKey() (homeModel, tea.Cmd) {
	s := m.selected()
	if s == nil {
		return m, nil
	}
	if s.State == sessions.External {
		cmd := m.setStatus(statusError, fmt.Sprintf("running in another terminal (pid %d) — can't attach", s.PID))
		return m, cmd
	}
	if m.ops == nil {
		cmd := m.setStatus(statusError, "tmux unavailable")
		return m, cmd
	}
	if s.State == sessions.Stopped {
		// resumeCmd checks the directory still exists, off the UI goroutine.
		name := tmux.SessionName(s.ID)
		if m.starting[name] {
			cmd := m.setStatus(statusProgress, "starting…")
			return m, cmd
		}
		m.markStarting(name)
		w, h := m.windowSize()
		return m, resumeCmd(m.ops, m.deps.Cfg, *s, w, h)
	}
	name := s.TmuxName
	if name == "" {
		name = tmux.SessionName(s.ID)
	}
	return m, attachCmd(m.ops, name)
}

// markStarting records that the tmux session name is being created, so a
// second enter on it is ignored until startedMsg or startFailedMsg.
func (m *homeModel) markStarting(name string) {
	if m.starting == nil {
		m.starting = map[string]bool{}
	}
	m.starting[name] = true
}

// startIn starts a new Claude session in dir. Only one start per directory
// runs at a time.
func (m homeModel) startIn(dir string) (homeModel, tea.Cmd) {
	return m.startNew(dir, false)
}

// createIn makes dir (and any missing parents) and then starts a new
// session there, as startIn does.
func (m homeModel) createIn(dir string) (homeModel, tea.Cmd) {
	return m.startNew(dir, true)
}

func (m homeModel) startNew(dir string, create bool) (homeModel, tea.Cmd) {
	if m.ops == nil {
		cmd := m.setStatus(statusError, "tmux unavailable")
		return m, cmd
	}
	if m.starting[newGuard(dir)] {
		cmd := m.setStatus(statusProgress, "starting…")
		return m, cmd
	}
	m.markStarting(newGuard(dir))
	w, h := m.windowSize()
	return m, startWithID(m.ops, m.deps.Cfg, dir, newUUID(), create, w, h)
}

// stopKey opens the stop confirmation for the selected session.
func (m homeModel) stopKey() (homeModel, tea.Cmd) {
	s := m.selected()
	if s == nil {
		return m, nil
	}
	switch s.State {
	case sessions.External:
		cmd := m.setStatus(statusError, "running in another terminal — stop it there")
		return m, cmd
	case sessions.Stopped:
		cmd := m.setStatus(statusProgress, "already stopped")
		return m, cmd
	}
	if m.ops == nil {
		cmd := m.setStatus(statusError, "tmux unavailable")
		return m, cmd
	}
	name := s.TmuxName
	if name == "" {
		name = tmux.SessionName(s.ID)
	}
	ops, title := m.ops, s.Title
	m.confirm = &confirmPrompt{
		title:    "stop " + title + "?",
		lines:    []string{"The Claude process is ended and the tmux session closed. You can resume it later."},
		progress: "stopping " + title + "…",
		yes:      "stop",
		run: func() tea.Msg {
			return actionMsg{err: ops.Kill(name), done: "stopped " + title}
		},
	}
	m.layout()
	return m, nil
}
