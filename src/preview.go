package src

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/stardust1420/starfield/internal/sessions"
	"github.com/stardust1420/starfield/internal/tmux"
)

// previewLines is the height of the p panel, borders included, so it shows
// previewLines-2 lines of the pane.
const previewLines = 12

// ansiReset ends every previewed line, so an escape sequence the capture
// cut off can't colour the rest of the UI.
const ansiReset = "\x1b[0m"

// previewMsg is the result of one pane capture for the tmux session name.
type previewMsg struct {
	name    string
	content string
	err     error
}

// previewPanel draws the preview box with the generic title "preview":
// exactly lines tall, each line at most width wide.
func previewPanel(content string, width, lines int) string {
	return previewPanelTitled("preview", content, width, lines)
}

// previewPanelTitled keeps the last lines-2 lines of content (trailing blank
// lines dropped), truncates each to the panel's inner width and boxes them
// with the title in the top border. ANSI styling is kept.
func previewPanelTitled(title, content string, width, lines int) string {
	inner := lines - 2
	var ls []string
	if content != "" {
		ls = strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	}
	for len(ls) > 0 && strings.TrimSpace(ansi.Strip(ls[len(ls)-1])) == "" {
		ls = ls[:len(ls)-1]
	}
	if len(ls) > inner {
		ls = ls[len(ls)-inner:]
	}
	if width > 0 {
		for i, l := range ls {
			ls[i] = ansiTruncate(l, max(1, width-4))
		}
	}
	for i, l := range ls {
		ls[i] = l + ansiReset
	}
	return renderPanelColored(width, title, strings.Join(ls, "\n"), inner, colorBorder, mutedStyle)
}

// tmuxNameOf is the tmux session a managed session runs in.
func tmuxNameOf(s *sessions.Session) string {
	if s.TmuxName != "" {
		return s.TmuxName
	}
	return tmux.SessionName(s.ID)
}

// previewTarget is the tmux name to capture for the selected session, or ""
// when it isn't managed and live.
func (m homeModel) previewTarget() string {
	s := m.selected()
	if s == nil || (s.State != sessions.Running && s.State != sessions.Starting) {
		return ""
	}
	return tmuxNameOf(s)
}

// previewCmd captures the selected session's pane off the UI goroutine, or
// returns nil if the panel is closed or there is nothing live to capture.
func (m homeModel) previewCmd() tea.Cmd {
	name := m.previewTarget()
	if !m.previewOpen || name == "" || m.ops == nil {
		return nil
	}
	ops := m.ops
	return func() tea.Msg {
		out, err := ops.Capture(name, previewLines-2)
		return previewMsg{name: name, content: out, err: err}
	}
}

// applyPreview stores a capture unless the selection has moved on.
func (m *homeModel) applyPreview(msg previewMsg) {
	if !m.previewOpen || msg.name != m.previewTarget() {
		return
	}
	m.previewName, m.previewContent = msg.name, msg.content
	m.previewErr = ""
	if msg.err != nil {
		m.previewErr = msg.err.Error()
	}
}

// previewView is the panel for the selected session: the pane for a live
// one, otherwise its last prompt.
func (m homeModel) previewView(w int) string {
	s := m.selected()
	if s == nil {
		return previewPanelTitled("preview", "", w, previewLines)
	}
	name := m.previewTarget()
	title := "preview · " + tmuxNameOf(s)
	if name == "" {
		return previewPanelTitled("preview · "+s.Title, m.promptBody(s, w), w, previewLines)
	}
	if m.previewErr != "" && m.previewName == name {
		return previewPanelTitled(title, mutedStyle.Render("preview unavailable: "+oneLine(m.previewErr)), w, previewLines)
	}
	content := ""
	if m.previewName == name {
		content = m.previewContent
	}
	return previewPanelTitled(title, content, w, previewLines)
}

// promptBody is the full last prompt, plain and wrapped to the panel.
func (m homeModel) promptBody(s *sessions.Session, w int) string {
	p := strings.TrimSpace(ansi.Strip(s.LastPrompt))
	if p == "" {
		return mutedStyle.Render("no prompt recorded")
	}
	if w > 4 {
		p = ansi.Wrap(p, w-4, "")
	}
	return p
}
