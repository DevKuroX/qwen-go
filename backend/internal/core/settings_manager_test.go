package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeSettings(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	return path
}

func TestSettings_MigratesLegacyTokenSaverMode(t *testing.T) {
	cases := []struct {
		mode      string
		wantRtk   bool
		wantCav   bool
		wantLevel string
	}{
		{"off", false, false, ""},
		{"lite", true, false, ""},
		{"full", true, true, "full"},
		{"ultra", true, true, "ultra"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			path := writeSettings(t, `{"token_saver_mode":"`+c.mode+`"}`)
			sm := NewSettingsManager(path)
			rtk, cav, level := sm.SaverFlags()
			if rtk != c.wantRtk || cav != c.wantCav || level != c.wantLevel {
				t.Errorf("flags: rtk=%v cav=%v level=%q  want rtk=%v cav=%v level=%q",
					rtk, cav, level, c.wantRtk, c.wantCav, c.wantLevel)
			}
			// Disk should be scrubbed of the legacy key.
			raw, _ := os.ReadFile(path)
			var probe map[string]any
			_ = json.Unmarshal(raw, &probe)
			if _, has := probe["token_saver_mode"]; has {
				t.Errorf("legacy key not scrubbed from disk: %s", string(raw))
			}
		})
	}
}

func TestSettings_MigratesLegacyTokenSaverEnabled(t *testing.T) {
	path := writeSettings(t, `{"token_saver_enabled":true}`)
	sm := NewSettingsManager(path)
	rtk, cav, level := sm.SaverFlags()
	if !rtk || !cav || level != "full" {
		t.Errorf("expected legacy true → rtk+cav+full, got rtk=%v cav=%v level=%q", rtk, cav, level)
	}
}

func TestSettings_AppliesDefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	sm := NewSettingsManager(path)
	rtk, cav, level := sm.SaverFlags()
	if !rtk || cav || level != "full" {
		t.Errorf("expected defaults (rtk on, cav off, full), got rtk=%v cav=%v level=%q", rtk, cav, level)
	}
}

func TestSettings_AppliesDefaultsForEmptyJSON(t *testing.T) {
	path := writeSettings(t, `{}`)
	sm := NewSettingsManager(path)
	rtk, cav, level := sm.SaverFlags()
	if !rtk || cav || level != "full" {
		t.Errorf("expected defaults on empty JSON, got rtk=%v cav=%v level=%q", rtk, cav, level)
	}
}

func TestSettings_PreservesExplicitNewFields(t *testing.T) {
	path := writeSettings(t, `{"rtk_enabled":false,"caveman_enabled":true,"caveman_level":"ultra"}`)
	sm := NewSettingsManager(path)
	rtk, cav, level := sm.SaverFlags()
	if rtk || !cav || level != "ultra" {
		t.Errorf("expected explicit values preserved, got rtk=%v cav=%v level=%q", rtk, cav, level)
	}
}

func TestSettings_UpdateAcceptsNewKeys(t *testing.T) {
	path := writeSettings(t, `{}`)
	sm := NewSettingsManager(path)
	err := sm.Update(map[string]any{
		"rtk_enabled":     false,
		"caveman_enabled": true,
		"caveman_level":   "lite",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	rtk, cav, level := sm.SaverFlags()
	if rtk || !cav || level != "lite" {
		t.Errorf("after update: rtk=%v cav=%v level=%q", rtk, cav, level)
	}
}

func TestSettings_UpdateAcceptsLegacyMode(t *testing.T) {
	path := writeSettings(t, `{}`)
	sm := NewSettingsManager(path)
	if err := sm.Update(map[string]any{"token_saver_mode": "ultra"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	rtk, cav, level := sm.SaverFlags()
	if !rtk || !cav || level != "ultra" {
		t.Errorf("legacy mode update: rtk=%v cav=%v level=%q want true true ultra", rtk, cav, level)
	}
}

func TestSettings_UpdateRejectsUnknownLevel(t *testing.T) {
	path := writeSettings(t, `{"caveman_level":"full"}`)
	sm := NewSettingsManager(path)
	_ = sm.Update(map[string]any{"caveman_level": "bogus"})
	if got := sm.Get().CavemanLevel; got != "full" {
		t.Errorf("expected level kept at 'full' after bogus update, got %q", got)
	}
}
