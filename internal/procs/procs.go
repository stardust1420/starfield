// Package procs reads and queries the system process table.
package procs

import (
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// Proc is one row of the process table. Start is ps's lstart with runs of
// spaces collapsed (e.g. "Mon Nov 2 09:01:02 2026"), used to tell a process
// from a later one that reused its PID.
type Proc struct {
	PID, PPID int
	Comm      string
	Start     string
}

// Table maps PID to its process.
type Table map[int]Proc

// ParsePS parses `ps -axo pid=,ppid=,lstart=,comm=` output: two integers,
// the five whitespace-separated lstart fields, then the command (which may
// contain spaces). Lines that don't fit are skipped.
func ParsePS(out string) Table {
	t := Table{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		// The command is the rest of the line after the 7th field, spaces
		// inside it kept.
		rest := strings.TrimSpace(line)
		for i := 0; i < 7; i++ {
			_, rest, _ = cutField(rest)
		}
		comm := strings.TrimSpace(rest)
		if comm == "" {
			continue
		}
		t[pid] = Proc{PID: pid, PPID: ppid, Comm: comm, Start: strings.Join(f[2:7], " ")}
	}
	return t
}

// NormalizeStart collapses runs of whitespace in an lstart string, so ps's
// space-padded day ("Nov  2") compares equal to "Nov 2".
func NormalizeStart(s string) string { return strings.Join(strings.Fields(s), " ") }

const lstartLayout = "Mon Jan 2 15:04:05 2006"

// SameStart reports whether a start time recorded by Claude (procStart, in
// UTC) and ps's lstart (local time) name the same instant. Identical
// strings (after NormalizeStart) also match, for a UTC machine or a writer
// that used local time.
func SameStart(recorded, lstart string) bool {
	recorded, lstart = NormalizeStart(recorded), NormalizeStart(lstart)
	if recorded == lstart {
		return true
	}
	r, err1 := time.ParseInLocation(lstartLayout, recorded, time.UTC)
	l, err2 := time.ParseInLocation(lstartLayout, lstart, time.Local)
	return err1 == nil && err2 == nil && r.Equal(l)
}

func cutField(s string) (field, rest string, ok bool) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return "", "", false
	}
	return s[:i], strings.TrimLeft(s[i:], " \t"), true
}

// psCommand is the ps run by Snapshot. LC_ALL=C keeps lstart in English:
// Claude records procStart that way, and SameStart can't match a start
// time printed in another language.
func psCommand() *exec.Cmd {
	cmd := exec.Command("ps", "-axo", "pid=,ppid=,lstart=,comm=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

// Snapshot runs ps and returns the current process table.
func Snapshot() (Table, error) {
	out, err := psCommand().Output()
	if err != nil {
		return nil, err
	}
	return ParsePS(string(out)), nil
}

// IsAncestor reports whether anc is a (strict) ancestor of pid, walking at
// most 64 hops so a cyclic table terminates.
func (t Table) IsAncestor(anc, pid int) bool {
	cur := pid
	for i := 0; i < 64; i++ {
		p, ok := t[cur]
		if !ok {
			return false
		}
		if p.PPID == anc {
			return true
		}
		cur = p.PPID
	}
	return false
}

// Depth returns how many parent hops lead from pid up to anc: 0 when they
// are the same process, -1 when anc isn't an ancestor (at most 64 hops).
func (t Table) Depth(anc, pid int) int {
	if anc == pid {
		return 0
	}
	cur := pid
	for i := 1; i <= 64; i++ {
		p, ok := t[cur]
		if !ok {
			return -1
		}
		if p.PPID == anc {
			return i
		}
		cur = p.PPID
	}
	return -1
}

// IsClaude reports whether pid is a claude process: the native binary, or
// node for an npm install.
func (t Table) IsClaude(pid int) bool {
	p, ok := t[pid]
	if !ok {
		return false
	}
	base := path.Base(p.Comm)
	return base == "claude" || base == "node" || strings.Contains(p.Comm, "/claude")
}
