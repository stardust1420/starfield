package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTmuxLifecycle(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	// Short dir: unix socket paths are limited to ~104 bytes.
	tmp, err := os.MkdirTemp("/tmp", "sf")
	if err != nil {
		tmp, err = os.MkdirTemp("", "sf") // sandboxed: fall back to $TMPDIR
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(tmp) > 50 {
		os.RemoveAll(tmp)
		t.Skipf("temp dir too long for a unix socket: %s", tmp)
	}
	t.Setenv("TMUX_TMPDIR", tmp)
	sock := fmt.Sprintf("sft%d", time.Now().UnixNano()%1e9)
	conf := tmp + "/tmux.conf"
	if err := os.WriteFile(conf, []byte(RenderConf("C-]")), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Client{Bin: bin, Socket: sock, ConfPath: conf}
	c.run = c.exec
	t.Cleanup(func() {
		_ = exec.Command(bin, "-L", sock, "-f", conf, "kill-server").Run()
		os.RemoveAll(tmp)
	})

	if ps, err := c.List(); err != nil || len(ps) != 0 {
		t.Fatalf("pre-list: %v %v", ps, err)
	}
	id := "11111111-2222-4333-8444-555555555555"
	if err := c.NewSession("sf-test", t.TempDir(), id, []string{"sleep", "30"}, 80, 24); err != nil {
		if strings.Contains(err.Error(), "Operation not permitted") {
			t.Skipf("sandbox forbids creating the tmux socket: %v", err)
		}
		t.Fatal(err)
	}
	// The conf must actually have been applied: tmux starts the server even
	// when a conf line is malformed.
	show := func(args ...string) string {
		out, err := c.do(args...)
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if msgs := show("show-messages"); strings.Contains(msgs, "tmux.conf") {
		t.Errorf("conf errors: %s", msgs)
	}
	if got := show("show-options", "-gv", "prefix"); got != "None" {
		t.Errorf("prefix = %q, want None", got)
	}
	detachBound := false
	for _, l := range strings.Split(show("list-keys", "-T", "root"), "\n") {
		if strings.Contains(l, "C-]") && strings.Contains(l, "detach-client") {
			detachBound = true
		}
	}
	if !detachBound {
		t.Error("C-] not bound to detach-client in root table")
	}
	if got := show("show-options", "-sv", "extended-keys"); got != "on" {
		t.Errorf("extended-keys = %q, want on", got)
	}

	ps, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ps {
		if p.Session == "sf-test" && p.ClaudeID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("session not listed: %+v", ps)
	}
	if _, err := c.Capture("sf-test", 10); err != nil {
		t.Fatal(err)
	}
	// Targets are exact: a prefix of the name must not match.
	if err := c.Kill("sf-tes"); err == nil {
		t.Error("Kill(sf-tes) matched sf-test")
	}
	if _, err := c.Capture("sf-tes", 10); err == nil {
		t.Error("Capture(sf-tes) matched sf-test")
	}
	if err := c.Kill("sf-test"); err != nil {
		t.Fatal(err)
	}
	if ps, err := c.List(); err != nil || len(ps) != 0 {
		t.Fatalf("post-list: %v %v", ps, err)
	}
}
