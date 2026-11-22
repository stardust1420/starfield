package procs

import "testing"

func TestParsePS(t *testing.T) {
	out := "  1     0 Sun Nov 22 11:28:31 2026 /sbin/launchd\n" +
		"   50     1 Mon Nov  2 09:01:02 2026 -/bin/zsh\n" +
		" 60 50 Mon Nov  2 09:01:03 2026 /Applications/My App/bin/tool\n" +
		"junk line\n" +
		"abc 1 Sun Nov 22 11:28:31 2026 x\n" +
		" 80 1 Sun Nov 22\n" +
		"\n" +
		" 70 60 Sun Nov 22 11:28:31 2026 claude\n"
	tb := ParsePS(out)
	if len(tb) != 4 {
		t.Fatalf("len=%d %v", len(tb), tb)
	}
	if tb[60].Comm != "/Applications/My App/bin/tool" || tb[60].PPID != 50 {
		t.Errorf("bad 60: %+v", tb[60])
	}
	if tb[50].Comm != "-/bin/zsh" || tb[50].Start != "Mon Nov 2 09:01:02 2026" {
		t.Errorf("bad 50: %+v", tb[50])
	}
	if tb[70].Comm != "claude" || tb[70].Start != "Sun Nov 22 11:28:31 2026" {
		t.Errorf("bad 70: %+v", tb[70])
	}
}

func TestIsAncestor(t *testing.T) {
	tb := Table{
		1:  {PID: 1, PPID: 0, Comm: "init"},
		50: {PID: 50, PPID: 1, Comm: "tmux"},
		60: {PID: 60, PPID: 50, Comm: "zsh"},
		70: {PID: 70, PPID: 60, Comm: "claude"},
	}
	if !tb.IsAncestor(50, 70) {
		t.Error("50 should be ancestor of 70")
	}
	if tb.IsAncestor(60, 50) {
		t.Error("60 not ancestor of 50")
	}
	cyc := Table{1: {PID: 1, PPID: 2, Comm: "a"}, 2: {PID: 2, PPID: 1, Comm: "b"}}
	if cyc.IsAncestor(99, 1) {
		t.Error("cycle should not match")
	}
}

func TestDepth(t *testing.T) {
	tb := Table{
		1:  {PID: 1, PPID: 0, Comm: "init"},
		50: {PID: 50, PPID: 1, Comm: "tmux"},
		60: {PID: 60, PPID: 50, Comm: "zsh"},
		70: {PID: 70, PPID: 60, Comm: "claude"},
	}
	for _, c := range []struct{ anc, pid, want int }{
		{70, 70, 0}, {60, 70, 1}, {50, 70, 2}, {70, 50, -1}, {99, 70, -1},
	} {
		if got := tb.Depth(c.anc, c.pid); got != c.want {
			t.Errorf("Depth(%d,%d) = %d; want %d", c.anc, c.pid, got, c.want)
		}
	}
	cyc := Table{1: {PID: 1, PPID: 2, Comm: "a"}, 2: {PID: 2, PPID: 1, Comm: "b"}}
	if got := cyc.Depth(99, 1); got != -1 {
		t.Errorf("cycle: %d", got)
	}
}

func TestIsClaude(t *testing.T) {
	tb := Table{
		1: {PID: 1, Comm: "claude"},
		2: {PID: 2, Comm: "/Users/x/.local/bin/claude"},
		3: {PID: 3, Comm: "claude-helper"},
		4: {PID: 4, Comm: "node"},
		5: {PID: 5, Comm: "/opt/homebrew/bin/node"},
		6: {PID: 6, Comm: "nodejs-thing"},
	}
	for pid, want := range map[int]bool{1: true, 2: true, 3: false, 4: true, 5: true, 6: false, 9: false} {
		if got := tb.IsClaude(pid); got != want {
			t.Errorf("pid %d: got %v want %v", pid, got, want)
		}
	}
}
