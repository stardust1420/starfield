package tmux

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionName(t *testing.T) {
	if got := SessionName("0123456789abcdef"); got != "sf-01234567" {
		t.Fatalf("got %q", got)
	}
	if got := SessionName("abc"); got != "sf-abc" {
		t.Fatalf("short id: got %q", got)
	}
}

func TestParsePanes(t *testing.T) {
	out := "sf-aaaaaaaa\t123\tfull-id-1\t/a/b\t1700000000\t1\n" +
		"other\t456\t\t\t1700000100\t0\n\n"
	got := parsePanes(out)
	want := []Pane{
		{Session: "sf-aaaaaaaa", PanePID: 123, ClaudeID: "full-id-1", Dir: "/a/b", Created: time.Unix(1700000000, 0), Attached: true},
		{Session: "other", PanePID: 456, Created: time.Unix(1700000100, 0)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestRenderConfBindsDetachKey(t *testing.T) {
	conf := RenderConf("C-]")
	for _, s := range []string{"bind -n C-] detach-client", "extended-keys on", "set -g prefix None", "unbind C-b", "#d97757", "C-] ⏎ home"} {
		if !strings.Contains(conf, s) {
			t.Errorf("conf missing %q", s)
		}
	}
}

func TestNewSessionArgv(t *testing.T) {
	var calls [][]string
	c := &Client{Bin: "tmux", Socket: "starfield", ConfPath: "/x/tmux.conf"}
	c.run = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	}
	err := c.NewSession("sf-1", "/d", "id-1", []string{"claude", "--resume", "id-1"}, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	pre := []string{"-L", "starfield", "-f", "/x/tmux.conf"}
	cat := func(a ...string) []string { return append(append([]string{}, pre...), a...) }
	want := [][]string{
		cat("new-session", "-d", "-s", "sf-1", "-c", "/d", "-x", "80", "-y", "24", "--", "claude", "--resume", "id-1"),
		cat("set-option", "-t", "sf-1", "@sf_session", "id-1"),
		cat("set-option", "-t", "sf-1", "@sf_dir", "/d"),
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("got %v\nwant %v", calls, want)
	}
}

func TestAttachCmdDropsTMUXEnv(t *testing.T) {
	t.Setenv("TMUX", "x")
	c := &Client{Bin: "/bin/tmux", Socket: "starfield", ConfPath: "/x/tmux.conf"}
	cmd := c.AttachCmd("sf-1")
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "TMUX=") {
			t.Fatalf("TMUX leaked: %q", e)
		}
	}
	if len(cmd.Env) == 0 {
		t.Fatal("env empty")
	}
	want := []string{"/bin/tmux", "-L", "starfield", "-f", "/x/tmux.conf", "attach-session", "-t", "sf-1"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("got %v", cmd.Args)
	}
}
