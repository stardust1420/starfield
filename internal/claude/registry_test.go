package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stardust1420/starfield/internal/procs"
)

func TestReadRegistrySkipsDeadOrReusedPID(t *testing.T) {
	dir := t.TempDir()
	w := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("70.json", `{"pid":70,"sessionId":"s70","cwd":"/a","startedAt":1795346917028,"name":"n70","status":"busy"}`)
	w("80.json", `{"pid":80,"sessionId":"s80","cwd":"/b","startedAt":1795346917028}`)
	w("90.json", `{"pid":90,"sessionId":"s90","cwd":"/c","startedAt":1795346917028}`)
	w("100.json", `{not json`)
	w("70.abc.key", `secret`)
	table := procs.Table{
		70: {PID: 70, PPID: 1, Comm: "claude"},
		90: {PID: 90, PPID: 1, Comm: "node-other"},
	}
	got, err := ReadRegistry(dir, table)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	s := got[0]
	if s.PID != 70 || s.SessionID != "s70" || s.Dir != "/a" || s.Name != "n70" || s.Status != "busy" {
		t.Errorf("bad %+v", s)
	}
	if s.StartedAt.UnixMilli() != 1795346917028 {
		t.Errorf("startedAt %v", s.StartedAt)
	}
}

func TestReadRegistryMissingDir(t *testing.T) {
	got, err := ReadRegistry(filepath.Join(t.TempDir(), "nope"), procs.Table{})
	if err != nil || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}
