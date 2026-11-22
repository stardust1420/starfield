package src

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The list screens show their items as a grid of cards. Cards are grouped
// into sections by full-width headings (a directory on the home screen); a
// heading can be selectable, but the home screen's are not, so the cursor
// only visits cards.
//
//	▸ starfield   ~/Developer/starfield · 1 running
//	┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓   ╭────────────────────────────╮
//	┃                            ┃   │                            │
//	┃  Home screen with cards    ┃   │  Fix tmux status bar       │
//	┃                            ┃   │                            │
//	┃  ⎇ main · opus-5-5         ┃   │  ⎇ main · opus-5-5         │
//	┃  "add the session grid"    ┃   │  "theme the status bar"    │
//	┃  $10.22 · 133 msgs         ┃   │  $1.80                     │
//	┃  ◐ working · up 2h         ┃   │  ○ 3h ago                  │
//	┃                            ┃   │                            │
//	┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛   ╰────────────────────────────╯
//
// A card with a glyph puts it before the title and indents the body under
// the title; without one, title and body start at the same column.
//
// Body lines are one fact each; builders leave out what doesn't apply
// rather than leaving a blank line in the middle.
//
// The screen keeps a slice of its items parallel to the grid's entries, and
// grid.cursor indexes both.

const (
	gridMinCard   = 36 // narrowest card, border included
	gridGap       = 3  // columns between cards, as on the home screen
	gridBodyLines = 4  // lines under each card's title and the blank below it
	// Spaced like the home cards: border, a blank line of padding, the
	// title, a blank line, the body, a blank line of padding, border.
	gridCardLines = 2 + 2 + 1 + 1 + gridBodyLines
)

// gridEntry is one card or heading. glyph, meta and body are already styled;
// title is plain so it can be coloured for the selection.
type gridEntry struct {
	key        string // identifies the item across refreshes
	heading    bool   // a full-width section heading instead of a card
	selectable bool
	glyph      string
	title      string
	meta       string   // a heading's text after the title
	body       []string // a card's lines under the title
}

type gridView struct {
	entries []gridEntry
	cursor  int // index into entries; always a selectable one, or -1
	offset  int // first visible line
	width   int
	height  int
}

// gridRow is one visual row: a heading, or up to cols cards.
type gridRow struct {
	entries  []int
	heading  bool
	top      int // first line, blank lines above it included
	gapAbove int
	body     int // body lines of its cards: the most any of them has
}

func (r gridRow) height() int {
	if r.heading {
		return 1
	}
	return gridCardLines - gridBodyLines + r.body
}

// bodyLines is how many body lines the cards of a row need, so a row of
// sparse cards isn't padded out to the height of a full one.
func (g gridView) bodyLines(entries []int) int {
	n := 1
	for _, i := range entries {
		n = max(n, min(len(g.entries[i].body), gridBodyLines))
	}
	return n
}

// cols is how many cards fit side by side, and cardWidth how wide each is.
func (g gridView) cols() int {
	if g.width <= 0 {
		return 1
	}
	return max(1, (g.width+gridGap)/(gridMinCard+gridGap))
}

func (g gridView) cardWidth() int {
	if g.width <= 0 {
		return gridMinCard
	}
	n := g.cols()
	return (g.width - gridGap*(n-1)) / n
}

// rows lays the entries out: a heading on its own row, cards filling rows
// of cols. A blank line separates rows, except under a heading.
func (g gridView) rows() []gridRow {
	var rows []gridRow
	line := 0
	add := func(r gridRow) {
		if len(rows) > 0 {
			prev := rows[len(rows)-1]
			if !prev.heading {
				r.gapAbove = 1
			}
			if r.heading {
				r.gapAbove = 1
			}
		}
		r.top = line
		line += r.gapAbove + r.height()
		rows = append(rows, r)
	}
	cols := g.cols()
	var cur []int
	flush := func() {
		if len(cur) > 0 {
			add(gridRow{entries: cur, body: g.bodyLines(cur)})
			cur = nil
		}
	}
	for i, e := range g.entries {
		if e.heading {
			flush()
			add(gridRow{entries: []int{i}, heading: true})
			continue
		}
		cur = append(cur, i)
		if len(cur) == cols {
			flush()
		}
	}
	flush()
	return rows
}

