// Package config reads starfield's settings from the environment.
package config

import (
	"path/filepath"
	"strconv"
	"strings"
)

// Config is starfield's runtime configuration.
type Config struct {
	Roots         []string // directories scanned for projects (STARFIELD_ROOTS)
	HistoryDays   int      // how far back to list stopped sessions
	DetachKey     string   // tmux key syntax, e.g. "C-]"
	ClaudeCmd     []string // command that launches Claude
	ReducedMotion bool     // freeze the starfield animation
	ClaudeDir     string   // ~/.claude
	StateDir      string   // ~/.config/starfield
}

const defaultHistoryDays = 14

// Load builds a Config from getenv (os.Getenv in production) and the home
// directory. Bad values fall back to the defaults.
func Load(getenv func(string) string, home string) Config {
	c := Config{
		HistoryDays: defaultHistoryDays,
		DetachKey:   "C-]",
		ClaudeCmd:   []string{"caffeinate", "-is", "claude"},
		ClaudeDir:   filepath.Join(home, ".claude"),
		StateDir:    filepath.Join(home, ".config", "starfield"),
	}

	if v := getenv("STARFIELD_ROOTS"); v != "" {
		for _, r := range strings.Split(v, ":") {
			if r = expandHome(strings.TrimSpace(r), home); r != "" {
				c.Roots = append(c.Roots, r)
			}
		}
	}
	if len(c.Roots) == 0 {
		c.Roots = []string{filepath.Join(home, "Developer")}
	}

	if n, err := strconv.Atoi(strings.TrimSpace(getenv("STARFIELD_HISTORY_DAYS"))); err == nil && n > 0 {
		c.HistoryDays = n
	}
	if v := strings.TrimSpace(getenv("STARFIELD_DETACH_KEY")); v != "" {
		c.DetachKey = v
	}
	if f := strings.Fields(getenv("STARFIELD_CLAUDE_CMD")); len(f) > 0 {
		c.ClaudeCmd = f
	}
	if v := getenv("STARFIELD_REDUCED_MOTION"); v != "" && v != "0" {
		c.ReducedMotion = true
	}
	return c
}

func expandHome(p, home string) string {
	switch {
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	}
	return p
}
