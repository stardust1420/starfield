package config

import (
	"reflect"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	home := "/home/u"
	old := goos
	goos = "darwin" // the default command depends on the OS
	t.Cleanup(func() { goos = old })

	c := Load(env(nil), home)
	if want := []string{home + "/Developer"}; !reflect.DeepEqual(c.Roots, want) {
		t.Errorf("Roots = %v; want %v", c.Roots, want)
	}
	if c.HistoryDays != 14 {
		t.Errorf("HistoryDays = %d; want 14", c.HistoryDays)
	}
	if c.DetachKey != "C-]" {
		t.Errorf("DetachKey = %q; want C-]", c.DetachKey)
	}
	if want := []string{"caffeinate", "-is", "claude"}; !reflect.DeepEqual(c.ClaudeCmd, want) {
		t.Errorf("ClaudeCmd = %v; want %v", c.ClaudeCmd, want)
	}
	if c.ClaudeDir != home+"/.claude" || c.StateDir != home+"/.config/starfield" {
		t.Errorf("dirs = %q, %q", c.ClaudeDir, c.StateDir)
	}
	if c.ReducedMotion {
		t.Error("ReducedMotion should default to false")
	}

	c = Load(env(map[string]string{"STARFIELD_ROOTS": "~/a:/b"}), home)
	if want := []string{home + "/a", "/b"}; !reflect.DeepEqual(c.Roots, want) {
		t.Errorf("Roots = %v; want %v", c.Roots, want)
	}

	c = Load(env(map[string]string{"STARFIELD_HISTORY_DAYS": "x"}), home)
	if c.HistoryDays != 14 {
		t.Errorf("HistoryDays = %d; want 14 on bad input", c.HistoryDays)
	}

	c = Load(env(map[string]string{
		"STARFIELD_HISTORY_DAYS":   "3",
		"STARFIELD_DETACH_KEY":     "C-b",
		"STARFIELD_CLAUDE_CMD":     "claude  --foo",
		"STARFIELD_REDUCED_MOTION": "1",
	}), home)
	if c.HistoryDays != 3 || c.DetachKey != "C-b" || !c.ReducedMotion ||
		!reflect.DeepEqual(c.ClaudeCmd, []string{"claude", "--foo"}) {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestDefaultClaudeCmdPerOS(t *testing.T) {
	old := goos
	t.Cleanup(func() { goos = old })
	for os, want := range map[string][]string{
		"darwin": {"caffeinate", "-is", "claude"},
		"linux":  {"claude"},
	} {
		goos = os
		if got := Load(env(nil), "/home/u").ClaudeCmd; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ClaudeCmd = %v; want %v", os, got, want)
		}
	}
}

func TestHistoryDaysZeroMeansAllTime(t *testing.T) {
	c := Load(func(k string) string {
		if k == "STARFIELD_HISTORY_DAYS" {
			return "0"
		}
		return ""
	}, "/home/u")
	if c.HistoryDays != 0 {
		t.Errorf("HistoryDays = %d; want 0 (all time)", c.HistoryDays)
	}
}

func TestDetachKeyIsValidated(t *testing.T) {
	for k, want := range map[string]string{
		"C-]":      "C-]",
		"M-q":      "M-q",
		"C-M-x":    "C-M-x",
		"F12":      "F12",
		"C-a,b":    "C-]", // a comma breaks the status-line format
		"C-a b":    "C-]", // whitespace breaks bind
		"C-a;kill": "C-]",
		"#":        "C-]",
		"C-{":      "C-]",
		"C-":       "C-]",
	} {
		c := Load(func(name string) string {
			if name == "STARFIELD_DETACH_KEY" {
				return k
			}
			return ""
		}, "/home/u")
		if c.DetachKey != want {
			t.Errorf("STARFIELD_DETACH_KEY=%q gave %q; want %q", k, c.DetachKey, want)
		}
	}
}
