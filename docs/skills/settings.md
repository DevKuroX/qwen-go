# Skill: Dynamic settings

## When to use
- Adding a runtime-tunable knob (toggle, level, string config)
- Migrating a legacy setting shape
- Wiring a setting through to providers / handlers
- Adding the setting to the dashboard Settings page

## Mental model
`SettingsManager` is a singleton (`core.GlobalSettingsManager`) that loads `data/settings.json` once at boot and exposes a thread-safe `Get()` / `Update()` API. The struct uses pointers (`*bool`) for fields that need to distinguish "unset" from "explicitly false" during migration. Defaults applied at `Load` time after migration. **Disk is authoritative** — Update writes through. Migrations run on Load, scrub legacy keys, immediate save.

## Files

| Symbol | File |
|--------|------|
| `DynamicSettings` struct | `backend/internal/core/settings_manager.go:9-23` |
| `SettingsManager`, `GlobalSettingsManager`, `NewSettingsManager` | same file |
| `Load`, `applyDefaults`, `migrateFromLegacyMode` | same file |
| `save`, `Get`, `Update`, `SaverFlags` | same file |
| Settings file | `data/settings.json` |
| Admin GET/PUT | `backend/internal/api/admin.go` (search `settings`) |
| Frontend page | `frontend/app/(admin)/dashboard/settings/page.tsx` |

## Public API

```go
// Read — returns copy, safe to use outside lock
s := core.GlobalSettingsManager.Get()
fmt.Println(s.CavemanLevel)

// Convenience for the saver triple (handles nil pointers)
rtk, cav, level := core.GlobalSettingsManager.SaverFlags()

// Write — partial update, persists immediately
core.GlobalSettingsManager.Update(map[string]interface{}{
    "caveman_level": "ultra",
    "rtk_enabled":   true,
})
```

## Current settings (reference)

| Key | Type | Purpose |
|-----|------|---------|
| `rtk_enabled` | `*bool` | Apply RTK tool-output compression |
| `caveman_enabled` | `*bool` | Inject Caveman terse prompt |
| `caveman_level` | string | `lite` / `full` / `ultra` |
| `moemail_domain`, `moemail_key` | string | MoeMail temp-email service for registration |
| `tempmail_domain`, `tempmail_key` | string | Alt temp-email service |
| `proxy_enabled`, `proxy_url`, `proxy_username`, `proxy_password` | proxy settings |
| `max_inflight_per_account` | int | Account pool concurrency cap |
| `model_aliases` | map[string]string | Legacy runtime alias map (preferred: per-provider `model_aliases` in `provider_configs.json`) |

## Recipe — add a new setting

1. **Struct field** on `DynamicSettings` with `json` tag (and `omitempty` if optional):
   ```go
   MyToggle *bool `json:"my_toggle,omitempty"`
   ```
2. **Default** in `applyDefaults`:
   ```go
   if sm.settings.MyToggle == nil {
       f := false
       sm.settings.MyToggle = &f
   }
   ```
3. **Update branch** in `Update`:
   ```go
   case "my_toggle":
       if b, ok := v.(bool); ok { sm.settings.MyToggle = &b }
   ```
4. **Admin API** — `admin.go` GET/PUT settings: surface the field in the response shape; PUT branch translates it via `Update`
5. **Frontend** — `settings/page.tsx`: add UI control + `saveSetting('my_toggle', value)` call
6. **Consumer** — wherever you read it: `s := core.GlobalSettingsManager.Get(); if *s.MyToggle { ... }`

## Recipe — migrate a legacy field

Pattern (already used for `token_saver_mode` → `rtk_enabled` + `caveman_enabled` + `caveman_level`):

```go
// In Load(), after json.Unmarshal of the main struct:
if sm.settings.NewField == nil {  // unset signal
    var legacy struct { OldField string `json:"old_field,omitempty"` }
    json.Unmarshal(data, &legacy)
    if legacy.OldField != "" {
        sm.migrateFromLegacy(legacy.OldField)
        sm.save()  // scrub legacy key from disk
        return     // skip applyDefaults; migration set the values
    }
}
sm.applyDefaults()
```

The new struct **must not** declare the legacy field — `save()` won't serialize what isn't in the struct, which is how the scrub works.

## Invariants — DO NOT BREAK

1. **`Load` is called exactly once** by `NewSettingsManager`. Don't call from outside
2. **Pointer-bool fields distinguish unset vs false** — losing this breaks migration. Don't switch to plain `bool`
3. **`applyDefaults` runs AFTER migration** — so migration sets explicit values, defaults only fill what migration didn't touch
4. **`Update` persists to disk** — every call hits the filesystem. Don't loop Update in hot paths; batch via direct struct mutation under `sm.mu.Lock()` + final `sm.save()` if you must
5. **`SettingsManager.mu` protects all reads/writes** — `Get()` copies the struct out under the lock; consumers get a snapshot

## Common edits

- **Surface in dashboard**: bump frontend Settings page + `admin.go` GET shape + Update branch
- **Read from a provider**: `core.GlobalSettingsManager.Get().FieldName` — singleton, always available after server.Run
- **Gate a handler behind a setting**: read at handler entry, early-return with clear error if disabled

## Gotchas

- **Disk → memory only at boot or explicit Load** — editing `data/settings.json` while the server runs has zero effect until restart. Use the admin PUT endpoint or the dashboard
- **`omitempty` on `*bool`** — `nil` is treated as unset and omitted from JSON. This is how the migration probe works (`if sm.settings.RtkEnabled == nil`). Don't add `omitempty` to fields where `nil` and `false` should disk-encode the same way
- **`map[string]interface{}` in `Update`** — when reading a bool from JSON in Go via `interface{}`, type assertion is `bool`; for a number it's `float64`. Branch accordingly
- **Legacy keys lingering** — if you add a migration, verify the next `save()` actually drops the legacy key (struct must not have it as a field). Test with: write old-shape JSON → Load → re-read disk → assert legacy key absent
- **Frontend reads via admin GET** — don't bypass the API and read settings.json directly from the frontend; it's not served as a static file

## Cross-skill

- [api-endpoint](api-endpoint.md) — admin GET/PUT for settings live in admin.go
- [provider-add](provider-add.md) — providers consume settings via the singleton
- [account-pool](account-pool.md) — pool reads `max_inflight_per_account` from here
