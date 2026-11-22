package src

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/stardust1420/starfield/internal/claude"
	"github.com/stardust1420/starfield/internal/dirs"
	"github.com/stardust1420/starfield/internal/sessions"
)

// The new-session picker: a command-palette style panel over the recent
// Claude dirs and the repos under the configured roots, fuzzy-filtered as
// you type. A query that looks like a path (/, ~ or .) also offers that
// path itself, pinned at the top.
//
//	✦ starfield › new session
//
//	        ╭─ new session ──────────────────────────────────────────╮
//	        │ ❯ sta▏                                           3/41 │
//	        │ ──────────────────────────────────────────────────────  │
//	        │ ▌ ◷ recent  ~/Developer/starfield                   2h │
//	        │   ⌂ repo    ~/Developer/other/stardocker               │
//	        ╰──────────────────────────────────────────────────────╯
//	          enter start · tab complete · ↑↓ select · esc back

// scanDepth is how deep the picker looks under each root for repos.
const scanDepth = 2

var (
	colorAccentMuted = lipgloss.Color("#a8634a") // dimmed coral: recent badge
	colorBright      = lipgloss.Color("255")     // the selected row's text
	colorRowText     = lipgloss.Color("250")

	pickerHintStyle  = mutedStyle
	pickerErrorStyle = lipgloss.NewStyle().Foreground(colorDanger)
)

// scannedMsg carries the full candidate list (recent + scanned roots),
// built off the UI goroutine. id is the picker that asked for it.
type scannedMsg struct {
	id    int
	cands []dirs.Candidate
}

// pickerRow is one result: a candidate, its path as shown (~ for home),
// the byte offsets of the query's matched characters in that string, and,
// for a typed path or a new name, whether it exists but isn't a directory
// (bad) or doesn't exist yet and enter will create it (create).
type pickerRow struct {
	cand   dirs.Candidate
	disp   string
	pos    []int
	bad    bool
	create bool
}

type pickerModel struct {
	back          homeModel // returned on esc, and started from on enter
	id            int       // tags this picker's scan
	cands         []dirs.Candidate
	field         textField
	rows          []pickerRow
	cursor        int
	offset        int // first row shown
	width, height int
	home, cwd     string
	err           string // inline error from the last enter
	now           func() time.Time
}

func initPickerModel(back homeModel, cands []dirs.Candidate, width, height int) pickerModel {
	cwd, _ := os.Getwd()
	m := pickerModel{
		back:   back,
		id:     newTickID(),
		cands:  cands,
		field:  textField{limit: 256},
		width:  width,
		height: height,
		home:   back.home,
		cwd:    cwd,
		now:    time.Now,
	}
	m.refilter()
	return m
}

// historyOf turns the home screen's sessions into the history dirs.Candidates
// ranks recent directories by.
func historyOf(ss []sessions.Session) []claude.HistorySession {
	out := make([]claude.HistorySession, 0, len(ss))
	for _, s := range ss {
		if s.Dir == "" {
			continue
		}
		out = append(out, claude.HistorySession{Dir: s.Dir, LastActive: s.LastActive})
	}
	return out
}

// scanCmd builds the full candidate list (it walks the roots) off the UI
// goroutine.
func scanCmd(id int, hist []claude.HistorySession, roots []string) tea.Cmd {
	return func() tea.Msg {
		return scannedMsg{id: id, cands: dirs.Candidates(hist, roots, scanDepth)}
	}
}

// newKey opens the picker with the recent dirs right away and scans the
// roots in the background.
func (m homeModel) newKey() (tea.Model, tea.Cmd) {
	hist := historyOf(m.all)
	p := initPickerModel(m, dirs.Candidates(hist, nil, scanDepth), m.width, m.height)
	return p, scanCmd(p.id, hist, m.deps.Cfg.Roots)
}

func (m pickerModel) Init() tea.Cmd { return nil }

// isPathQuery reports whether the query should be offered as a path.
func isPathQuery(q string) bool {
	return strings.HasPrefix(q, "/") || strings.HasPrefix(q, "~") || strings.HasPrefix(q, ".")
}

// typedPath resolves a path query: ~ expanded, relative paths against the
// directory starfield was launched from, cleaned.
func (m pickerModel) typedPath(q string) string {
	p := dirs.Expand(q, m.home)
	if !filepath.IsAbs(p) {
		p = filepath.Join(m.cwd, p)
	}
	return filepath.Clean(p)
}

// refilter rebuilds the rows from the query, keeping the selection on the
// same path if it is still listed (else the top).
func (m *pickerModel) refilter() {
	keep := ""
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		keep = m.rows[m.cursor].cand.Path
	}
	m.rows = m.buildRows()
	m.cursor = 0
	for i, r := range m.rows {
		if r.cand.Path == keep {
			m.cursor = i
			break
		}
	}
	m.scroll()
}

