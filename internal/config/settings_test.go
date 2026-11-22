package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "starfield")
	if err := SaveSettings(dir, Settings{HistoryDays: Int(7)}); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(dir)
	if err != nil || s.HistoryDays == nil || *s.HistoryDays != 7 {
		t.Fatalf("LoadSettings = %+v, %v; want 7", s, err)
	}
	// 0 means all time and must survive the round trip.
	if err := SaveSettings(dir, Settings{HistoryDays: Int(0)}); err != nil {
		t.Fatal(err)
	}
	if s, _ := LoadSettings(dir); s.HistoryDays == nil || *s.HistoryDays != 0 {
		t.Errorf("all time didn't round-trip: %+v", s)
	}
}

func TestLoadSettingsMissingOrBad(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadSettings(dir); err != nil || s.HistoryDays != nil {
		t.Errorf("missing file: %+v, %v; want empty, no error", s, err)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{nope"), 0o644)
	if s, err := LoadSettings(dir); err == nil || s.HistoryDays != nil {
		t.Errorf("corrupt file: %+v, %v; want empty with an error", s, err)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"history_days": -3}`), 0o644)
	if s, _ := LoadSettings(dir); s.HistoryDays != nil {
		t.Errorf("negative days accepted: %+v", s)
	}
}

func TestApplySettingsOverridesHistoryDays(t *testing.T) {
	c := Load(func(string) string { return "" }, "/home/u")
	c = c.Apply(Settings{HistoryDays: Int(0)})
	if c.HistoryDays != 0 {
		t.Errorf("HistoryDays = %d; want 0 (all time) from settings", c.HistoryDays)
	}
	if c = Load(func(string) string { return "" }, "/home/u").Apply(Settings{}); c.HistoryDays != 14 {
		t.Errorf("no saved value: HistoryDays = %d; want the default 14", c.HistoryDays)
	}
}
