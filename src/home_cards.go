package src

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/stardust1420/starfield/internal/sessions"
)

// Session cards on the home screen: one heading per directory, then a card
// per session in that directory. Body lines, each left out when it doesn't
// apply:
//
//	⎇ main · opus-5-5
//	"<last prompt>"
//	$10.22 · 133 msgs
//	● idle · up 2h        (or ◐ working, ◔ starting…, ◌ external, ○ 3h ago)

var (
	promptStyle  = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	valueStyle   = lipgloss.NewStyle().Foreground(colorText)
	headingGlyph = statusAccentStyle.Render("▸")
	dotSep       = mutedStyle.Render(" · ")
)

// busyFrames is the pulse of a working session's glyph, one step every few
// frame ticks.
var busyFrames = []string{"◐", "◓", "◑", "◒"}

func busyGlyph(frame int) string { return busyFrames[(frame/3)%len(busyFrames)] }

// sessionEntries builds the grid for ss (already in display order) and the
// sessions parallel to it, nil for headings. Directories keep the order of
// their first session.
func sessionEntries(ss []sessions.Session, now time.Time, home string) ([]gridEntry, []*sessions.Session) {
	return buildSessionEntries(ss, now, home, glyphBusy)
}

// buildSessionEntries is sessionEntries with the busy glyph to draw, so the
// home screen can pulse it.
func buildSessionEntries(ss []sessions.Session, now time.Time, home, busy string) ([]gridEntry, []*sessions.Session) {
	var order []string
	groups := map[string][]int{}
	for i, s := range ss {
		if _, ok := groups[s.Dir]; !ok {
			order = append(order, s.Dir)
		}
		groups[s.Dir] = append(groups[s.Dir], i)
	}

	entries := make([]gridEntry, 0, len(ss)+len(order))
	items := make([]*sessions.Session, 0, cap(entries))
	for _, dir := range order {
		running := 0
		for _, i := range groups[dir] {
			if isManagedLive(ss[i].State) {
				running++
			}
		}
		entries = append(entries, dirHeading(dir, home, running))
		items = append(items, nil)
		for _, i := range groups[dir] {
			s := &ss[i]
			entries = append(entries, sessionCard(*s, now, home, busy))
			items = append(items, s)
		}
	}
	return entries, items
}

// isManagedLive reports whether a session is one of ours and alive: these
// count as "running" in headings and the header.
func isManagedLive(st sessions.State) bool {
	return st == sessions.Running || st == sessions.Starting
}

func dirHeading(dir, home string, running int) gridEntry {
	// Dir is kept raw for tmux; sanitize it only for display.
	name := sessions.Sanitize(filepath.Base(dir))
	if dir == "" {
		name = "(unknown directory)"
	}
	meta := mutedStyle.Render(sessions.Sanitize(tildePath(dir, home)))
	if running > 0 {
		meta += dotSep + runningStyle.Render(strconv.Itoa(running)+" running")
	}
	return gridEntry{key: "dir:" + dir, heading: true, glyph: headingGlyph, title: name, meta: meta}
}

// sessionCard is one session's card. busy is the glyph for a working
// session at this frame.
func sessionCard(s sessions.Session, now time.Time, home, busy string) gridEntry {
	var body []string
	if line := branchModelLine(s); line != "" {
		body = append(body, line)
	}
	if p := oneLine(s.LastPrompt); p != "" && p != oneLine(s.Title) {
		body = append(body, promptStyle.Render(`"`+p+`"`))
	}
	if line := costLine(s); line != "" {
		body = append(body, line)
	}
	body = append(body, stateLine(s, now, busy))

	title := oneLine(s.Title)
	if title == "" {
		title = "untitled session"
	}
	return gridEntry{key: "session:" + s.ID, selectable: true, title: title, body: body}
}

func branchModelLine(s sessions.Session) string {
	var parts []string
	if s.Branch != "" {
		parts = append(parts, mutedStyle.Render("⎇ ")+valueStyle.Render(s.Branch))
	}
	if m := shortModel(s.Model); m != "" {
		parts = append(parts, valueStyle.Render(m))
	}
	return strings.Join(parts, dotSep)
}

