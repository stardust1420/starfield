package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/src"
)

// version is set at build time by GoReleaser (-X main.version=...). For
// `go install ...@vX.Y.Z` it comes from the module's build info instead.
var version = "dev"

func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

const usage = `starfield: a terminal UI for your Claude Code sessions, run through tmux.

Usage:
  starfield [flags]

Flags:
  -h, --help       Show this help
  -v, --version    Print the version

Environment:
  STARFIELD_ROOTS=~/Developer   Colon-separated directories scanned for projects
  STARFIELD_HISTORY_DAYS=14     How many days of stopped sessions to list
  STARFIELD_DETACH_KEY=C-]      Key (tmux syntax) that returns to the home screen
  STARFIELD_CLAUDE_CMD="caffeinate -is claude"
                                Command that launches Claude (split on spaces)
  STARFIELD_REDUCED_MOTION=1    Keep the starfield still

Press ? inside the app for keybindings.
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	showVersion := flag.Bool("version", false, "")
	flag.BoolVar(showVersion, "v", false, "")
	flag.Parse()

	if *showVersion {
		fmt.Println("starfield", resolvedVersion())
		return
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "starfield: unexpected argument %q\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}

	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "starfield needs tmux: brew install tmux")
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "starfield: %v\n", err)
		os.Exit(1)
	}
	deps := src.Deps{Cfg: config.Load(os.Getenv, home)}

	p := tea.NewProgram(src.InitHomeModel(deps, 0, 0), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "starfield: %v\n", err)
		os.Exit(1)
	}
}
