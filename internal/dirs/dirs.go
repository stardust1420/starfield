// Package dirs finds candidate working directories for new sessions and
// fuzzy-filters them.
package dirs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stardust1420/starfield/internal/claude"
)

// Source says where a candidate came from.
type Source int

const (
	Recent  Source = iota // used by a previous Claude session
	Scanned               // found under a configured root
	Typed                 // entered by hand
)

// Candidate is one directory the picker can offer.
type Candidate struct {
	Path     string
	Source   Source
	LastUsed time.Time
}

// Scan lists git repos (and direct children) under root, up to depth levels
// below it. The result is sorted; a missing root gives nil.
func Scan(root string, depth int) []string {
	var out []string
	scan(root, 1, depth, &out)
	sort.Strings(out)
	return out
}

func scan(dir string, level, depth int, out *[]string) {
	if level > depth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		// Symlinks report ModeSymlink, so IsDir is false and they are skipped.
		if !e.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			*out = append(*out, p)
			continue
		}
		if level == 1 {
			*out = append(*out, p)
		}
		scan(p, level+1, depth, out)
	}
}

// Candidates returns recent session directories (newest first), then scanned
// ones not already listed. Only existing directories are included.
func Candidates(hist []claude.HistorySession, roots []string, depth int) []Candidate {
	return candidates(hist, roots, depth, isDir)
}

// Recents is the recent part of Candidates without checking each directory
// still exists, so it never touches the disk: a list to show at once while
// Candidates runs.
func Recents(hist []claude.HistorySession) []Candidate {
	return candidates(hist, nil, 0, func(string) bool { return true })
}

func candidates(hist []claude.HistorySession, roots []string, depth int, exists func(string) bool) []Candidate {
	latest := map[string]time.Time{}
	for _, h := range hist {
		if h.Dir == "" {
			continue
		}
		d := filepath.Clean(h.Dir)
		if t, ok := latest[d]; !ok || h.LastActive.After(t) {
			latest[d] = h.LastActive
		}
	}
	recent := make([]Candidate, 0, len(latest))
	for d, t := range latest {
		if exists(d) {
			recent = append(recent, Candidate{Path: d, Source: Recent, LastUsed: t})
		}
	}
	sort.Slice(recent, func(i, j int) bool {
		if !recent[i].LastUsed.Equal(recent[j].LastUsed) {
			return recent[i].LastUsed.After(recent[j].LastUsed)
		}
		return recent[i].Path < recent[j].Path
	})
	seen := make(map[string]bool, len(recent))
	for _, c := range recent {
		seen[c.Path] = true
	}
	out := recent
	for _, r := range roots {
		for _, p := range Scan(r, depth) {
			p = filepath.Clean(p)
			if seen[p] || !isDir(p) {
				continue
			}
			seen[p] = true
			out = append(out, Candidate{Path: p, Source: Scanned})
		}
	}
	return out
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Expand replaces a leading ~ or ~/ with home. ~user is left alone.
func Expand(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