// shortModel drops a trailing release date: "opus-4-5-20251101" → "opus-4-5".
func shortModel(m string) string {
	if i := strings.LastIndexByte(m, '-'); i >= 0 && len(m)-i-1 == 8 {
		if _, err := strconv.Atoi(m[i+1:]); err == nil {
			return m[:i]
		}
	}
	return m
}

// costLine is "$10.22 · 133 msgs". The message count is left out when it is
// only a lower bound (the history file was read in part).
func costLine(s sessions.Session) string {
	var parts []string
	if s.CostUSD > 0 {
		parts = append(parts, valueStyle.Render(fmt.Sprintf("$%.2f", s.CostUSD)))
	}
	if s.UserMessages > 0 && !s.Truncated {
		parts = append(parts, valueStyle.Render(strconv.Itoa(s.UserMessages))+mutedStyle.Render(plural(s.UserMessages, " msg", " msgs")))
	}
	return strings.Join(parts, dotSep)
}

func stateLine(s sessions.Session, now time.Time, busy string) string {
	switch s.State {
	case sessions.Running:
		head := runningStyle.Render(glyphRunning + " idle")
		if s.Status == "busy" {
			head = statusAccentStyle.Render(busy + " working")
		}
		if s.Started.IsZero() {
			return head
		}
		up := now.Sub(s.Started)
		if up < time.Minute {
			return head + mutedStyle.Render(" · just started")
		}
		return head + mutedStyle.Render(" · up "+relTime(up))
	case sessions.Starting:
		return statusAccentStyle.Render(glyphStarting + " starting…")
	case sessions.External:
		head := externalStyle.Render(glyphExternal + " external")
		if s.PID > 0 {
			return head + mutedStyle.Render(" · pid "+strconv.Itoa(s.PID))
		}
		return head
	default:
		if s.LastActive.IsZero() {
			return mutedStyle.Render(glyphStopped + " stopped")
		}
		ago := relTime(now.Sub(s.LastActive))
		if ago != "just now" {
			ago += " ago"
		}
		return mutedStyle.Render(glyphStopped + " " + ago)
	}
}

// filterSessions keeps the sessions to show: live ones only when
// runningOnly, and those whose title, directory, branch or last prompt
// contains query (already lower-cased).
func filterSessions(ss []sessions.Session, runningOnly bool, query string) []sessions.Session {
	out := make([]sessions.Session, 0, len(ss))
	for _, s := range ss {
		if runningOnly && s.State == sessions.Stopped {
			continue
		}
		if !matches(query, s.Title, s.Dir, s.Branch, s.LastPrompt) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// sessionCounts renders the header's "● 3 running · ◌ 1 external ·
// ○ 24 stopped" followed by the view tag.
func sessionCounts(ss []sessions.Session, runningOnly bool) string {
	var running, external, stopped int
	for _, s := range ss {
		switch {
		case isManagedLive(s.State):
			running++
		case s.State == sessions.External:
			external++
		default:
			stopped++
		}
	}
	count := func(style lipgloss.Style, glyph string, n int, label string) string {
		return style.Render(glyph) + " " + crumbStyle.Render(strconv.Itoa(n)) + mutedStyle.Render(" "+label)
	}
	tag := "[all]"
	if runningOnly {
		tag = "[running]"
	}
	return count(runningStyle, glyphRunning, running, "running") + dotSep +
		count(externalStyle, glyphExternal, external, "external") + dotSep +
		count(mutedStyle, glyphStopped, stopped, "stopped") + "  " + statusAccentStyle.Render(tag)
}

// relTime is a compact duration: "just now", "5m", "3h", "2d", "3w", "4mo",
// "2y". It rounds down.
func relTime(d time.Duration) string {
	const day = 24 * time.Hour
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d < day:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d < 7*day:
		return fmt.Sprintf("%dd", d/day)
	case d < 30*day:
		return fmt.Sprintf("%dw", d/(7*day))
	case d < 365*day:
		return fmt.Sprintf("%dmo", d/(30*day))
	}
	return fmt.Sprintf("%dy", d/(365*day))
}

// tildePath shows dir with the home directory written as ~.
func tildePath(dir, home string) string {
	switch {
	case home == "" || dir == "":
		return dir
	case dir == home:
		return "~"
	case strings.HasPrefix(dir, home+string(filepath.Separator)):
		return "~" + dir[len(home):]
	}
	return dir
}

// oneLine collapses whitespace runs (newlines included) to single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