func (m pickerModel) buildRows() []pickerRow {
	q := strings.TrimSpace(m.field.value)
	var rows []pickerRow
	typed := ""
	if isPathQuery(q) {
		typed = m.typedPath(q)
		rows = append(rows, m.pathRow(typed))
	}
	disp := make([]string, len(m.cands))
	for i, c := range m.cands {
		disp[i] = tildePath(c.Path, m.home)
	}
	for _, mt := range dirs.Fuzzy(q, disp) {
		c := m.cands[mt.Index]
		if c.Path == typed {
			continue
		}
		rows = append(rows, pickerRow{cand: c, disp: disp[mt.Index], pos: mt.Positions})
	}
	// A plain name is also offered as a directory under the first root,
	// after the matches, so `myproj` + enter makes ~/Developer/myproj.
	if !isPathQuery(q) && isDirName(q) {
		p := filepath.Join(m.newRoot(), q)
		listed := false
		for _, r := range rows {
			listed = listed || r.cand.Path == p
		}
		if r := m.pathRow(p); !listed && !r.bad {
			rows = append(rows, r)
		}
	}
	return rows
}

// pathRow is the row for a path the user named rather than picked: an
// existing directory, one enter will create (it doesn't exist yet), or bad
// (it exists but isn't a directory, or can't be checked).
func (m pickerModel) pathRow(p string) pickerRow {
	r := pickerRow{cand: dirs.Candidate{Path: p, Source: dirs.Typed}, disp: tildePath(p, m.home)}
	fi, err := os.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.create = true
	case err != nil || !fi.IsDir():
		r.bad = true
	}
	return r
}

// isDirName reports whether q can be used as one new directory name.
func isDirName(q string) bool {
	return q != "" && q != "." && q != ".." && !strings.ContainsAny(q, "/\x00")
}

// newRoot is where a plain name is created: the first configured root, or
// the directory starfield was launched from when there is none.
func (m pickerModel) newRoot() string {
	if roots := m.back.deps.Cfg.Roots; len(roots) > 0 {
		return roots[0]
	}
	return m.cwd
}

// listRows is how many result rows fit: height-8, at least one.
func (m pickerModel) listRows() int { return max(1, m.height-8) }

// scroll keeps the cursor inside the shown rows.
func (m *pickerModel) scroll() {
	n := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+n {
		m.offset = m.cursor - n + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-n))
}

func (m *pickerModel) move(d int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = max(0, min(len(m.rows)-1, m.cursor+d))
	m.scroll()
}

// leave returns to the home screen with its loops restarted.
func (m pickerModel) leave() (tea.Model, tea.Cmd) {
	h := m.back.resume(m.width, m.height)
	return h, h.Init()
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
		return m, nil
	case scannedMsg:
		if msg.id == m.id {
			m.cands = msg.cands
			m.refilter()
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case startedMsg:
		// A session the home screen was starting is ready: go back so the
		// attach happens.
		h := m.back.resume(m.width, m.height)
		next, cmd := h.Update(msg)
		return next, tea.Batch(next.(homeModel).Init(), cmd)
	case startFailedMsg, attachDoneMsg, actionMsg, clearStatusMsg:
		// Results of the home screen's own commands: keep its state current.
		next, cmd := m.back.Update(msg)
		m.back = next.(homeModel)
		return m, cmd
	}
	return m, nil
}

func (m pickerModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.leave()
	case "up", "ctrl+p":
		m.move(-1)
		return m, nil
	case "down", "ctrl+n":
		m.move(1)
		return m, nil
	case "pgup":
		m.move(-m.listRows())
		return m, nil
	case "pgdown":
		m.move(m.listRows())
		return m, nil
	case "tab":
		if r := m.selected(); r != nil {
			m.field.value = r.disp
			m.err = ""
			m.refilter()
		}
		return m, nil
	case "enter":
		return m.start()
	}
	before := m.field.value
	if m.field.update(msg) && m.field.value != before {
		m.err = ""
		m.cursor = 0
		m.rows = nil // a new query starts at the best match
		m.refilter()
	}
	return m, nil
}

