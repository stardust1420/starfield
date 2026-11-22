package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Settings are the choices made inside the app, saved to
// <StateDir>/settings.json so they outlast a restart. A nil field was never
// set; the environment or the default applies then.
type Settings struct {
	// HistoryDays is the window for stopped sessions; 0 means all time.
	HistoryDays *int `json:"history_days,omitempty"`
}

const settingsFile = "settings.json"

// Int returns a pointer to n, for Settings fields.
func Int(n int) *int { return &n }

// LoadSettings reads the saved settings. A missing file is no error. A file
// that can't be read or parsed returns empty Settings with the error, so the
// caller can carry on with the defaults; an out-of-range value is dropped.
func LoadSettings(stateDir string) (Settings, error) {
	b, err := os.ReadFile(filepath.Join(stateDir, settingsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	if s.HistoryDays != nil && *s.HistoryDays < 0 {
		s.HistoryDays = nil
	}
	return s, nil
}

// SaveSettings writes s, replacing the file in one rename so a crash never
// leaves it half-written.
func SaveSettings(stateDir string, s Settings) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(stateDir, settingsFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(stateDir, settingsFile))
}

// Apply returns c with the saved settings on top: a value chosen in the app
// wins over STARFIELD_HISTORY_DAYS, which only sets the starting value.
func (c Config) Apply(s Settings) Config {
	if s.HistoryDays != nil {
		c.HistoryDays = *s.HistoryDays
	}
	return c
}
