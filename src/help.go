package src

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// newHelp returns a help.Model styled like the rest of the UI: bold keys,
// grey descriptions, plain spacing instead of bullets.
func newHelp() help.Model {
	h := help.New()
	h.ShortSeparator = "   "
	h.FullSeparator = "     "
	h.Styles.ShortKey = crumbStyle
	h.Styles.FullKey = crumbStyle
	h.Styles.ShortDesc = mutedStyle
	h.Styles.FullDesc = mutedStyle
	h.Styles.ShortSeparator = mutedStyle
	h.Styles.FullSeparator = mutedStyle
	h.Styles.Ellipsis = mutedStyle
	return h
}

// keyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Help  key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "down", "left", "right", "k", "j", "h", "l"),
		key.WithHelp("←↑↓→/hjkl", "move"),
	),
	Down: key.NewBinding(
		key.WithKeys("g", "G", "pgup", "pgdown"),
		key.WithHelp("g/G pgup/pgdn", "first, last, page"),
	),
	Left: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Right: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "forward"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
}

var (
	quitKey   = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	filterKey = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
)

// listKeyMap is the help of a list screen: the shared bindings plus that
// screen's actions.
type listKeyMap struct {
	keyMap
	actions []key.Binding
}

func (k listKeyMap) ShortHelp() []key.Binding {
	short := append([]key.Binding{}, k.actions...)
	return append(short, filterKey, k.Left, k.Help)
}

func (k listKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left},
		k.actions,
		{filterKey, quitKey, k.Help},
	}
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Help}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Left, k.Help},
	}
}

// homeKeyMap is the help of the home screen, in one of its two views:
// projects (inProject false) or one project's sessions. Its short help is
// compact so a status message still fits beside it at 80 columns.
type homeKeyMap struct {
	keyMap
	inProject   bool
	Move        key.Binding // short form of keyMap.Up
	HelpShort   key.Binding // short form of keyMap.Help
	Enter       key.Binding // open a project / attach a session
	New         key.Binding
	Back        key.Binding
	Stop        key.Binding
	Preview     key.Binding
	RunningOnly key.Binding
	History     key.Binding
}

func newHomeKeys(inProject bool) homeKeyMap {
	k := homeKeyMap{
		keyMap:      keys,
		inProject:   inProject,
		Move:        key.NewBinding(key.WithKeys("up", "down", "left", "right"), key.WithHelp("←↑↓→", "move")),
		HelpShort:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		New:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new project")),
		Back:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "projects")),
		Stop:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Preview:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preview")),
		RunningOnly: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all/running")),
		History:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "history window")),
	}
	if inProject {
		k.Enter.SetHelp("enter", "attach")
		k.New.SetHelp("n", "new session")
	}
	return k
}

var (
	projectKeys = newHomeKeys(false)
	sessionKeys = newHomeKeys(true)
)

func (k homeKeyMap) ShortHelp() []key.Binding {
	// The primary actions only; the rest are in the full help, so a status
	// message has room beside the keys.
	if k.inProject {
		return []key.Binding{k.Enter, k.New, k.Back, filterKey, k.HelpShort, quitKey}
	}
	return []key.Binding{k.Enter, k.New, k.RunningOnly, filterKey, k.HelpShort, quitKey}
}

func (k homeKeyMap) FullHelp() [][]key.Binding {
	if k.inProject {
		return [][]key.Binding{
			{k.Up, k.Down},
			{k.Enter, k.New, k.Stop, k.Preview, k.RunningOnly, k.History},
			{filterKey, k.Back, k.Help, quitKey},
		}
	}
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter, k.New, k.RunningOnly, k.History},
		{filterKey, k.Help, quitKey},
	}
}
