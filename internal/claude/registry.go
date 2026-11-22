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
}

// ReadRegistry lists live sessions, keeping only those whose PID is a claude
// process in table. A missing directory yields (nil, nil); unreadable or
// malformed files are skipped.
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
		if !table.IsClaude(rf.PID) {
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