func (m pickerModel) selected() *pickerRow {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

// start hands control back to the home screen, which creates the session
// and attaches once it exists (startedMsg).
func (m pickerModel) start() (tea.Model, tea.Cmd) {
	r := m.selected()
	if r == nil {
		return m, nil
	}
	if r.bad {
		m.err = "✗ not a directory: " + r.disp
		return m, nil
	}
	// Land on the new session's project, so detaching comes back to it.
	h := m.back.resume(m.width, m.height).openProject(r.cand.Path)
	var cmd tea.Cmd
	if r.create {
		h, cmd = h.createIn(r.cand.Path)
	} else {
		h, cmd = h.startIn(r.cand.Path)
	}
	return h, tea.Batch(h.Init(), cmd)
}

// panelWidth is min(90, window-4), at least 40.
func (m pickerModel) panelWidth() int {
	return max(40, min(90, m.width-4))
}

func (m pickerModel) View() string {
	w := m.panelWidth()
	inner := w - 4 // border and padding

	// Prompt: ❯ query▏ with the match count on the right. A query too long
	// for the line shows its end (where the cursor is), not a wrapped line.
	count := mutedStyle.Render(fmt.Sprintf("%d/%d", len(m.rows), len(m.cands)))
	room := max(1, inner-lipgloss.Width(count)-1-lipgloss.Width("❯ ▏"))
	query := m.field.value
	if lipgloss.Width(query) > room {
		query = "…" + ansi.TruncateLeft(query, lipgloss.Width(query)-room+1, "")
	}
	prompt := brandStyle.Render("❯ ") + crumbStyle.Render(query) + statusAccentStyle.Render("▏")
	gap := max(1, inner-lipgloss.Width(prompt)-lipgloss.Width(count))
	lines := []string{prompt + strings.Repeat(" ", gap) + count, borderStyle.Render(strings.Repeat("─", inner))}

	n := min(m.listRows(), max(1, len(m.rows)))
	if len(m.rows) == 0 {
		lines = append(lines, "  "+mutedStyle.Render("no matches — type a name to create it, or a path starting with / or ~"))
	}
	for i := m.offset; i < len(m.rows) && i < m.offset+n; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor, inner))
	}
	panel := renderPanelColored(w, "new session", strings.Join(lines, "\n"), 2+n, colorAccent, brandStyle)

	enter := "enter start"
	if r := m.selected(); r != nil && r.create {
		enter = "enter create & start"
	}
	hint := pickerHintStyle.Render(enter + " · tab complete · ↑↓ select · esc back")
	if m.err != "" {
		hint = pickerErrorStyle.Render(m.err)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, panel, strings.Repeat(" ", textIndent)+hint)
	if m.width <= 0 || m.height <= 0 {
		return body
	}
	header := renderHeader(pageWidth(m.width), "", "new session")

	// Keep the panel's top still while typing shrinks it: centre the
	// tallest it can be for the current candidates, not its current height.
	tallest := 2 + min(m.listRows(), max(1, len(m.cands)+1)) + 2 + 1
	top := max(1, (m.height-1-tallest)/2)
	page := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, body)
	return header + strings.Repeat("\n", top+1) + page
}

// renderRow draws one result: selection bar, source badge, the path with
// matched characters in coral, and a relative time (recent) or the typed
// path's error, right-aligned.
func (m pickerModel) renderRow(r pickerRow, sel bool, width int) string {
	bar := "  "
	text := lipgloss.NewStyle().Foreground(colorRowText)
	if sel {
		bar = statusAccentStyle.Render("▌") + " "
		text = lipgloss.NewStyle().Foreground(colorBright).Bold(true)
	}

	var badge string
	switch r.cand.Source {
	case dirs.Recent:
		badge = lipgloss.NewStyle().Foreground(colorAccentMuted).Render("◷ recent")
	case dirs.Scanned:
		badge = externalStyle.Render("⌂ repo  ")
	default:
		badge = runningStyle.Render("↳ path  ")
	}
	if r.create {
		badge = statusAccentStyle.Render("+ new   ")
	}

	right := ""
	switch {
	case r.create:
		right = mutedStyle.Render("will be created")
	case r.bad:
		right = pickerErrorStyle.Render("✗ not a directory")
	case r.cand.Source == dirs.Recent && !r.cand.LastUsed.IsZero():
		right = mutedStyle.Render(relTime(m.now().Sub(r.cand.LastUsed)))
	}

	left := bar + badge + "  "
	room := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	path := ansiTruncate(highlight(r.disp, r.pos, text), room)
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(path)-lipgloss.Width(right))
	return left + path + strings.Repeat(" ", gap) + right
}

// highlight renders s in base with the bytes at pos (rune starts) in bold
// coral, grouping runs so a row isn't one escape sequence per character.
func highlight(s string, pos []int, base lipgloss.Style) string {
	if len(pos) == 0 {
		return base.Render(sessions.Sanitize(s))
	}
	hit := make(map[int]bool, len(pos))
	for _, p := range pos {
		hit[p] = true
	}
	match := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	var b strings.Builder
	run, runHit := "", false
	flush := func() {
		if run == "" {
			return
		}
		// Paths are untrusted text: no escapes or control bytes on screen.
		run = sessions.Sanitize(run)
		if runHit {
			b.WriteString(match.Render(run))
		} else {
			b.WriteString(base.Render(run))
		}
		run = ""
	}
	for off, r := range s {
		if hit[off] != runHit {
			flush()
			runHit = hit[off]
		}
		run += string(r)
	}
	flush()
	return b.String()
}
