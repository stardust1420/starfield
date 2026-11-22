package src

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/sessions"
)

// historySteps are the windows t cycles through for stopped sessions, in
// days; 0 is all time, after which it wraps to the shortest.
var historySteps = []int{1, 3, 7, 14, 30, 90, 0}

// nextHistoryWindow is the step after days. A value that isn't a step (say
// STARFIELD_HISTORY_DAYS=4) moves to the next larger one.
func nextHistoryWindow(days int) int {
	if days == 0 {
		return historySteps[0]
	}
	for _, s := range historySteps {
		if s == 0 || s > days {
			return s
		}
	}
	return 0
}

// withinWindow drops stopped sessions last active more than days ago (0
// keeps them all). Running, starting and external sessions are always kept.
// Discovery already applies the window; this makes a narrower one take
// effect before the refresh lands.
func withinWindow(ss []sessions.Session, days int, now time.Time) []sessions.Session {
	if days == 0 {
		return ss
	}
	since := now.AddDate(0, 0, -days)
	out := make([]sessions.Session, 0, len(ss))
	for _, s := range ss {
		if s.State == sessions.Stopped && s.LastActive.Before(since) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// historyTag is the header's "◷ 14d" or "◷ all time".
func historyTag(days int) string {
	label := "all time"
	if days > 0 {
		label = fmt.Sprintf("%dd", days)
	}
	return mutedStyle.Render("◷ ") + crumbStyle.Render(label)
}

// historyPhrase is the window in words, for the status line.
func historyPhrase(days int) string {
	switch days {
	case 0:
		return "from all time"
	case 1:
		return "from the last 1 day"
	}
	return fmt.Sprintf("from the last %d days", days)
}

// settingsSavedMsg reports whether saving the settings worked.
type settingsSavedMsg struct{ err error }

// saveSettingsCmd saves the window off the UI goroutine. With no state
// directory (tests) there is nothing to save.
func saveSettingsCmd(stateDir string, days int) tea.Cmd {
	if stateDir == "" {
		return nil
	}
	return func() tea.Msg {
		return settingsSavedMsg{err: config.SaveSettings(stateDir, config.Settings{HistoryDays: config.Int(days)})}
	}
}

// cycleHistory moves to the next window: the cards narrow at once, a
// refresh fetches anything a wider window adds, and the choice is saved.
func (m homeModel) cycleHistory() (homeModel, tea.Cmd) {
	m.historyDays = nextHistoryWindow(m.historyDays)
	m.rebuild()
	status := m.setStatus(statusSuccess, "stopped sessions "+historyPhrase(m.historyDays))
	refresh := m.refreshCmd()
	return m, tea.Batch(status, refresh, saveSettingsCmd(m.deps.Cfg.StateDir, m.historyDays))
}
