// Package config reads starfield's settings from the environment.
package config

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Config is starfield's runtime configuration.
type Config struct {
	Roots         []string // directories scanned for projects (STARFIELD_ROOTS)
	HistoryDays   int      // how far back to list stopped sessions, in days; 0 is all time
	DetachKey     string   // tmux key syntax, e.g. "C-]"
	ClaudeCmd     []string // command that launches Claude
	ReducedMotion bool     // freeze the starfield animation
	ClaudeDir     string   // ~/.claude
	StateDir      string   // ~/.config/starfield
}

const defaultHistoryDays = 14

// goos is runtime.GOOS; tests replace it.
var goos = runtime.GOOS

// defaultClaudeCmd keeps the Mac awake while Claude runs (caffeinate is
// macOS only); elsewhere it is plain claude.
func defaultClaudeCmd() []string {
	if goos == "darwin" {
		return []string{"caffeinate", "-is", "claude"}
	}
	return []string{"claude"}
}

// Load builds a Config from getenv (os.Getenv in production) and the home
// directory. Bad values fall back to the defaults.
func Load(getenv func(string) string, home string) Config {
	c := Config{
		HistoryDays: defaultHistoryDays,
		DetachKey:   "C-]",
		ClaudeCmd:   defaultClaudeCmd(),
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

	if n, err := strconv.Atoi(strings.TrimSpace(getenv("STARFIELD_HISTORY_DAYS"))); err == nil && n >= 0 {
		c.HistoryDays = n // 0 is all time
	}
	if v := strings.TrimSpace(getenv("STARFIELD_DETACH_KEY")); validKey(v) {
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

// keyPattern is a tmux key: modifiers (C-, M-, S-), then one printable
// character or a key name such as F5, Enter or PageUp.
var keyPattern = regexp.MustCompile(`^([CMS]-)*([!-~]|[A-Za-z][A-Za-z0-9]+)$`)

// validKey reports whether k can go into the generated tmux.conf as the
// detach key. Characters that end a tmux command or break the status-line
// format it is shown in (, ; # " ' \ { } $) are refused, so a typo can't
// leave a session with no way back.
func validKey(k string) bool {
	return keyPattern.MatchString(k) && !strings.ContainsAny(k, ",;#\"'\\{}$")
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