// setEntries replaces the entries, keeping the cursor on the entry with the
// same key if it is still there, or else near where it was.
func (g *gridView) setEntries(entries []gridEntry) {
	prev := ""
	if g.cursor >= 0 && g.cursor < len(g.entries) {
		prev = g.entries[g.cursor].key
	}
	old := g.cursor
	g.entries = entries
	g.cursor = -1
	for i, e := range entries {
		if prev != "" && e.key == prev && e.selectable {
			g.cursor = i
			break
		}
	}
	if g.cursor < 0 {
		g.cursor = g.nearestSelectable(min(max(old, 0), len(entries)-1))
	}
	g.scroll()
}

// selectKey moves the cursor to the selectable entry with key, if there
// is one.
func (g *gridView) selectKey(key string) {
	for i, e := range g.entries {
		if e.key == key && e.selectable {
			g.cursor = i
			g.scroll()
			return
		}
	}
}

// nearestSelectable is i if selectable, else the next selectable entry
// after it, else the one before it, else -1.
func (g gridView) nearestSelectable(i int) int {
	if i < 0 {
		return -1
	}
	for j := i; j < len(g.entries); j++ {
		if g.entries[j].selectable {
			return j
		}
	}
	for j := i - 1; j >= 0; j-- {
		if g.entries[j].selectable {
			return j
		}
	}
	return -1
}

func (g *gridView) resize(width, height int) {
	g.width, g.height = width, height
	g.scroll()
}

// handleKey moves the selection. It reports whether the key was a
// movement key.
func (g *gridView) handleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "right", "l", "tab":
		g.step(1)
	case "left", "h", "shift+tab":
		g.step(-1)
	case "down", "j":
		g.vertical(1)
	case "up", "k":
		g.vertical(-1)
	case "pgdown", "ctrl+d":
		for range max(1, g.height/(gridCardLines+1)) {
			g.vertical(1)
		}
	case "pgup", "ctrl+u":
		for range max(1, g.height/(gridCardLines+1)) {
			g.vertical(-1)
		}
	case "home", "g":
		g.cursor = g.nearestSelectable(0)
	case "end", "G":
		for j := len(g.entries) - 1; j >= 0; j-- {
			if g.entries[j].selectable {
				g.cursor = j
				break
			}
		}
	default:
		return false
	}
	g.scroll()
	return true
}

// step moves to the next or previous selectable entry in reading order.
func (g *gridView) step(dir int) {
	for j := g.cursor + dir; j >= 0 && j < len(g.entries); j += dir {
		if g.entries[j].selectable {
			g.cursor = j
			return
		}
	}
}

// vertical moves to the row above or below, keeping the column where it
// can (the last card of a shorter row otherwise). Rows with nothing
// selectable are skipped.
func (g *gridView) vertical(dir int) {
	rows := g.rows()
	r, col := g.position(rows)
	if r < 0 {
		return
	}
	for nr := r + dir; nr >= 0 && nr < len(rows); nr += dir {
		row := rows[nr]
		pick := -1
		for c := min(col, len(row.entries)-1); c >= 0; c-- {
			if g.entries[row.entries[c]].selectable {
				pick = row.entries[c]
				break
			}
		}
		if pick >= 0 {
			g.cursor = pick
			return
		}
	}
}

// position is the row and column of the cursor, or -1, -1.
func (g gridView) position(rows []gridRow) (int, int) {
	for r, row := range rows {
		for c, i := range row.entries {
			if i == g.cursor {
				return r, c
			}
		}
	}
	return -1, -1
}

// scroll moves the view so the selected row is on screen, with its section
// heading too when that is the row just above it.
func (g *gridView) scroll() {
	if g.height <= 0 {
		return
	}
	rows := g.rows()
	total := 0
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		total = last.top + last.gapAbove + last.height()
	}
	if r, _ := g.position(rows); r >= 0 {
		top := rows[r].top + rows[r].gapAbove
		bottom := top + rows[r].height()
		if r > 0 && rows[r-1].heading && rows[r].gapAbove == 0 {
			top = rows[r-1].top + rows[r-1].gapAbove
		}
		if r == 0 {
			top = 0
		}
		if top < g.offset {
			g.offset = top
		}
		if bottom > g.offset+g.height {
			g.offset = bottom - g.height
		}
	}
	g.offset = max(0, min(g.offset, total-g.height))
}

var (
	gridCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)
	gridSelectedCardStyle = gridCardStyle.Border(lipgloss.ThickBorder()).BorderForeground(colorAccent)
	gridTitleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	gridSelectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	gridHeadingStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorText)
)

