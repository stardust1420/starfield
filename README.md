# starfield

A terminal UI for your Claude Code sessions. Starfield shows every session you have, in every directory, as a card: running, running in another terminal, or stopped. Press enter to jump into one, press the detach key to come back, and the session keeps running underneath. Sessions live in a private tmux server, so shift+enter, colours, mouse scroll and the clipboard all behave like a normal terminal.

## Requirements

- macOS or Linux
- Go 1.24 or newer (to build)
- tmux 3.3 or newer (`brew install tmux`)
- the Claude Code CLI (`claude`) on your PATH

## Install

```bash
go install github.com/stardust1420/starfield@latest
```

or from a checkout:

```bash
go build .      # produces ./starfield
```

Tip: `alias sf=starfield` in your shell profile.

## How it works

Starfield runs every managed Claude session in its own tmux session on a private socket (`tmux -L starfield`), separate from your normal tmux. Entering a session attaches your terminal to it. The detach key (default `C-]`) returns to the home screen, and the session keeps running. Quitting starfield with `q` leaves all sessions running too. To see them from outside: `tmux -L starfield ls`.

Stopped sessions come from Claude's own history (`~/.claude`); pressing enter on one resumes it with `claude --resume`.

## Keybindings

Starfield opens on your **projects**: one card per directory that has Claude sessions, under the STARFIELD banner. `enter` opens a project to show its sessions; `esc` goes back.

### Projects

| Key | Action |
| --- | --- |
| `←↑↓→` / `hjkl` | move the selection |
| `g` / `G`, `pgup` / `pgdn` | first, last, page |
| `tab` / `shift+tab` | next / previous card |
| `enter` | open the project's sessions |
| `n` | new project (opens the directory picker) |
| `a` | toggle all projects / only those with a running session |
| `t` | history window for stopped sessions: 1d → 3d → 7d → 14d → 30d → 90d → all time (shown as `◷ 14d` in the header; remembered across launches) |
| `/` | filter projects by name or path |
| `esc` | clear the filter |
| `?` | toggle the full help |
| `q` / `ctrl+c` | quit (sessions keep running) |

### Sessions (inside a project)

| Key | Action |
| --- | --- |
| `←↑↓→` / `hjkl`, `g` / `G`, `pgup` / `pgdn`, `tab` | move, as on projects |
| `enter` | attach to a running session, or resume a stopped one |
| `n` | new session in this project |
| `x` | stop the selected managed session (asks first) |
| `p` | toggle the live preview panel (pane contents, or last prompt for stopped/external sessions) |
| `a` | toggle all sessions / running only |
| `t` | history window, as on projects (one setting for both) |
| `/` | filter sessions by title, branch or last prompt |
| `esc` | clear the filter, then back to projects |
| `?` | toggle the full help |
| `q` / `ctrl+c` | quit (sessions keep running) |

### New session picker

| Key | Action |
| --- | --- |
| typing | fuzzy-filter recent and scanned directories; a path (`/`, `~`, `.`) is offered as typed, and a plain name is offered as a new directory under the first root (`myproj` → `~/Developer/myproj`) |
| `↑` / `↓` (`ctrl+p` / `ctrl+n`) | select |
| `pgup` / `pgdn` | page |
| `tab` | complete the query to the selected path |
| `enter` | start Claude there and attach (detaching returns to that project's sessions) |
| `esc` | back |

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `STARFIELD_ROOTS` | `~/Developer` | colon-separated directories scanned for repos (depth 2) in the picker; new directories made from a plain name go in the first one |
| `STARFIELD_HISTORY_DAYS` | `14` | starting window for stopped sessions, in days; once you pick one with `t` it is saved to `~/.config/starfield/settings.json` and used instead |
| `STARFIELD_DETACH_KEY` | `C-]` | key that returns to the home screen; a plain tmux key name such as `C-]` or `C-q`. A changed key takes effect once the private tmux server restarts: when no starfield sessions are running, or after `tmux -L starfield kill-server` |
| `STARFIELD_CLAUDE_CMD` | `caffeinate -is claude` on macOS, `claude` elsewhere | command that launches Claude, split on whitespace |
| `STARFIELD_REDUCED_MOTION` | unset | `1` keeps the starfield and busy glyph still |

## FAQ

### Why can't I attach to external sessions?

An external session (`◌`) is a Claude process running in some other terminal, not under starfield's tmux, so there is nothing for starfield to attach to. Quit it there (`/exit`), then press enter on its card in starfield to resume it as a managed session.

## Troubleshooting

- Shift+enter inside tmux needs extended keys; starfield's private tmux config turns them on, so it works out of the box in terminals that support them (Ghostty, kitty, iTerm2, WezTerm).
- Mouse scroll inside a session uses tmux's scrollback (mouse mode is on).
- `starfield needs tmux`: install it with `brew install tmux`.

## License

MIT
