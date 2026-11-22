# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Starfield is a Go terminal UI (Bubble Tea + Bubbles + Lip Gloss) that manages Claude Code sessions through tmux. Module path: `github.com/stardust1420/starfield` (Go 1.24). Platforms: darwin and linux.

## Commands

```bash
go build .        # produces ./starfield (gitignored); --version / --help
go run .          # needs tmux on the PATH
go vet ./...
gofmt -l .
go test ./...     # one test: go test ./src/... -run TestName
```

The TUI needs a real TTY, so Claude can't drive it. Check changes with build + vet + test, then have the user run `! go run .`. `go mod tidy` and git writes fail inside the Bash sandbox; run them with the sandbox disabled.

## Architecture

- `main.go` parses `-v/--version` and `-h/--help`, checks that `tmux` is on the PATH, loads `config.Load(os.Getenv, home)` and starts `src.InitHomeModel(deps, 0, 0)`. `src.Deps` carries everything the screens need.
- `internal/config`: env-driven `Config` (`STARFIELD_ROOTS`, `STARFIELD_HISTORY_DAYS`, `STARFIELD_DETACH_KEY`, `STARFIELD_CLAUDE_CMD`, `STARFIELD_REDUCED_MOTION`).
- `src/` is the UI kit, copied from the sibling project stardocker and stripped of Docker code. Screens are independent `tea.Model`s; navigation works by returning a different model from `Update`.
  - `layout.go`: palette (`colorAccent` is Claude coral `#d97757`, green is running, steel `#7aa2f7` is external, grey is stopped), glyph constants (`glyphRunning` ●, `glyphBusy` ◐, `glyphExternal` ◌, `glyphStopped` ○, `glyphStarting` ◔), `renderHeader`, `renderPanelColored`, `renderBottomBar`.
  - `cards.go`: `gridView`, the card grid with selection, scrolling and `setEntries`. Card lines are pre-styled and truncated with `ansiTruncate`.
  - `starfield.go`: animated sky for the empty/home state; `STARFIELD_REDUCED_MOTION=1` freezes it.
  - `input.go`: `textField`, `filterBar`, `matches`. `confirm.go`: `confirmPrompt`, `statusLine`. `help.go`: `newHelp`, shared `keyMap`, `listKeyMap`.
  - `tick.go`: `newTickID`, `tickMsg`, `tickCmd`. Each refreshing model gets its own id and ignores ticks that aren't its own.
  - `home.go`: `Deps` and the home screen (currently a stub).
- Models use value receivers, so mutations inside `Init()` are discarded.
- Every tmux call uses `-L starfield -f <conf>`; never touch the user's default tmux server. Managed session names are `sf-` + the first 8 chars of the Claude session UUID.

## Releasing

CI (`.github/workflows/ci.yml`) runs gofmt, vet, build and test. Pushing a `v*` tag runs GoReleaser (`.goreleaser.yaml`) for linux/darwin x amd64/arm64.
