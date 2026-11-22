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

func tickCmd(id int) tea.Cmd {
	return tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
		return tickMsg{id: id, t: t}
	})
}
