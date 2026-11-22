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

// The home screen opens on projects: one card per directory that has
// sessions, under the STARFIELD banner. enter opens a project's sessions.
//
//	alpha
//
//	  ~/Developer/alpha
//	  ⎇ main
//	  ● 1 running · ○ 4 stopped
//	  $12.40 · 3h ago

// project is one directory's sessions, summed up for its card.
type project struct {
	Dir        string
	Running    int // managed and alive (running or starting)
	External   int
	Stopped    int
	Busy       bool      // a managed session is working
	Branch     string    // the most recently active session's branch
	CostUSD    float64   // all its sessions together
	LastActive time.Time // the latest of its sessions
}

// groupProjects makes one project per directory, in the order each
// directory first appears in ss (Merge's order: live sessions first, then
// most recent).
func groupProjects(ss []sessions.Session) []project {
	index := map[string]int{}
	var out []project
	for _, s := range ss {
		i, ok := index[s.Dir]
		if !ok {
			i = len(out)
			index[s.Dir] = i
			out = append(out, project{Dir: s.Dir})
		}
		p := &out[i]
		switch {
		case isManagedLive(s.State):
			p.Running++
			p.Busy = p.Busy || (s.State == sessions.Running && s.Status == "busy")
		case s.State == sessions.External:
			p.External++
		default:
			p.Stopped++
		}
		p.CostUSD += s.CostUSD
		if s.LastActive.After(p.LastActive) {
			p.LastActive = s.LastActive
			if s.Branch != "" {
				p.Branch = s.Branch
			}
		}
		if p.Branch == "" {
			p.Branch = s.Branch
		}
	}
	return out
}

// filterProjects keeps projects with a live session when runningOnly, and
// those whose name or path contains query (already lower-cased).
func filterProjects(ps []project, runningOnly bool, query string) []project {
	out := make([]project, 0, len(ps))
	for _, p := range ps {
		if runningOnly && p.Running+p.External == 0 {
			continue
		}
		if !matches(query, filepath.Base(p.Dir), p.Dir) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// sessionsIn keeps the sessions of one directory.
func sessionsIn(ss []sessions.Session, dir string) []sessions.Session {
	out := make([]sessions.Session, 0, len(ss))
	for _, s := range ss {
		if s.Dir == dir {
			out = append(out, s)
		}
	}
	return out
}

// projectName is a directory's card title.
func projectName(dir string) string {
	if dir == "" {
		return "(unknown directory)"
	}
	return sessions.Sanitize(filepath.Base(dir)) // Dir is raw; sanitize to draw it
}

func projectCard(p project, now time.Time, home string) gridEntry {
	body := []string{mutedStyle.Render(sessions.Sanitize(tildePath(p.Dir, home)))}
	if p.Branch != "" {
		body = append(body, mutedStyle.Render("⎇ ")+valueStyle.Render(sessions.Sanitize(p.Branch)))
	}

	var counts []string
	if p.Running > 0 {
		glyph := glyphRunning
		if p.Busy {
			glyph = glyphBusy
		}
		counts = append(counts, runningStyle.Render(glyph+" "+strconv.Itoa(p.Running)+" running"))
	}
	if p.External > 0 {
		counts = append(counts, externalStyle.Render(glyphExternal+" "+strconv.Itoa(p.External)+" external"))
	}
	if p.Stopped > 0 {
		counts = append(counts, mutedStyle.Render(glyphStopped+" "+strconv.Itoa(p.Stopped)+" stopped"))
	}
	if len(counts) > 0 {
		body = append(body, strings.Join(counts, dotSep))
	}

	var tail []string
	if p.CostUSD > 0 {
		tail = append(tail, valueStyle.Render(fmt.Sprintf("$%.2f", p.CostUSD)))
	}
	if !p.LastActive.IsZero() {
		ago := relTime(now.Sub(p.LastActive))
		if ago != "just now" {
			ago += " ago"
		}
		tail = append(tail, mutedStyle.Render(ago))
	}
	if len(tail) > 0 {
		body = append(body, strings.Join(tail, dotSep))
	}
	return gridEntry{key: "project:" + p.Dir, selectable: true, title: projectName(p.Dir), body: body}
}

// projectEntries builds the projects grid and the projects parallel to it.
func projectEntries(ps []project, now time.Time, home string) ([]gridEntry, []*project) {
	entries := make([]gridEntry, len(ps))
	items := make([]*project, len(ps))
	for i := range ps {
		entries[i] = projectCard(ps[i], now, home)
		items[i] = &ps[i]
	}
	return entries, items
}

// The banner: STARFIELD in block letters (ANSI Shadow), built letter by
// letter so every row lines up.
var bannerGlyphs = map[rune][6]string{
	'S': {"███████╗", "██╔════╝", "███████╗", "╚════██║", "███████║", "╚══════╝"},
	'T': {"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	'A': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'R': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	'F': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "██║     ", "╚═╝     "},
	'I': {"██╗", "██║", "██║", "██║", "██║", "╚═╝"},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'L': {"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
	'D': {"██████╗ ", "██╔══██╗", "██║  ██║", "██║  ██║", "██████╔╝", "╚═════╝ "},
}

// bannerLines is the plain banner, six rows.
func bannerLines() []string {
	var rows [6]strings.Builder
	for _, r := range "STARFIELD" {
		g := bannerGlyphs[r]
		for i := range rows {
			rows[i].WriteString(g[i])
		}
	}
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].String()
	}
	return out
}

// bannerShades run top to bottom from pale peach to Claude coral, the
// shadow row darkest.
var bannerShades = []lipgloss.Color{"#f6c7a8", "#f0ab88", "#e8916d", "#d97757", "#c4643f", "#8f4a30"}

// bannerHeight is the lines the banner takes on the page: six rows, the
// tagline and a blank line under it, above the header line.
const bannerHeight = 8

// bannerMinHeight is the shortest window that still fits the banner and a
// row of cards.
const bannerMinHeight = 30

// renderBanner is the coloured banner and tagline centred in width.
func renderBanner(width int) string {
	lines := bannerLines()
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().Foreground(bannerShades[i]).Bold(true).Render(l)
	}
	tag := mutedStyle.Render("✦  claude code sessions, one place  ✦")
	block := lipgloss.JoinVertical(lipgloss.Center, append(lines, tag)...)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block) + "\n\n"
}

// showBanner reports whether the window has room for the banner.
func showBanner(width, height int) bool {
	return height >= bannerMinHeight && width >= lipgloss.Width(bannerLines()[0])+4
}
