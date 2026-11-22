package procs

import "testing"

func TestParsePS(t *testing.T) {
	out := "  1     0 /sbin/launchd\n" +
		"   50     1 -/bin/zsh\n" +
		" 60 50 /Applications/My App/bin/tool\n" +
		"junk line\n" +
		"abc 1 x\n" +
		"\n" +
		" 70 60 claude\n"
	tb := ParsePS(out)
	if len(tb) != 4 {
		t.Fatalf("len=%d %v", len(tb), tb)
	}
	if tb[60].Comm != "/Applications/My App/bin/tool" || tb[60].PPID != 50 {
		t.Errorf("bad 60: %+v", tb[60])
	}
	if tb[50].Comm != "-/bin/zsh" {
		t.Errorf("bad 50: %+v", tb[50])
	}
	if tb[70].Comm != "claude" {
		t.Errorf("bad 70: %+v", tb[70])
	}
}

func TestIsAncestor(t *testing.T) {
	tb := Table{
		1:  {1, 0, "init"},
		50: {50, 1, "tmux"},
		60: {60, 50, "zsh"},
		70: {70, 60, "claude"},
	}
	if !tb.IsAncestor(50, 70) {
		t.Error("50 should be ancestor of 70")
	}
	if tb.IsAncestor(60, 50) {
		t.Error("60 not ancestor of 50")
	}
	cyc := Table{1: {1, 2, "a"}, 2: {2, 1, "b"}}
	if cyc.IsAncestor(99, 1) {
		t.Error("cycle should not match")
	}
}

func TestIsClaude(t *testing.T) {
	tb := Table{
		1: {1, 0, "claude"},
		2: {2, 0, "/Users/x/.local/bin/claude"},
		3: {3, 0, "claude-helper"},
		4: {4, 0, "node"},
	}
	for pid, want := range map[int]bool{1: true, 2: true, 3: false, 4: false, 9: false} {
		if got := tb.IsClaude(pid); got != want {
			t.Errorf("pid %d: got %v want %v", pid, got, want)
		}
	}
}
