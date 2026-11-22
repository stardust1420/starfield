package claude

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const basicID = "11111111-1111-4111-8111-111111111111"

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func copyFixture(t *testing.T, name, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseSessionFileBasic(t *testing.T) {
	s, err := ParseSessionFile(filepath.Join("testdata", "basic.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != basicID {
		t.Errorf("ID = %q", s.ID)
	}
	if s.Dir != "/x/my_proj" {
		t.Errorf("Dir = %q", s.Dir)
	}
	if s.Title != "Widget colors" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.LastPrompt != "now make it green" {
		t.Errorf("LastPrompt = %q", s.LastPrompt)
	}
	if s.Branch != "main" {
		t.Errorf("Branch = %q", s.Branch)
	}
	if s.Model != "opus-5-5" {
		t.Errorf("Model = %q", s.Model)
	}
	if s.CostUSD != 10.22 {
		t.Errorf("CostUSD = %v", s.CostUSD)
	}
	if s.UserMessages != 2 {
		t.Errorf("UserMessages = %d", s.UserMessages)
	}
	if want := mustTime(t, "2026-11-21T13:40:54.043Z"); !s.Started.Equal(want) {
		t.Errorf("Started = %v", s.Started)
	}
	if want := mustTime(t, "2026-11-21T13:55:00.000Z"); !s.LastActive.Equal(want) {
		t.Errorf("LastActive = %v", s.LastActive)
	}
	if s.Truncated {
		t.Error("Truncated should be false for a small file")
	}
}

func TestParseSessionFileTruncatedLastLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trunc.jsonl")
	copyFixture(t, "basic.jsonl", p)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"type":"assistant","message":{"model":"claude-son`)
	f.Close()
	s, err := ParseSessionFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus-5-5" || s.Title != "Widget colors" || s.UserMessages != 2 {
		t.Errorf("unexpected parse: %+v", s)
	}
}

type countingFile struct {
	f     *os.File
	bytes *int64
}

func (c countingFile) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.f.ReadAt(p, off)
	*c.bytes += int64(n)
	return n, err
}
func (c countingFile) Close() error { return c.f.Close() }

func withCountingOpener(t *testing.T) (bytes, opens *int64) {
	t.Helper()
	bytes, opens = new(int64), new(int64)
	orig := openFile
	openFile = func(path string) (readerAtCloser, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		*opens++
		return countingFile{f: f, bytes: bytes}, nil
	}
	t.Cleanup(func() { openFile = orig })
	return bytes, opens
}

func TestParseSessionFileReadsBoundedBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.jsonl")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	head := `{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-11-21T10:00:00.000Z","cwd":"/big/proj","sessionId":"` + basicID + `","gitBranch":"dev","turnOrigin":"human"}` + "\n"
	io.WriteString(f, head)
	filler := `{"type":"attachment","data":"` + strings.Repeat("z", 900) + `","timestamp":"2026-11-21T11:00:00.000Z"}` + "\n"
	for written := 0; written < 5<<20; written += len(filler) {
		io.WriteString(f, filler)
	}
	tail := `{"type":"assistant","message":{"model":"claude-sonnet-5"},"timestamp":"2026-11-21T12:00:00.000Z","cwd":"/big/proj"}` + "\n" +
		`{"type":"custom-title","customTitle":"Big one"}` + "\n" +
		`{"type":"last-prompt","lastPrompt":"final"}` + "\n" +
		`{"type":"cost-state","totalCostUSD":7.5}` + "\n"
	io.WriteString(f, tail)
	f.Close()

	bytes, _ := withCountingOpener(t)
	s, err := ParseSessionFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if *bytes > 320<<10 {
		t.Errorf("read %d bytes, want <= %d", *bytes, 320<<10)
	}
	if !s.Truncated {
		t.Error("Truncated should be true")
	}
	if s.Dir != "/big/proj" || s.Branch != "dev" || s.Title != "Big one" || s.LastPrompt != "final" ||
		s.Model != "sonnet-5" || s.CostUSD != 7.5 || s.UserMessages != 1 {
		t.Errorf("unexpected parse: %+v", s)
	}
	if want := mustTime(t, "2026-11-21T10:00:00Z"); !s.Started.Equal(want) {
		t.Errorf("Started = %v", s.Started)
	}
	if want := mustTime(t, "2026-11-21T12:00:00Z"); !s.LastActive.Equal(want) {
		t.Errorf("LastActive = %v", s.LastActive)
	}
	if s.ID != basicID {
		t.Errorf("ID = %q", s.ID)
	}
}

func TestScanHistorySkipsOldEmptyAndSubdirs(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-x-my-proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(proj, "good.jsonl")
	copyFixture(t, "basic.jsonl", good)
	old := filepath.Join(proj, "old.jsonl")
	copyFixture(t, "basic.jsonl", old)
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(old, oldTime, oldTime)
	empty := filepath.Join(proj, "empty.jsonl")
	os.WriteFile(empty, []byte(`{"type":"mode","mode":"default"}`+"\n"), 0o644)
	copyFixture(t, "basic.jsonl", filepath.Join(proj, "sub", "nested.jsonl"))
	os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("x"), 0o644)

	got, err := ScanHistory(root, time.Now().Add(-14*24*time.Hour), NewHistoryCache())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != good {
		t.Fatalf("got %+v", got)
	}
	if got[0].ID != basicID {
		t.Errorf("ID = %q", got[0].ID)
	}
}

func TestParseSessionFileIDFallsBackToFilename(t *testing.T) {
	p := filepath.Join(t.TempDir(), "abc-123.jsonl")
	os.WriteFile(p, []byte(`{"type":"last-prompt","lastPrompt":"hi"}`+"\n"), 0o644)
	s, err := ParseSessionFile(p)
	if err != nil || s.ID != "abc-123" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestScanHistoryMissingDir(t *testing.T) {
	got, err := ScanHistory(filepath.Join(t.TempDir(), "nope"), time.Time{}, NewHistoryCache())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestHistoryCacheReusesUnchangedFile(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "p")
	os.MkdirAll(proj, 0o755)
	a := filepath.Join(proj, "a.jsonl")
	copyFixture(t, "basic.jsonl", a)
	_, opens := withCountingOpener(t)
	cache := NewHistoryCache()

	first, err := ScanHistory(root, time.Time{}, cache)
	if err != nil || len(first) != 1 {
		t.Fatalf("%v %v", first, err)
	}
	if *opens != 1 {
		t.Fatalf("opens after first scan = %d", *opens)
	}
	second, _ := ScanHistory(root, time.Time{}, cache)
	if *opens != 1 || len(second) != 1 {
		t.Fatalf("second scan reopened: opens=%d", *opens)
	}

	// Changing the file invalidates the entry.
	f, _ := os.OpenFile(a, os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, `{"type":"last-prompt","lastPrompt":"changed"}`)
	f.Close()
	third, _ := ScanHistory(root, time.Time{}, cache)
	if *opens != 2 || third[0].LastPrompt != "changed" {
		t.Fatalf("opens=%d prompt=%q", *opens, third[0].LastPrompt)
	}

	// Removed files are pruned from the cache.
	os.Remove(a)
	ScanHistory(root, time.Time{}, cache)
	if len(cache.entries) != 0 {
		t.Errorf("cache not pruned: %d", len(cache.entries))
	}
}

func TestParseSessionFileSkipsSyntheticModel(t *testing.T) {
	p := filepath.Join(t.TempDir(), basicID+".jsonl")
	body := `{"type":"user","message":{"role":"user","content":"hi"},"timestamp":"2026-11-21T13:40:54.043Z","cwd":"/x","sessionId":"` + basicID + `","turnOrigin":"human"}
{"type":"assistant","message":{"model":"claude-opus-5-5","role":"assistant"},"timestamp":"2026-11-21T13:41:00.000Z","cwd":"/x","sessionId":"` + basicID + `"}
{"type":"assistant","message":{"model":"<synthetic>","role":"assistant"},"timestamp":"2026-11-21T13:42:00.000Z","cwd":"/x","sessionId":"` + basicID + `"}
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := ParseSessionFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "opus-5-5" {
		t.Errorf("Model = %q; want opus-5-5", s.Model)
	}
}
