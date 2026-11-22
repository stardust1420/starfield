// Package sessions combines Claude history, the live registry and tmux panes
// into the single session list the home screen renders.
//
// Merge rules: each live registry entry becomes one session keyed by its
// SessionID (Running when a tmux pane is, or is an ancestor of, its process,
// else External); if several entries share an ID, the managed one wins, else
// the newest StartedAt. Panes left over become Starting, and the remaining
// history entries become Stopped. Sorting is by state, then LastActive
// descending, then ID. Dir falls back history, live, pane; Title falls back
// history title, registry name, last prompt.
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

// Merge combines the three sources using the rules in the package comment.
func Merge(hist []claude.HistorySession, live []claude.LiveSession, panes []tmux.Pane, table procs.Table) []Session {
	histByID := make(map[string]claude.HistorySession, len(hist))
	for _, h := range hist {
		histByID[h.ID] = h
	}
	emitted := map[string]bool{}
	paneUsed := make([]bool, len(panes))
	var out []Session

	for _, l := range pickLive(live, panes, table) {
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
			if managedBy(p, l, table) {
				paneUsed[i] = true
				s.State = Running
				s.TmuxName = p.Session
				if s.Dir == "" {
					s.Dir = p.Dir
				}
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

func managedBy(p tmux.Pane, l claude.LiveSession, table procs.Table) bool {
	return p.PanePID == l.PID || table.IsAncestor(p.PanePID, l.PID)
}

// pickLive keeps one live entry per SessionID, preferring one managed by a
// pane, then the newest StartedAt. Order of first appearance is preserved.
func pickLive(live []claude.LiveSession, panes []tmux.Pane, table procs.Table) []claude.LiveSession {
	managed := func(l claude.LiveSession) bool {
		for _, p := range panes {
			if managedBy(p, l, table) {
				return true
			}
		}
		return false
	}
	idx := map[string]int{}
	var out []claude.LiveSession
	for _, l := range live {
		i, seen := idx[l.SessionID]
		if !seen {
			idx[l.SessionID] = len(out)
			out = append(out, l)
			continue
		}
		cur := out[i]
		cm, lm := managed(cur), managed(l)
		if (lm && !cm) || (lm == cm && l.StartedAt.After(cur.StartedAt)) {
			out[i] = l
		}
	}
	return out
}
