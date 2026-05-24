package core

import (
	"encoding/json"
	"os"
	"sync"
)

type DynamicSettings struct {
	ModelAliases      map[string]string `json:"model_aliases,omitempty"`
	RtkEnabled        *bool             `json:"rtk_enabled,omitempty"`
	CavemanEnabled    *bool             `json:"caveman_enabled,omitempty"`
	CavemanLevel      string            `json:"caveman_level,omitempty"`
	MoeMailDomain     string            `json:"moemail_domain,omitempty"`
	MoeMailKey        string            `json:"moemail_key,omitempty"`
	TempMailDomain    string            `json:"tempmail_domain,omitempty"`
	TempMailKey       string            `json:"tempmail_key,omitempty"`
	ProxyEnabled      bool              `json:"proxy_enabled,omitempty"`
	ProxyURL          string            `json:"proxy_url,omitempty"`
	ProxyUsername     string            `json:"proxy_username,omitempty"`
	ProxyPassword     string            `json:"proxy_password,omitempty"`
	MaxInflightPerAcc int               `json:"max_inflight_per_account,omitempty"`
}

type SettingsManager struct {
	mu       sync.RWMutex
	path     string
	settings DynamicSettings
}

var GlobalSettingsManager *SettingsManager

func NewSettingsManager(path string) *SettingsManager {
	sm := &SettingsManager{path: path}
	sm.Load()
	GlobalSettingsManager = sm
	return sm
}

func (sm *SettingsManager) Load() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.path)
	if err != nil {
		if os.IsNotExist(err) {
			sm.settings = DynamicSettings{}
			sm.applyDefaults()
			return
		}
		sm.settings = DynamicSettings{}
		sm.applyDefaults()
		return
	}

	json.Unmarshal(data, &sm.settings)

	// Legacy migration: token_saver_mode / token_saver_enabled → rtk_enabled + caveman_enabled + caveman_level
	if sm.settings.RtkEnabled == nil && sm.settings.CavemanEnabled == nil {
		var legacy struct {
			TokenSaverMode    string `json:"token_saver_mode,omitempty"`
			TokenSaverEnabled *bool  `json:"token_saver_enabled,omitempty"`
		}
		json.Unmarshal(data, &legacy)
		mode := legacy.TokenSaverMode
		if mode == "" && legacy.TokenSaverEnabled != nil {
			if *legacy.TokenSaverEnabled {
				mode = "full"
			} else {
				mode = "off"
			}
		}
		if mode != "" {
			sm.migrateFromLegacyMode(mode)
			// Scrub legacy key from disk on next save (struct already excludes it).
			_ = sm.save()
			return
		}
	}

	sm.applyDefaults()
}

// applyDefaults ensures the saver fields have non-nil values when nothing was
// persisted yet. Defaults match 9router's EndpointPageClient.js initial state:
// rtk on, caveman off, level "full".
func (sm *SettingsManager) applyDefaults() {
	if sm.settings.RtkEnabled == nil {
		t := true
		sm.settings.RtkEnabled = &t
	}
	if sm.settings.CavemanEnabled == nil {
		f := false
		sm.settings.CavemanEnabled = &f
	}
	if sm.settings.CavemanLevel == "" {
		sm.settings.CavemanLevel = "full"
	}
}

// migrateFromLegacyMode maps the old 4-segment enum onto the new triple.
// off    → rtk off, caveman off, level ""
// lite   → rtk on,  caveman off, level "" (RTK-only baseline)
// full   → rtk on,  caveman on,  level "full"
// ultra  → rtk on,  caveman on,  level "ultra"
func (sm *SettingsManager) migrateFromLegacyMode(mode string) {
	t := true
	f := false
	switch mode {
	case "off":
		sm.settings.RtkEnabled = &f
		sm.settings.CavemanEnabled = &f
		sm.settings.CavemanLevel = ""
	case "lite":
		sm.settings.RtkEnabled = &t
		sm.settings.CavemanEnabled = &f
		sm.settings.CavemanLevel = ""
	case "full":
		sm.settings.RtkEnabled = &t
		sm.settings.CavemanEnabled = &t
		sm.settings.CavemanLevel = "full"
	case "ultra":
		sm.settings.RtkEnabled = &t
		sm.settings.CavemanEnabled = &t
		sm.settings.CavemanLevel = "ultra"
	default:
		sm.applyDefaults()
	}
}

