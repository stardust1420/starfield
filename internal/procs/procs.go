// Package procs reads and queries the system process table.
package procs

import (
	"os/exec"
	"path"
	"strconv"
	"strings"
)

// Proc is one row of the process table.
type Proc struct {
	PID, PPID int
	Comm      string
}

// Table maps PID to its process.
type Table map[int]Proc

// ParsePS parses `ps -axo pid=,ppid=,comm=` output. Lines that don't start
// with two integers are skipped; the rest of the line is the command.
func ParsePS(out string) Table {
	t := Table{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		pidStr, rest, ok := cutField(line)
		if !ok {
			continue
		}
		ppidStr, comm, ok := cutField(rest)
		if !ok {
			continue
		}
		pid, err1 := strconv.Atoi(pidStr)
		ppid, err2 := strconv.Atoi(ppidStr)
		comm = strings.TrimSpace(comm)
		if err1 != nil || err2 != nil || comm == "" {
			continue
		}
		t[pid] = Proc{PID: pid, PPID: ppid, Comm: comm}
	}
	return t
}

func cutField(s string) (field, rest string, ok bool) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return "", "", false
	}
	return s[:i], strings.TrimLeft(s[i:], " \t"), true
}

// Snapshot runs ps and returns the current process table.
func Snapshot() (Table, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Output()
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

// IsClaude reports whether pid is a claude process.
func (t Table) IsClaude(pid int) bool {
	p, ok := t[pid]
	if !ok {
		return false
	}
	return path.Base(p.Comm) == "claude" || strings.Contains(p.Comm, "/claude")
}
