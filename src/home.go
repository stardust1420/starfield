package src

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/sessions"
	"github.com/stardust1420/starfield/internal/tmux"
)

// Deps is everything the screens need from the outside world.
type Deps struct {
	Cfg  config.Config
	Tmux *tmux.Client
	Disc *sessions.Discoverer
}

// discoveredMsg is the result of one discovery, tagged with the tick id of
// the screen that asked for it. err is the first source that failed; the
// sessions from the others are still there.
type discoveredMsg struct {
	id       int
	sessions []sessions.Session
	err      error
	// fromTick is set when the refresh loop's own tick started the
	// discovery. Only those results arm the next tick; a refresh started by
	// an attach or a stop must not, or every one of them would add a loop.
	fromTick bool
}

// discoverMu serialises Discover calls: a discovery started by a screen
// that has since been replaced (resume gives it a new tick id) may still be
// running when the next one starts, and a Discoverer isn't safe for
// concurrent use.
var discoverMu sync.Mutex

// discoverCmd runs a discovery off the UI goroutine.
func discoverCmd(d *sessions.Discoverer, id int, fromTick bool, days int) tea.Cmd {
	return func() tea.Msg {
		if d == nil {
			return discoveredMsg{id: id, fromTick: fromTick}
		}
		discoverMu.Lock()
		defer discoverMu.Unlock()
		ss, err := d.DiscoverWithin(days)
		return discoveredMsg{id: id, sessions: ss, err: err, fromTick: fromTick}
	}
}

// homeModel is the home screen: every Claude session as a card, grouped by
// directory. It refreshes on a tick loop (one discovery in flight at a
// time) and runs a frame loop for the busy pulse and, while there's nothing
// to list, the starfield.
type homeModel struct {
	deps          Deps
	ops           sessionOps      // nil when tmux is unavailable
	starting      map[string]bool // tmux sessions being created, so a second enter is ignored
	help          help.Model
	keys          homeKeyMap
	width, height int
	home          string // the user's home directory, shown as ~

	grid  gridView
	all   []sessions.Session  // the latest discovery, in display order
	shown []sessions.Session  // all after the a / filter
	items []*sessions.Session // items[i] is what grid entry i shows (nil for a heading)

	loaded   bool // the first discovery has returned
	inFlight bool // a discovery is running
	discErr  string
	tickID   int
	frame    int
	hasBusy  bool // a shown session is working, so its glyph pulses

	status      statusLine
	confirm     *confirmPrompt // pending stop (Task 8), if any
	filter      filterBar
	runningOnly bool
	historyDays int        // window for stopped sessions (t cycles it); 0 is all time
	stars       *starfield // the empty-state sky; nil until the size is known

	previewOpen    bool   // the p panel is shown
	previewName    string // tmux name the preview content was captured from
	previewContent string
	previewErr     string
}

func InitHomeModel(d Deps, width, height int) homeModel {
	home, _ := os.UserHomeDir()
	m := homeModel{
		deps:   d,
		help:   newHelp(),
		keys:   homeKeys,
		width:  width,
		height: height,
		home:   home,
		grid:   gridView{cursor: -1},
		tickID: newTickID(),
		filter: newFilterBar(),
		stars:  newHomeStarfield(width, height),

		historyDays: d.Cfg.HistoryDays,
	}
	if d.Tmux != nil {
		m.ops = d.Tmux
	}
	m.layout()
	return m
}

// newHomeStarfield builds the sky for a window of this size, or nil if the
// size isn't known yet.
func newHomeStarfield(width, height int) *starfield {
	if width <= 0 || height <= 0 {
		return nil
	}
	return newStarfield(width, height, uint64(time.Now().UnixNano()))
}

// Init fires the first discovery right away and starts the frame loop
// (unless motion is reduced). Run it again on a model returned by resume.
func (m homeModel) Init() tea.Cmd {
	id := m.tickID
	cmds := []tea.Cmd{
		tea.SetWindowTitle("starfield"),
		func() tea.Msg { return tickMsg{id: id, t: time.Now()} },
	}
	if !m.deps.Cfg.ReducedMotion {
		cmds = append(cmds, frameCmd(id))
	}
	return tea.Batch(cmds...)
}

