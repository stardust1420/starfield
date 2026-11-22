// Package sessions combines Claude history, the live registry and tmux panes
// into the single session list the home screen renders.
//
// Merge rules: each live registry entry becomes one session keyed by its
// SessionID (Running when a tmux pane is, or is an ancestor of, its process,
// else External); if several entries share an ID, the managed one wins, else
// the newest StartedAt. Each pane goes to at most one live session (and each
// live session gets at most one pane), best match first: the pane's Claude
// id is the session's, then the pane's own process is the session's, then
// the fewest parent hops from the pane to the process. Panes left over become Starting, and the remaining
// history entries become Stopped. Sorting is by state, then LastActive
// descending, then ID. Dir falls back history, live, pane; Title falls back
// history title, registry name, last prompt. Title, LastPrompt, Branch, Model
// and Status are passed through Sanitize; Dir is not (tmux uses it).
package sessions

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

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

	picked := pickLive(live, panes, table)
	paneOf := assignPanes(picked, panes, table)
	for li, l := range picked {
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
		if i, ok := paneOf[li]; ok {
			p := panes[i]
			paneUsed[i] = true
			s.State = Running
			s.TmuxName = p.Session
			if s.Dir == "" {
				s.Dir = p.Dir
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

	for i := range out {
		s := &out[i]
		s.Title, s.LastPrompt = Sanitize(s.Title), Sanitize(s.LastPrompt)
		s.Branch, s.Model, s.Status = Sanitize(s.Branch), Sanitize(s.Model), Sanitize(s.Status)
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
	return table.Depth(p.PanePID, l.PID) >= 0
}

// assignPanes matches live sessions (by index) to panes (by index), at most
// one each way, best match first: Claude id equal, then pane pid equal, then
// fewest hops; remaining ties go by session id, pid and pane name so the
// result doesn't depend on the order the sources listed them in.
func assignPanes(live []claude.LiveSession, panes []tmux.Pane, table procs.Table) map[int]int {
	type pair struct {
		l, p      int
		idOK, pid bool
		depth     int
	}
	var pairs []pair
	for li, l := range live {
		for pi, p := range panes {
			d := table.Depth(p.PanePID, l.PID)
			if d < 0 {
				continue
			}
			pairs = append(pairs, pair{li, pi, p.ClaudeID != "" && p.ClaudeID == l.SessionID, p.PanePID == l.PID, d})
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if a.idOK != b.idOK {
			return a.idOK
		}
		if a.pid != b.pid {
			return a.pid
		}
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		la, lb := live[a.l], live[b.l]
		if la.SessionID != lb.SessionID {
			return la.SessionID < lb.SessionID
		}
		if la.PID != lb.PID {
			return la.PID < lb.PID
		}
		return panes[a.p].Session < panes[b.p].Session
	})
	out := map[int]int{}
	paneTaken := map[int]bool{}
	for _, pr := range pairs {
		if _, done := out[pr.l]; done || paneTaken[pr.p] {
			continue
		}
		out[pr.l] = pr.p
		paneTaken[pr.p] = true
	}
	return out
}

// Sanitize makes untrusted text (titles, prompts, names from JSON) safe to
// print: escape sequences are removed, tabs and line breaks become spaces,
// and any other C0/C1 control character or DEL is dropped.
func Sanitize(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			return -1
		}
		return r
	}, s)
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
