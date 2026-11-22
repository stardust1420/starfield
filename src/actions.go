package src

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"

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
}

// attachDoneMsg is sent when the attached tmux client returns (detached, or
// the session ended).
type attachDoneMsg struct {
	name string
	err  error
}

// startedMsg means the tmux session now exists and can be attached.
type startedMsg struct{ name string }

// startFailedMsg means creating the tmux session failed.
type startFailedMsg struct{ err error }

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

func newSessionCmd(ops sessionOps, name, dir, id string, argv []string, w, h int) tea.Cmd {
	return func() tea.Msg {
		if err := ops.NewSession(name, dir, id, argv, w, h); err != nil {
			return startFailedMsg{err: err}
		}
		return startedMsg{name: name}
	}
}

// resumeCmd starts a detached tmux session running `claude --resume <id>`.
func resumeCmd(ops sessionOps, cfg config.Config, s sessions.Session, w, h int) tea.Cmd {
	return newSessionCmd(ops, tmux.SessionName(s.ID), s.Dir, s.ID, claudeArgv(cfg, "--resume", s.ID), w, h)
}

// startCmd starts a new Claude session with a fresh id in dir.
func startCmd(ops sessionOps, cfg config.Config, dir string, w, h int) tea.Cmd {
	id := newUUID()
	return newSessionCmd(ops, tmux.SessionName(id), dir, id, claudeArgv(cfg, "--session-id", id), w, h)
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
	return m.status.set(m.tickID, kind, text)
}

// refreshCmd starts a discovery now.
func (m *homeModel) refreshCmd() tea.Cmd {
	m.inFlight = true
	return discoverCmd(m.deps.Disc, m.tickID)
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
		if fi, err := os.Stat(s.Dir); err != nil || !fi.IsDir() {
			cmd := m.setStatus(statusError, "directory gone: "+s.Dir)
			return m, cmd
		}
		w, h := m.windowSize()
		return m, resumeCmd(m.ops, m.deps.Cfg, *s, w, h)
	}
	name := s.TmuxName
	if name == "" {
		name = tmux.SessionName(s.ID)
	}
	return m, attachCmd(m.ops, name)
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