// resume prepares the home screen for being returned to (from an attached
// session or the picker) with its cursor, filter and view intact. It gets a
// new tick id so loops left over from before die out; return it with its
// Init() to restart them.
func (m homeModel) resume(width, height int) homeModel {
	m.width, m.height = width, height
	m.tickID = newTickID()
	m.inFlight = false // a discovery from before belongs to the old tick id
	m.confirm = nil
	m.stars = newHomeStarfield(width, height)
	m.layout()
	return m
}

// selected returns the session under the cursor, or nil.
func (m homeModel) selected() *sessions.Session {
	if m.grid.cursor < 0 || m.grid.cursor >= len(m.items) {
		return nil
	}
	return m.items[m.grid.cursor]
}

// rebuild fills the grid from the last discovery with the current view and
// filter. The grid keeps the cursor on the same session across rebuilds.
func (m *homeModel) rebuild() {
	m.shown = filterSessions(withinWindow(m.all, m.historyDays, time.Now()), m.runningOnly, m.filter.query())
	m.hasBusy = false
	for _, s := range m.shown {
		if s.State == sessions.Running && s.Status == "busy" {
			m.hasBusy = true
		}
	}
	entries, items := buildSessionEntries(m.shown, time.Now(), m.home, m.busyGlyph())
	m.items = items
	m.layout()
	m.grid.setEntries(entries)
}

func (m homeModel) busyGlyph() string {
	if m.deps.Cfg.ReducedMotion {
		return glyphBusy
	}
	return busyGlyph(m.frame)
}

// layout fits the grid to the window: full page width, and whatever height
// is left after the header, the help and any confirmation.
func (m *homeModel) layout() {
	panel := 0
	if m.previewOpen {
		panel = previewLines
	}
	m.grid.resize(pageWidth(m.width), listGridHeightWith(m.height, lipgloss.Height(m.bottomLeft()), m.confirm != nil, panel))
}

// bottomLeft is the help, or the filter field while it is being edited.
func (m homeModel) bottomLeft() string {
	if m.filter.editing {
		return m.filter.prompt()
	}
	return m.help.View(m.keys)
}

// emptyPage reports whether the screen shows the starfield instead of the
// grid: before the first discovery, and when there are no sessions at all.
func (m homeModel) emptyPage() bool { return !m.loaded || len(m.all) == 0 }

func (m homeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.stars = newHomeStarfield(msg.Width, msg.Height)
		m.layout()
		return m, nil

	case tickMsg:
		if msg.id != m.tickID {
			return m, nil
		}
		if m.inFlight {
			// A refresh is already running and won't arm a tick, so this
			// tick keeps the loop alive by rescheduling itself.
			return m, scheduleTick(m.tickID)
		}
		m.inFlight = true
		return m, discoverCmd(m.deps.Disc, m.tickID, true, m.historyDays)

	case settingsSavedMsg:
		if msg.err != nil {
			return m, m.setStatus(statusError, "couldn't save settings: "+msg.err.Error())
		}
		return m, nil

	case discoveredMsg:
		if msg.id != m.tickID {
			return m, nil
		}
		m.inFlight = false
		m.loaded = true
		// A failed discovery that found nothing keeps the last list rather
		// than replacing the cards with the empty state.
		if msg.err == nil || len(msg.sessions) > 0 {
			m.all = msg.sessions
		}
		var cmd tea.Cmd
		if msg.err != nil {
			text := "discovery: " + msg.err.Error()
			cmd = m.status.set(m.tickID, statusError, text)
			m.discErr = text
		} else if m.discErr != "" {
			if m.status.text == m.discErr {
				m.status.set(m.tickID, statusSuccess, "")
			}
			m.discErr = ""
		}
		m.rebuild()
		var next tea.Cmd
		if msg.fromTick {
			next = scheduleTick(m.tickID)
		}
		return m, tea.Batch(cmd, next, m.previewCmd())

	case frameMsg:
		if msg.id != m.tickID {
			return m, nil
		}
		m.frame++
		if m.emptyPage() && m.stars != nil {
			m.stars.step()
		}
		if m.hasBusy && m.frame%3 == 0 {
			m.rebuild()
		}
		return m, frameCmd(m.tickID)

	case startedMsg:
		delete(m.starting, msg.name)
		delete(m.starting, msg.guard)
		return m, attachCmd(m.ops, msg.name)

	case startFailedMsg:
		delete(m.starting, msg.name)
		return m, m.setStatus(statusError, msg.err.Error())

	case attachDoneMsg:
		var cmd tea.Cmd
		var ee *exec.ExitError
		if msg.err != nil && !errors.As(msg.err, &ee) {
			cmd = m.setStatus(statusError, "attach: "+msg.err.Error())
		}
		refresh := m.refreshCmd()
		return m, tea.Batch(cmd, refresh)

	case actionMsg:
		var cmd tea.Cmd
		if msg.err != nil {
			cmd = m.setStatus(statusError, msg.err.Error())
		} else {
			cmd = m.setStatus(statusSuccess, msg.done)
		}
		refresh := m.refreshCmd()
		return m, tea.Batch(cmd, refresh)

	case previewMsg:
		m.applyPreview(msg)
		return m, nil

	case clearStatusMsg:
		m.status.clear(msg, m.tickID)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m homeModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filter.editing {
		if m.filter.handle(msg) {
			m.rebuild()
		}
		m.layout()
		return m, nil
	}
	if m.confirm != nil {
		cmd, done := handleConfirmKey(m.confirm, msg)
		if done {
			if cmd != nil {
				m.status.set(m.tickID, statusProgress, m.confirm.progress)
			}
			m.confirm = nil
			m.layout()
		}
		return m, cmd
	}

	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
	case key.Matches(msg, filterKey):
		m.filter.open()
		m.layout()
	case key.Matches(msg, m.keys.RunningOnly):
		m.runningOnly = !m.runningOnly
		m.rebuild()
	case key.Matches(msg, m.keys.History):
		return m.cycleHistory()
	case msg.String() == "esc":
		if m.filter.query() != "" {
			m.filter.clear()
			m.rebuild()
		}
	case msg.String() == "enter":
		return m.enterKey()
	case msg.String() == "x":
		return m.stopKey()
	case key.Matches(msg, m.keys.New):
		return m.newKey()
	case key.Matches(msg, m.keys.Preview):
		m.previewOpen = !m.previewOpen
		m.previewName, m.previewContent, m.previewErr = "", "", ""
		m.layout()
		return m, m.previewCmd()
	default:
		before := m.previewTarget()
		m.grid.handleKey(msg)
		if m.previewOpen && m.previewTarget() != before {
			m.previewName, m.previewContent, m.previewErr = "", "", ""
			return m, m.previewCmd()
		}
	}
	return m, nil
}