func (sm *SettingsManager) save() error {
	data, err := json.MarshalIndent(sm.settings, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := sm.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, sm.path)
}

func (sm *SettingsManager) Get() DynamicSettings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.settings
}

// SaverFlags returns the three saver knobs with nil-pointers resolved — kept
// for handler callsites that don't want to deal with *bool dereferencing.
func (sm *SettingsManager) SaverFlags() (rtk bool, cav bool, level string) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.settings.RtkEnabled != nil {
		rtk = *sm.settings.RtkEnabled
	}
	if sm.settings.CavemanEnabled != nil {
		cav = *sm.settings.CavemanEnabled
	}
	level = sm.settings.CavemanLevel
	return
}

func (sm *SettingsManager) Update(updates map[string]interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if v, ok := updates["model_aliases"]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			aliases := make(map[string]string)
			for k, val := range m {
				if s, ok := val.(string); ok {
					aliases[k] = s
				}
			}
			sm.settings.ModelAliases = aliases
		}
	}
	if v, ok := updates["rtk_enabled"]; ok {
		if b, ok := v.(bool); ok {
			sm.settings.RtkEnabled = &b
		}
	}
	if v, ok := updates["caveman_enabled"]; ok {
		if b, ok := v.(bool); ok {
			sm.settings.CavemanEnabled = &b
		}
	}
	if v, ok := updates["caveman_level"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case "lite", "full", "ultra":
				sm.settings.CavemanLevel = s
			}
		}
	}
	// Legacy keys accepted for one release of overlap — translated, never written back.
	if v, ok := updates["token_saver_mode"]; ok {
		if s, ok := v.(string); ok {
			sm.migrateFromLegacyMode(s)
		}
	}
	if v, ok := updates["token_saver_enabled"]; ok {
		if b, ok := v.(bool); ok {
			if b {
				sm.migrateFromLegacyMode("full")
			} else {
				sm.migrateFromLegacyMode("off")
			}
		}
	}
	if v, ok := updates["moemail_domain"]; ok {
		sm.settings.MoeMailDomain, _ = v.(string)
	}
	if v, ok := updates["moemail_key"]; ok {
		sm.settings.MoeMailKey, _ = v.(string)
	}
	if v, ok := updates["tempmail_domain"]; ok {
		sm.settings.TempMailDomain, _ = v.(string)
	}
	if v, ok := updates["tempmail_key"]; ok {
		sm.settings.TempMailKey, _ = v.(string)
	}
	if v, ok := updates["proxy_enabled"]; ok {
		if b, ok := v.(bool); ok {
			sm.settings.ProxyEnabled = b
		}
	}
	if v, ok := updates["proxy_url"]; ok {
		sm.settings.ProxyURL, _ = v.(string)
	}
	if v, ok := updates["proxy_username"]; ok {
		sm.settings.ProxyUsername, _ = v.(string)
	}
	if v, ok := updates["proxy_password"]; ok {
		sm.settings.ProxyPassword, _ = v.(string)
	}
	if v, ok := updates["max_inflight_per_account"]; ok {
		if f, ok := v.(float64); ok {
			sm.settings.MaxInflightPerAcc = int(f)
		}
	}
	if v, ok := updates["engine_mode"]; ok {
		if s, ok := v.(string); ok {
			if GlobalConfig != nil {
				GlobalConfig.EngineMode = s
			}
		}
	}
	if v, ok := updates["admin_key"]; ok {
		if s, ok := v.(string); ok {
			if GlobalConfig != nil {
				GlobalConfig.AdminKey = s
			}
		}
	}

	return sm.save()
}
