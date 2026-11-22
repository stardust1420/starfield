package sessions

import (
	"path/filepath"
	"time"

	"github.com/stardust1420/starfield/internal/claude"
	"github.com/stardust1420/starfield/internal/config"
	"github.com/stardust1420/starfield/internal/procs"
	"github.com/stardust1420/starfield/internal/tmux"
)

// Discoverer gathers every source and merges them. It owns a HistoryCache and
// is not safe for concurrent use: call Discover from one goroutine at a time
// (the UI runs one discovery at a time).
type Discoverer struct {
	cfg   config.Config
	cache *claude.HistoryCache

	// Sources, replaceable in tests.
	scanHistory  func(dir string, since time.Time, cache *claude.HistoryCache) ([]claude.HistorySession, error)
	snapshot     func() (procs.Table, error)
	readRegistry func(dir string, table procs.Table) ([]claude.LiveSession, error)
	listPanes    func() ([]tmux.Pane, error)
}

func NewDiscoverer(cfg config.Config, tm *tmux.Client) *Discoverer {
	d := &Discoverer{
		cfg:          cfg,
		cache:        claude.NewHistoryCache(),
		scanHistory:  claude.ScanHistory,
		snapshot:     procs.Snapshot,
		readRegistry: claude.ReadRegistry,
		listPanes:    func() ([]tmux.Pane, error) { return nil, nil },
	}
	if tm != nil {
		d.listPanes = tm.List
	}
	return d
}

// Discover lists stopped sessions from the configured window
// (cfg.HistoryDays); see DiscoverWithin.
func (d *Discoverer) Discover() ([]Session, error) {
	return d.DiscoverWithin(d.cfg.HistoryDays)
}

// DiscoverWithin never fails as a whole: if a source errors, the others are
// still merged and the first error is returned alongside the sessions.
// Stopped sessions are listed if active in the last days days; 0 means all
// of them. Running sessions are listed whatever their age.
func (d *Discoverer) DiscoverWithin(days int) ([]Session, error) {
	var first error
	note := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	var since time.Time // zero: everything
	if days > 0 {
		since = time.Now().AddDate(0, 0, -days)
	}
	hist, err := d.scanHistory(filepath.Join(d.cfg.ClaudeDir, "projects"), since, d.cache)
	note(err)
	table, err := d.snapshot()
	note(err)
	live, err := d.readRegistry(filepath.Join(d.cfg.ClaudeDir, "sessions"), table)
	note(err)
	panes, err := d.listPanes()
	note(err)
	return Merge(hist, live, panes, table), first
}
