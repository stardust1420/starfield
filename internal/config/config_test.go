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
