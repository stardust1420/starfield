// Package claude reads Claude Code's on-disk state: session history under
// ~/.claude/projects and the live session registry.
package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	headWindow = 64 << 10
	tailWindow = 256 << 10
)

// HistorySession is one Claude session found in the history files.
type HistorySession struct {
	ID, Dir, Path, Title, LastPrompt, Branch, Model string
	CostUSD                                         float64
	UserMessages                                    int // lower bound when Truncated
	Started, LastActive                             time.Time
	// Truncated is true when the file was larger than the head and tail
	// windows, so the middle was not read.
	Truncated bool
}

type readerAtCloser interface {
	io.ReaderAt
	io.Closer
}

// openFile is replaceable so tests can count opens and bytes read.
var openFile = func(path string) (readerAtCloser, error) { return os.Open(path) }

type record struct {
	Type        string  `json:"type"`
	Timestamp   string  `json:"timestamp"`
	Cwd         string  `json:"cwd"`
	SessionID   string  `json:"sessionId"`
	GitBranch   string  `json:"gitBranch"`
	TurnOrigin  string  `json:"turnOrigin"`
	CustomTitle string  `json:"customTitle"`
	LastPrompt  string  `json:"lastPrompt"`
	TotalCost   float64 `json:"totalCostUSD"`
	Message     struct {
		Model string `json:"model"`
	} `json:"message"`
}

// ParseSessionFile reads the head and tail of one session file.
func ParseSessionFile(path string) (HistorySession, error) {
	f, err := openFile(path)
	if err != nil {
		return HistorySession{}, err
	}
	defer f.Close()
	st, err := os.Stat(path)
	if err != nil {
		return HistorySession{}, err
	}
	size := st.Size()

	var chunks [][]byte
	truncated := size > headWindow+tailWindow
	if !truncated {
		buf := make([]byte, size)
		n, err := f.ReadAt(buf, 0)
		if err != nil && err != io.EOF {
			return HistorySession{}, err
		}
		chunks = [][]byte{buf[:n]}
	} else {
		head := make([]byte, headWindow)
		n, err := f.ReadAt(head, 0)
		if err != nil && err != io.EOF {
			return HistorySession{}, err
		}
		head = head[:n]
		// The window cuts a line: drop the trailing partial line.
		if i := bytes.LastIndexByte(head, '\n'); i >= 0 {
			head = head[:i+1]
		}
		tail := make([]byte, tailWindow)
		n, err = f.ReadAt(tail, size-tailWindow)
		if err != nil && err != io.EOF {
			return HistorySession{}, err
		}
		tail = tail[:n]
		// The window starts mid-line: drop the leading partial line.
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
		chunks = [][]byte{head, tail}
	}

	s := HistorySession{Path: path, Truncated: truncated}
	var haveCwd bool
	for _, chunk := range chunks {
		for _, line := range bytes.Split(chunk, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var r record
			if json.Unmarshal(line, &r) != nil {
				continue
			}
			if s.ID == "" && r.SessionID != "" {
				s.ID = r.SessionID
			}
			if !haveCwd && r.Cwd != "" {
				haveCwd = true
				s.Dir = r.Cwd
				s.Branch = r.GitBranch
			}
			if r.Timestamp != "" {
				if ts, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
					if s.Started.IsZero() {
						s.Started = ts
					}
					s.LastActive = ts
				}
			}
			switch r.Type {
			case "user":
				if r.TurnOrigin == "human" {
					s.UserMessages++
				}
			case "assistant":
				if r.Message.Model != "" {
					s.Model = strings.TrimPrefix(r.Message.Model, "claude-")
				}
			case "custom-title":
				if r.CustomTitle != "" {
					s.Title = r.CustomTitle
				}
			case "last-prompt":
				if r.LastPrompt != "" {
					s.LastPrompt = r.LastPrompt
				}
			case "cost-state":
				s.CostUSD = r.TotalCost
			}
		}
	}
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if s.LastActive.IsZero() {
		s.LastActive = st.ModTime()
	}
	return s, nil
}

type cacheKey struct {
	path  string
	mtime int64
	size  int64
}

// HistoryCache remembers parsed sessions keyed by (path, mtime, size). It is
// not safe for concurrent use.
type HistoryCache struct {
	entries map[string]cacheEntry
}

type cacheEntry struct {
	key     cacheKey
	session HistorySession
}

func NewHistoryCache() *HistoryCache {
	return &HistoryCache{entries: map[string]cacheEntry{}}
}

// ScanHistory lists sessions under projectsDir (one level of project dirs,
// top-level *.jsonl only) modified at or after since. A missing projectsDir
// yields no sessions.
func ScanHistory(projectsDir string, since time.Time, cache *HistoryCache) ([]HistorySession, error) {
	if cache == nil {
		cache = NewHistoryCache()
	}
	dirs, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := map[string]bool{}
	var out []HistorySession
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		pdir := filepath.Join(projectsDir, d.Name())
		files, err := os.ReadDir(pdir)
		if err != nil {
			continue
		}
		for _, fe := range files {
			if fe.IsDir() || !strings.HasSuffix(fe.Name(), ".jsonl") {
				continue
			}
			info, err := fe.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			path := filepath.Join(pdir, fe.Name())
			seen[path] = true
			if info.ModTime().Before(since) {
				continue
			}
			key := cacheKey{path, info.ModTime().UnixNano(), info.Size()}
			var s HistorySession
			if e, ok := cache.entries[path]; ok && e.key == key {
				s = e.session
			} else {
				s, err = ParseSessionFile(path)
				if err != nil {
					continue
				}
				cache.entries[path] = cacheEntry{key, s}
			}
			if s.UserMessages == 0 && s.LastPrompt == "" {
				continue
			}
			out = append(out, s)
		}
	}
	for p := range cache.entries {
		if !seen[p] {
			delete(cache.entries, p)
		}
	}
	return out, nil
}
