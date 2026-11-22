// Package sessions combines Claude history, the live registry and tmux panes
// into the single session list the home screen renders.
package sessions

import (
	"sort"
	"time"

	"github.com/stardust1420/starfield/internal/claude"
	"github.com/stardust1420/starfield/internal/procs"
	"github.com/stardust1420/starfield/internal/tmux"
)

// State orders sessions on screen: lower values sort first.
type State int

const (
	Running  State = iota // managed by a tmux pane, Claude alive
	Starting              // managed tmux pane, no registry entry yet
	External              // Claude alive outside Starfield's tmux
	Stopped               // history only
)

// Session is one row of the session list.
type Session struct {
	ID, Dir, Title, LastPrompt, Branch, Model, Status, TmuxName string
	CostUSD                                                     float64
	UserMessages                                                int
	Truncated                                                   bool // UserMessages is a lower bound
	Started, LastActive                                         time.Time
	State                                                       State
	PID                                                         int // live PID for Running/External, else 0
}

// Merge combines the three sources. See the package docs for the rules.
func Merge(hist []claude.HistorySession, live []claude.LiveSession, panes []tmux.Pane, table procs.Table) []Session {
	histByID := make(map[string]claude.HistorySession, len(hist))
	for _, h := range hist {
		histByID[h.ID] = h
	}
	emitted := map[string]bool{}
	paneUsed := make([]bool, len(panes))
	var out []Session

	for _, l := range live {
		s := fromHistory(histByID[l.SessionID], l.SessionID)
		s.State = External
		s.PID = l.PID
		s.Status = l.Status
		s.Started = l.StartedAt
		if s.Title == "" {
			s.Title = l.Name
		}
		if s.Title == "" {
			s.Title = s.LastPrompt
		}
		if s.Dir == "" {
			s.Dir = l.Dir
		}
		if s.LastActive.IsZero() {
			s.LastActive = l.StartedAt
		}
		for i, p := range panes {
			if paneUsed[i] {
				continue
			}
			if p.PanePID == l.PID || table.IsAncestor(p.PanePID, l.PID) {
				paneUsed[i] = true
				s.State = Running
				s.TmuxName = p.Session
				break
			}
		}
		emitted[l.SessionID] = true
		out = append(out, s)
	}

	for i, p := range panes {
		if paneUsed[i] || emitted[p.ClaudeID] {
			continue
		}
		s := fromHistory(histByID[p.ClaudeID], p.ClaudeID)
		s.State = Starting
		s.TmuxName = p.Session
		if s.Dir == "" {
			s.Dir = p.Dir
		}
		if s.Started.IsZero() {
			s.Started = p.Created
		}
		if s.LastActive.IsZero() {
			s.LastActive = p.Created
		}
		s.Title = firstNonEmpty(s.Title, s.LastPrompt)
		emitted[p.ClaudeID] = true
		out = append(out, s)
	}

	for _, h := range hist {
		if emitted[h.ID] {
			continue
		}
		s := fromHistory(h, h.ID)
		s.State = Stopped
		s.Title = firstNonEmpty(s.Title, s.LastPrompt)
		emitted[h.ID] = true
		out = append(out, s)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.State != b.State {
			return a.State < b.State
		}
		if !a.LastActive.Equal(b.LastActive) {
			return a.LastActive.After(b.LastActive)
		}
		return a.ID < b.ID
	})
	return out
}

func fromHistory(h claude.HistorySession, id string) Session {
	return Session{
		ID: id, Dir: h.Dir, Title: h.Title, LastPrompt: h.LastPrompt,
		Branch: h.Branch, Model: h.Model, CostUSD: h.CostUSD,
		UserMessages: h.UserMessages, Truncated: h.Truncated,
		Started: h.Started, LastActive: h.LastActive,
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
