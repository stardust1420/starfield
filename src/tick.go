package src

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// tickMsg carries the id of the model that scheduled it. A tick that is still
// in flight when the screen changes is delivered to the new model, which
// ignores it because the id doesn't match; that stops refresh loops piling up.
type tickMsg struct {
	id int
	t  time.Time
}

var lastTickID atomic.Int64

func newTickID() int {
	return int(lastTickID.Add(1))
}

// refreshInterval is how often a screen refreshes its data.
const refreshInterval = 2 * time.Second

// scheduleTick arms the next refresh. It is tickCmd; tests swap it to count
// how many refresh loops a screen is running.
var scheduleTick = tickCmd

func tickCmd(id int) tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg{id: id, t: t}
	})
}
