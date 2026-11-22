package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestReadRegistryMatchesProcStartAndKind(t *testing.T) {
	dir := t.TempDir()
	w := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// node comm, matching start (ps pads the day) -> kept.
	w("1.json", `{"pid":1,"sessionId":"s1","procStart":"Mon Nov 2 11:28:31 2026","kind":"interactive"}`)
	// reused pid: claude comm but a different start -> dropped.
	w("2.json", `{"pid":2,"sessionId":"s2","procStart":"Sun Nov 22 11:28:31 2026"}`)
	// non-interactive kinds -> dropped.
	w("3.json", `{"pid":3,"sessionId":"s3","procStart":"Sun Nov 22 11:28:31 2026","kind":"print"}`)
	w("4.json", `{"pid":4,"sessionId":"s4","kind":"sdk"}`)
	// no procStart: falls back to the comm check, node is accepted.
	w("5.json", `{"pid":5,"sessionId":"s5"}`)
	// procStart but the table has no start: falls back to the comm check.
	w("6.json", `{"pid":6,"sessionId":"s6","procStart":"Sun Nov 22 11:28:31 2026"}`)
	// node with matching start but not in the table -> dropped.
	w("7.json", `{"pid":7,"sessionId":"s7","procStart":"Sun Nov 22 11:28:31 2026"}`)
	table := procs.Table{
		1: {PID: 1, Comm: "node", Start: "Mon Nov  2 11:28:31 2026"},
		2: {PID: 2, Comm: "claude", Start: "Sun Nov 22 12:00:00 2026"},
		3: {PID: 3, Comm: "claude", Start: "Sun Nov 22 11:28:31 2026"},
		4: {PID: 4, Comm: "claude"},
		5: {PID: 5, Comm: "node"},
		6: {PID: 6, Comm: "bash"},
	}
	got, err := ReadRegistry(dir, table)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.SessionID] = true
	}
	if len(got) != 2 || !ids["s1"] || !ids["s5"] {
		t.Fatalf("got %+v; want s1 and s5", got)
	}
}

// Claude writes procStart in UTC while ps prints lstart in local time (seen
// on a real machine: "Sun Nov 22 11:28:31 2026" vs "Sun Nov 22 16:58:31
// 2026" in IST).
func TestReadRegistryProcStartUTCvsLocal(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("IST", 5*3600+1800)
	t.Cleanup(func() { time.Local = old })
	dir := t.TempDir()
	w := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("1.json", `{"pid":1,"sessionId":"s1","procStart":"Sun Nov 22 11:28:31 2026","kind":"interactive"}`)
	w("2.json", `{"pid":2,"sessionId":"s2","procStart":"Sun Nov 22 11:28:31 2026","kind":"interactive"}`)
	table := procs.Table{
		1: {PID: 1, Comm: "claude", Start: "Sun Nov 22 16:58:31 2026"},
		2: {PID: 2, Comm: "claude", Start: "Sun Nov 22 16:58:32 2026"}, // reused pid
	}
	got, err := ReadRegistry(dir, table)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "s1" {
		t.Fatalf("got %+v; want only s1", got)
	}
}
