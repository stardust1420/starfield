package src

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// countTicks swaps scheduleTick for a counter for the length of the test, so
// a test can see how many refresh timers the home screen has armed.
func countTicks(t *testing.T) *int {
	t.Helper()
	n := new(int)
	orig := scheduleTick
	scheduleTick = func(id int) tea.Cmd {
		*n++
		return nil
	}
	t.Cleanup(func() { scheduleTick = orig })
	return n
}

// discoveryOf runs cmd and returns the discovery result it produces, so the
// test decides when that result reaches the model.
func discoveryOf(t *testing.T, cmd tea.Cmd) discoveredMsg {
	t.Helper()
	for _, msg := range runAll(cmd) {
		if d, ok := msg.(discoveredMsg); ok {
			return d
		}
	}
	t.Fatal("no discovery was started")
	return discoveredMsg{}
}

func TestStopRefreshDoesNotStartSecondTickLoop(t *testing.T) {
	ticks := countTicks(t)
	m := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)

	// The regular loop: a tick starts a discovery, its result arms the next tick.
	next, cmd := m.Update(tickMsg{id: m.tickID})
	m = next.(homeModel)
	next, _ = m.Update(discoveryOf(t, cmd))
	m = next.(homeModel)
	if *ticks != 1 {
		t.Fatalf("after one tick round, %d timers armed; want 1", *ticks)
	}

	// A stop finishes while that timer is pending: its refresh must not arm
	// a second timer.
	next, cmd = m.Update(actionMsg{})
	m = next.(homeModel)
	next, _ = m.Update(discoveryOf(t, cmd))
	m = next.(homeModel)
	if *ticks != 1 {
		t.Errorf("after a stop's refresh, %d timers armed; want still 1", *ticks)
	}

	// Same for returning from an attached session.
	next, cmd = m.Update(attachDoneMsg{name: "x"})
	m = next.(homeModel)
	next, _ = m.Update(discoveryOf(t, cmd))
	_ = next
	if *ticks != 1 {
		t.Errorf("after an attach's refresh, %d timers armed; want still 1", *ticks)
	}
}

func TestTickDuringRefreshKeepsTheLoop(t *testing.T) {
	ticks := countTicks(t)
	m := InitHomeModel(Deps{Cfg: testCfg()}, 100, 30)

	// A refresh is running when the timer fires: the tick can't start a
	// second discovery, but it must not let the loop die either.
	next, cmd := m.Update(attachDoneMsg{name: "x"})
	m = next.(homeModel)
	refresh := discoveryOf(t, cmd)

	next, cmd = m.Update(tickMsg{id: m.tickID})
	m = next.(homeModel)
	for _, msg := range runAll(cmd) {
		if _, ok := msg.(discoveredMsg); ok {
			t.Fatal("a tick during a refresh started a second discovery")
		}
	}
	if *ticks != 1 {
		t.Fatalf("a tick during a refresh armed %d timers; want 1 (rescheduled)", *ticks)
	}

	next, _ = m.Update(refresh)
	_ = next
	if *ticks != 1 {
		t.Errorf("the refresh result armed another timer: %d; want 1", *ticks)
	}
}