// view renders the visible part of the grid, exactly g.height lines tall.
// empty is shown when there are no entries.
func (g gridView) view(empty string) string {
	if len(g.entries) == 0 {
		out := []string{"", strings.Repeat(" ", textIndent) + mutedStyle.Render(empty)}
		for len(out) < g.height {
			out = append(out, "")
		}
		return strings.Join(out[:max(g.height, 0)], "\n")
	}

	var lines []string
	cardWidth := g.cardWidth()
	for _, row := range g.rows() {
		for range row.gapAbove {
			lines = append(lines, "")
		}
		if row.heading {
			lines = append(lines, g.renderHeading(row.entries[0]))
			continue
		}
		cards := make([]string, 0, 2*len(row.entries))
		for c, i := range row.entries {
			if c > 0 {
				cards = append(cards, strings.Repeat(" ", gridGap))
			}
			cards = append(cards, g.renderCard(i, cardWidth, row.body))
		}
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, cards...), "\n")...)
	}

	if g.height <= 0 {
		return strings.Join(lines, "\n")
	}
	start := min(g.offset, len(lines))
	lines = lines[start:min(len(lines), start+g.height)]
	for len(lines) < g.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (g gridView) renderHeading(i int) string {
	e := g.entries[i]
	titleStyle := gridHeadingStyle
	if e.selectable {
		titleStyle = gridTitleStyle
	}
	if i == g.cursor {
		titleStyle = gridSelectedStyle
	}
	line := " " + e.glyph
	if e.glyph != "" {
		line += " "
	}
	line += titleStyle.Render(e.title)
	if e.meta != "" {
		line += "   " + e.meta
	}
	if g.width > 0 {
		line = ansiTruncate(line, g.width)
	}
	return line
}

func (g gridView) renderCard(i, width, bodyLines int) string {
	e := g.entries[i]
	style, titleStyle := gridCardStyle, gridTitleStyle
	if i == g.cursor {
		style, titleStyle = gridSelectedCardStyle, gridSelectedStyle
	}
	inner := max(1, width-6) // border + padding

	prefix, indent := "", ""
	if e.glyph != "" {
		prefix, indent = e.glyph+" ", "  "
	}
	title := titleStyle.Render(truncate(e.title, max(1, inner-lipgloss.Width(prefix))))
	lines := []string{prefix + title, ""}
	for b := range bodyLines {
		line := ""
		if b < len(e.body) {
			line = ansiTruncate(e.body[b], max(1, inner-len(indent)))
		}
		lines = append(lines, indent+line)
	}
	return style.Width(width - 2).Render(strings.Join(lines, "\n"))
}

// listGridHeight is the height left for the grid on a list screen: the
// window minus the header, the blank lines around the grid, the help or
// field at the bottom and, while one is open, the confirmation panel.
func listGridHeight(windowHeight, bottomLines int, confirming bool) int {
	return listGridHeightWith(windowHeight, bottomLines, confirming, 0)
}

// listGridHeightWith is listGridHeight with an extra panel of previewLines
// lines (the live preview) under the grid. Any panel takes the place of the
// blank line above the bottom bar.
func listGridHeightWith(windowHeight, bottomLines int, confirming bool, previewLines int) int {
	panels := previewLines
	if confirming {
		panels += detailsLines + 2
	}
	used := 1 + 1 + bottomLines // header, blank, bottom
	if panels > 0 {
		used += panels
	} else {
		used++ // the blank line above the bottom bar
	}
	return max(gridCardLines, windowHeight-used)
}

// renderGridPage stacks a list screen: header, grid, the confirmation panel
// if any, and the bottom bar.
func renderGridPage(header, grid, panel, bottom string) string {
	blocks := []string{pageStyle.Render(header), "", pageStyle.Render(grid)}
	if panel != "" {
		blocks = append(blocks, pageStyle.Render(panel))
	} else {
		blocks = append(blocks, "")
	}
	blocks = append(blocks, pageStyle.Render(bottom))
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

// glyphFor colours a state glyph by the state it stands for.
func glyphFor(g string) string {
	switch g {
	case glyphRunning:
		return runningStyle.Render(g)
	case glyphBusy, glyphStarting:
		return statusAccentStyle.Render(g)
	case glyphExternal:
		return externalStyle.Render(g)
	default:
		return mutedStyle.Render(g)
	}
}