func (m homeModel) View() string {
	if m.emptyPage() {
		return m.emptyView()
	}

	w := pageWidth(m.width)
	counts := sessionCounts(withinWindow(m.all, m.historyDays, time.Now()), m.runningOnly) +
		"  " + historyTag(m.historyDays)
	header := renderHeader(w, counts, withFilterCrumb(m.filter)...)

	var empty string
	switch {
	case m.filter.query() != "":
		empty = "No sessions match " + m.filter.crumb()
	case m.runningOnly:
		empty = "Nothing running · a to show all"
	default: // every stopped session is older than the window
		empty = "No sessions " + historyPhrase(m.historyDays) + " · t to look further back"
	}
	panel := ""
	if m.confirm != nil {
		panel = renderConfirmPanel(w, m.confirm)
	}
	if m.previewOpen {
		if panel != "" {
			panel += "\n"
		}
		panel += m.previewView(w)
	}
	bottom := renderBottomBar(w, m.bottomLeft(), m.status.render(), m.status.kind == statusError)
	return renderGridPage(header, m.grid.view(empty), panel, bottom)
}

// emptyView is the starfield with a short message in the middle: while the
// first discovery runs, and when there are no sessions at all.
func (m homeModel) emptyView() string {
	text := mutedStyle.Render("scanning sessions…")
	if m.loaded {
		text = mutedStyle.Render("No Claude sessions yet — press ") + crumbStyle.Render("n") + mutedStyle.Render(" to start one")
		if m.historyDays > 0 {
			text = mutedStyle.Render("No sessions "+historyPhrase(m.historyDays)+" — ") + crumbStyle.Render("t") +
				mutedStyle.Render(" to look further back, ") + crumbStyle.Render("n") + mutedStyle.Render(" to start one")
		}
	}
	lines := []string{brandStyle.Render("✦ starfield"), "", text, ""}
	if st := m.status.render(); st != "" {
		lines = append(lines, st, "")
	}
	lines = append(lines, m.help.View(m.keys))
	page := lipgloss.JoinVertical(lipgloss.Center, lines...)

	if m.width <= 0 || m.height <= 0 {
		return page
	}
	if m.stars != nil {
		if sky := m.stars.compose(page); sky != "" {
			return sky
		}
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, page)
}
