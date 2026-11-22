package src

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/stardust1420/starfield/internal/sessions"
	"github.com/stardust1420/starfield/internal/tmux"
)

func TestPreviewPanelExactHeight(t *testing.T) {
	long := strings.Repeat("wide 界 line ", 40)
	many := make([]string, 50)
	for i := range many {
		many[i] = fmt.Sprintf("line %d \x1b[31mred\x1b[0m", i)
	}
	cases := map[string]string{
		"empty":        "",
		"short":        "hello",
		"one long":     long,
		"many":         strings.Join(many, "\n"),
		"blank tail":   "a\nb\n\n   \n\n",
		"unterminated": "\x1b[38;5;196mred forever\nnext",
		"wide runes":   strings.Repeat("界", 100),
	}
	for name, c := range cases {
		for _, w := range []int{80, 30, 8} {
			out := previewPanel(c, w, 12)
			if h := lipgloss.Height(out); h != 12 {
				t.Errorf("%s w=%d: height %d, want 12", name, w, h)
			}
			for i, l := range strings.Split(out, "\n") {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("%s w=%d: line %d width %d > %d", name, w, i, lw, w)
				}
			}
		}
	}
}

func TestPreviewPanelKeepsLastLinesAndResets(t *testing.T) {
	var b []string
	for i := 0; i < 30; i++ {
		b = append(b, fmt.Sprintf("row%02d", i))
	}
	out := previewPanel(strings.Join(b, "\n")+"\n\n\n", 60, 12)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "row29") || strings.Contains(plain, "row19") || !strings.Contains(plain, "row20") {
		t.Errorf("want rows 20-29:\n%s", plain)
	}
	out = previewPanel("\x1b[31mred", 60, 12)
	if !strings.Contains(out, "red\x1b[0m") {
		t.Errorf("no reset after unterminated line: %q", out)
	}
}

func TestPreviewPromptPanel(t *testing.T) {
	m := homeWith(t, &fakeOps{}, sessions.Session{ID: testID, Dir: "/x", State: sessions.Stopped, LastPrompt: "fix the \x1b[31mbug\x1b[0m " + strings.Repeat("word ", 40)})
	m, cmd := press(m, "p")
	if cmd != nil {
		t.Errorf("stopped session must not capture")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "fix the bug") || strings.Contains(v, "\x1b") {
		t.Errorf("prompt not shown: %s", v)
	}
	m = homeWith(t, &fakeOps{}, sessions.Session{ID: testID, Dir: "/x", State: sessions.External})
	m, _ = press(m, "p")
	if !strings.Contains(ansi.Strip(m.View()), "no prompt recorded") {
		t.Errorf("missing placeholder")
	}
}

func TestPreviewToggleShrinksGrid(t *testing.T) {
	m := homeWith(t, &fakeOps{}, sessions.Session{ID: testID, Dir: "/x", State: sessions.Stopped})
	before := m.grid.height
	m, _ = press(m, "p")
	// The panel takes the place of the blank line above the bottom bar.
	if m.grid.height != before-previewLines+1 {
		t.Errorf("grid height %d -> %d", before, m.grid.height)
	}
	if got := lipgloss.Height(m.View()); got != m.height {
		t.Errorf("page %d lines, window %d", got, m.height)
	}
	m, _ = press(m, "p")
	if m.grid.height != before {
		t.Errorf("grid height not restored: %d vs %d", m.grid.height, before)
	}
}

func liveSession() sessions.Session {
	return sessions.Session{ID: testID, Dir: "/x", State: sessions.Running, TmuxName: tmux.SessionName(testID)}
}

func TestPreviewCaptureFlow(t *testing.T) {
	f := &fakeOps{capture: "hello from pane"}
	m := homeWith(t, f, liveSession())
	m, cmd := press(m, "p")
	if cmd == nil {
		t.Fatal("opening the panel on a live session must capture")
	}
	msg, ok := cmd().(previewMsg)
	if !ok || msg.content != "hello from pane" || msg.name != tmux.SessionName(testID) {
		t.Fatalf("bad msg %#v", msg)
	}
	if f.captureLines != previewLines-2 {
		t.Errorf("captured %d lines", f.captureLines)
	}
	next, _ := m.Update(msg)
	m = next.(homeModel)
	if !strings.Contains(ansi.Strip(m.View()), "hello from pane") ||
		!strings.Contains(ansi.Strip(m.View()), "preview · "+tmux.SessionName(testID)) {
		t.Errorf("content/title missing:\n%s", ansi.Strip(m.View()))
	}

	// A message for another session is dropped.
	next, _ = m.Update(previewMsg{name: "sf-other", content: "STALE"})
	if strings.Contains(ansi.Strip(next.(homeModel).View()), "STALE") {
		t.Errorf("stale message shown")
	}

	// A capture error shows inside the panel.
	next, _ = m.Update(previewMsg{name: tmux.SessionName(testID), err: errors.New("boom")})
	v := ansi.Strip(next.(homeModel).View())
	if !strings.Contains(v, "preview unavailable: boom") {
		t.Errorf("error not in panel:\n%s", v)
	}

	// A tick's discovery also recaptures while open; closed, it does not.
	_, cmd = m.Update(discoveredMsg{id: m.tickID, sessions: []sessions.Session{liveSession()}})
	if cmd == nil {
		t.Fatal("no cmd after discovery")
	}
	m, _ = press(m, "p")
	if m.previewOpen {
		t.Fatal("still open")
	}
	if c := m.previewCmd(); c != nil {
		t.Errorf("closed panel must not capture")
	}
}
