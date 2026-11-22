package dirs

import (
	"sort"
	"strings"
	"unicode"
)

// Match is one path that matched a query.
type Match struct {
	Index, Score int
	Positions    []int // byte offsets of the matched characters
}

type rch struct {
	r    rune
	off  int
	base bool
}

// Fuzzy filters paths by a case-insensitive subsequence query, best first.
// An empty query returns every path in order.
func Fuzzy(query string, paths []string) []Match {
	if query == "" {
		out := make([]Match, len(paths))
		for i := range paths {
			out[i] = Match{Index: i}
		}
		return out
	}
	q := []rune(strings.ToLower(query))
	var out []Match
	buf := make([]rch, 0, 128)
	for i, p := range paths {
		buf = buf[:0]
		baseOff := strings.LastIndexByte(p, '/') + 1
		for off, r := range p {
			buf = append(buf, rch{unicode.ToLower(r), off, off >= baseOff})
		}
		best, ok := Match{}, false
		if pos, found := match(q, buf, 0); found {
			best, ok = Match{Index: i, Score: score(buf, pos), Positions: offsets(buf, pos)}, true
		}
		{
			// Second pass anchored at the basename start.
			start := len(buf)
			for k, c := range buf {
				if c.base {
					start = k
					break
				}
			}
			if start > 0 && start < len(buf) {
				if pos, found := match(q, buf, start); found {
					if s := score(buf, pos); !ok || s > best.Score {
						best, ok = Match{Index: i, Score: s, Positions: offsets(buf, pos)}, true
					}
				}
			}
		}
		if ok {
			out = append(out, best)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	return out
}

// match greedily finds q as a subsequence of buf[from:], returning rune indexes.
func match(q []rune, buf []rch, from int) ([]int, bool) {
	pos := make([]int, 0, len(q))
	qi := 0
	for k := from; k < len(buf) && qi < len(q); k++ {
		if buf[k].r == q[qi] {
			pos = append(pos, k)
			qi++
		}
	}
	return pos, qi == len(q)
}

func score(buf []rch, pos []int) int {
	s := 0
	for i, k := range pos {
		if buf[k].base {
			s += 10
		}
		if i > 0 {
			if gap := k - pos[i-1] - 1; gap == 0 {
				s += 5
			} else {
				s -= gap
			}
		}
		if k > 0 {
			switch buf[k-1].r {
			case '/', '-', '_', '.':
				s += 8
			}
		}
	}
	return s
}

func offsets(buf []rch, pos []int) []int {
	out := make([]int, len(pos))
	for i, k := range pos {
		out[i] = buf[k].off
	}
	return out
}
