package src

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/config"
)

// Deps is everything the screens need from the outside world. Later tasks
// add the session store, tmux client and so on.
type Deps struct {
	Cfg config.Config
}

// homeModel is a stub until the real home screen lands: it shows the header
// and quits on q / ctrl+c.
type homeModel struct {
	deps          Deps
	width, height int
}

func InitHomeModel(d Deps, width, height int) homeModel {
	return homeModel{deps: d, width: width, height: height}
}

func (h homeModel) Init() tea.Cmd { return tea.SetWindowTitle("starfield") }

func (h homeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, quitKey) {
			return h, tea.Quit
		}
	}
	return h, nil
}

func (h homeModel) View() string {
	return renderHeader(pageWidth(h.width), "", "Sessions")
}
