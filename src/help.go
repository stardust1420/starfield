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

// homeKeyMap is the help of the home screen. Its short help is compact so
// a status message still fits beside it at 80 columns. Tasks that add
// session actions (enter, n, x, p) append their bindings to actions.
type homeKeyMap struct {
	keyMap
	Move        key.Binding // short form of keyMap.Up
	HelpShort   key.Binding // short form of keyMap.Help
	RunningOnly key.Binding
	actions     []key.Binding
}

var homeKeys = homeKeyMap{
	keyMap:      keys,
	Move:        key.NewBinding(key.WithKeys("up", "down", "left", "right"), key.WithHelp("←↑↓→", "move")),
	HelpShort:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	RunningOnly: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all/running")),
	actions: []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "attach")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
	},
}

func (k homeKeyMap) ShortHelp() []key.Binding {
	// Only the primary action; the rest are in the full help, so a status
	// message has room beside the keys.
	short := append([]key.Binding{}, k.actions[:1]...)
	return append(short, k.RunningOnly, filterKey, k.HelpShort, quitKey)
}

func (k homeKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		append(append([]key.Binding{}, k.actions...), k.RunningOnly),
		{filterKey, k.Help, quitKey},
	}
}
