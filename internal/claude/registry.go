package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stardust1420/starfield/internal/procs"
)

// LiveSession is a running Claude Code process from ~/.claude/sessions.
type LiveSession struct {
	PID                          int
	SessionID, Dir, Name, Status string
	StartedAt                    time.Time
}

type registryFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"` // epoch milliseconds
	Name      string `json:"name"`
	Status    string `json:"status"`
	ProcStart string `json:"procStart"` // ps lstart format, e.g. "Sun Nov 22 11:28:31 2026"
	Kind      string `json:"kind"`      // "interactive" for a terminal session
}

// ReadRegistry lists live interactive sessions (kind "interactive" or unset;
// `claude -p` and SDK children are skipped) whose process is still alive in
// table: same PID and, when both sides know it, the same start time
// (procs.SameStart: procStart is UTC, ps prints local time), else a
// claude (or node) process. A missing directory yields (nil, nil);
// unreadable or malformed files are skipped.
func ReadRegistry(sessionsDir string, table procs.Table) ([]LiveSession, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []LiveSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
		if err != nil {
			continue
		}
		var rf registryFile
		if json.Unmarshal(data, &rf) != nil || rf.PID == 0 {
			continue
		}
		if rf.Kind != "" && rf.Kind != "interactive" {
			continue
		}
		if !alive(rf, table) {
			continue
		}
		out = append(out, LiveSession{
			PID: rf.PID, SessionID: rf.SessionID, Dir: rf.Cwd,
			Name: rf.Name, Status: rf.Status,
			StartedAt: time.UnixMilli(rf.StartedAt),
		})
	}
	return out, nil
}

// alive reports whether the registry entry's process is the one in table.
// The start time rules out a PID reused by another process; without it,
// the command name has to look like Claude.
func alive(rf registryFile, table procs.Table) bool {
	p, ok := table[rf.PID]
	if !ok {
		return false
	}
	if rf.ProcStart != "" && p.Start != "" {
		return procs.SameStart(rf.ProcStart, p.Start)
	}
	return table.IsClaude(rf.PID)
}
