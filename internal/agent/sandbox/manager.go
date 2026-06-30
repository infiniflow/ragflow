//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// manager.go is the Go port of `agent/sandbox/providers/manager.py`.
//
// Configuration source priority (matches the Python load order, see
// `agent/sandbox/client.py::_load_provider_from_settings`):
//
//  1. Admin-panel `sandbox.provider_type` + `sandbox.{provider_type}`
//     JSON config stored in the `system_settings` MySQL table. The
//     Go port reads this via `internal/dao.SystemSettingsDAO`.
//  2. SANDBOX_PROVIDER_TYPE env var — defaults to "self_managed".
//  3. SANDBOX_EXECUTOR_MANAGER_URL / AGENTRUN_* / LOCAL_* / SSH_*
//     / E2B_* env vars for the per-provider knobs. The
//     `xxxConfigFromEnv` helpers in each provider file build the
//     same config map the admin-panel JSON would, so the
//     `FromConfig` constructor is the single source of truth.
//
// Once initialized, the manager holds the active provider. There is
// at most one active provider at a time — same as Python, because
// sandbox configuration is global.

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ProviderManager is the Go equivalent of
// `agent/sandbox/providers/manager.py::ProviderManager`. It is
// goroutine-safe and lazily initialized.
type ProviderManager struct {
	mu       sync.RWMutex
	provider SandboxProvider
	loaded   bool
}

// globalManager is the package-level manager. Mirrors the Python
// `_provider_manager` global in `agent/sandbox/client.py`. Tests
// can use SetProvider to inject a custom provider.
var (
	globalManager     *ProviderManager
	globalManagerOnce sync.Once
)

// DefaultManager returns the process-wide provider manager, creating
// it on first use. The manager is created lazily so importing this
// package does not require any sandbox env vars to be set.
func DefaultManager() *ProviderManager {
	globalManagerOnce.Do(func() {
		globalManager = &ProviderManager{}
	})
	return globalManager
}

// SetProvider installs a provider directly, bypassing env-based
// initialization. Used by tests and by the boot path once admin-panel
// settings reading is wired.
func (m *ProviderManager) SetProvider(p SandboxProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = p
	m.loaded = true
}

// Provider returns the active provider. nil if not yet initialized.
func (m *ProviderManager) Provider() SandboxProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// IsConfigured reports whether a provider is loaded. Mirrors
// `ProviderManager.is_configured` on the Python side.
func (m *ProviderManager) IsConfigured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded && m.provider != nil
}

// Reset clears the manager. Used by reload paths and by tests.
func (m *ProviderManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = nil
	m.loaded = false
}

// InitFromEnv resolves the active provider type from
// SANDBOX_PROVIDER_TYPE, builds the matching provider, calls
// Initialize, and registers it. Subsequent calls are no-ops once a
// provider is loaded — callers wanting to pick up env changes must
// call Reset first.
//
// The returned error is suitable for surfacing in boot logs; the
// manager stays unconfigured when Initialize fails.
func (m *ProviderManager) InitFromEnv(ctx context.Context) error {
	m.mu.Lock()
	if m.loaded && m.provider != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	ptype := resolveProviderType()
	p, err := buildProvider(ptype)
	if err != nil {
		return fmt.Errorf("sandbox: build provider %q: %w", ptype, err)
	}
	if err := p.Initialize(ctx); err != nil {
		return fmt.Errorf("sandbox: initialize provider %q: %w", ptype, err)
	}
	m.SetProvider(p)
	return nil
}

// SystemSetting is the minimal row shape LoadFromSettings needs
// from the system_settings table. Aliased from the entity layer so
// the test fake matches the real DAO's return type.
type SystemSetting = entity.SystemSettings

// SettingsReader is the minimal DAO surface LoadFromSettings
// needs. Defining an interface (rather than depending on
// *dao.SystemSettingsDAO directly) makes the manager unit-testable
// without a real MySQL.
type SettingsReader interface {
	GetByName(name string) ([]entity.SystemSettings, error)
}

// LoadFromSettings resolves the active provider from the admin-panel
// `sandbox.provider_type` and `sandbox.{provider_type}` settings in
// the system_settings MySQL table. JSON-decodes the provider config
// and passes it to the provider's FromConfig constructor. Falls back
// to env-based init when:
//   - the reader returns no rows (the settings haven't been written);
//   - the reader returns an error (DB unreachable / table missing);
//   - the provider type is unknown.
//
// This matches the Python
// `agent/sandbox/client.py::_load_provider_from_settings` flow but
// reuses the provider's FromConfig path so env-driven and
// settings-driven init produce semantically identical providers.
// Subsequent calls are no-ops once a provider is loaded; use
// Reset + ReloadFromSettings to pick up admin-panel changes.
func (m *ProviderManager) LoadFromSettings(ctx context.Context) error {
	return m.LoadFromSettingsWithReader(ctx, dao.NewSystemSettingsDAO())
}

// LoadFromSettingsWithReader is the testable seam for
// LoadFromSettings. Production code calls LoadFromSettings (which
// uses the real *dao.SystemSettingsDAO); tests inject a fake
// SettingsReader.
func (m *ProviderManager) LoadFromSettingsWithReader(ctx context.Context, r SettingsReader) error {
	m.mu.Lock()
	if m.loaded && m.provider != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	ptype, cfg, err := loadSettingsConfig(r)
	if err != nil {
		// Soft fall back: settings missing / malformed / DB error
		// → use env defaults. This keeps boot resilient when the
		// admin panel hasn't been configured yet.
		return m.InitFromEnv(ctx)
	}

	p, err := buildProviderFromConfig(ptype, cfg)
	if err != nil {
		// Settings row references a provider type we don't ship
		// (e.g. legacy type, typo). Fall back to env so boot
		// proceeds with the operator's env config.
		return m.InitFromEnv(ctx)
	}
	if err != nil {
		return fmt.Errorf("sandbox: build provider %q from settings: %w", ptype, err)
	}
	if err := p.Initialize(ctx); err != nil {
		return fmt.Errorf("sandbox: initialize provider %q from settings: %w", ptype, err)
	}
	m.SetProvider(p)
	return nil
}

// ReloadFromSettings resets the manager and re-reads the admin-panel
// settings. Mirrors Python's `reload_provider()` in
// `agent/sandbox/client.py` — call after the operator updates the
// sandbox settings.
func (m *ProviderManager) ReloadFromSettings(ctx context.Context) error {
	return m.ReloadFromSettingsWithReader(ctx, dao.NewSystemSettingsDAO())
}

// ReloadFromSettingsWithReader is the testable seam for
// ReloadFromSettings.
func (m *ProviderManager) ReloadFromSettingsWithReader(ctx context.Context, r SettingsReader) error {
	m.Reset()
	return m.LoadFromSettingsWithReader(ctx, r)
}

// loadSettingsConfig reads `sandbox.provider_type` and the
// matching `sandbox.{type}` JSON config from MySQL. Returns
// (ProviderType, nil) when the settings table has no rows for
// these keys (caller falls back to env).
func loadSettingsConfig(r SettingsReader) (ProviderType, map[string]any, error) {
	rows, err := r.GetByName("sandbox.provider_type")
	if err != nil {
		return "", nil, err
	}
	if len(rows) == 0 {
		// No settings row at all → caller falls back to env.
		return "", nil, errSettingsNotConfigured
	}
	ptype := ProviderType(rows[0].Value)
	if ptype == "" {
		return "", nil, errSettingsNotConfigured
	}

	cfgRows, err := r.GetByName("sandbox." + string(ptype))
	if err != nil {
		return ptype, nil, err
	}
	if len(cfgRows) == 0 {
		// Provider type set, but no per-provider config row.
		// The caller will try the env-driven path for the same
		// type, which may also miss if env is unconfigured for
		// that type. Treat as "no settings" so the env fallback
		// is uniform.
		return ptype, nil, errSettingsNotConfigured
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(cfgRows[0].Value), &cfg); err != nil {
		// Malformed JSON: fall back to env rather than booting
		// with an empty config (which would build a provider with
		// zero values for every field).
		return ptype, nil, errSettingsMalformed
	}
	return ptype, cfg, nil
}

// errSettingsNotConfigured signals "the admin panel hasn't
// configured sandbox settings; the env path should run." Treated
// as a soft signal: the manager swallows this and calls
// InitFromEnv, so boot never fails just because no one has
// touched the admin panel.
var errSettingsNotConfigured = errors.New("sandbox: admin-panel settings not configured")

// errSettingsMalformed signals "the settings JSON couldn't be
// parsed; the env path should run." Same soft-failure handling
// as errSettingsNotConfigured.
var errSettingsMalformed = errors.New("sandbox: admin-panel settings JSON malformed")

// resolveProviderType reads SANDBOX_PROVIDER_TYPE. Defaults to
// "self_managed" to match the Python
// `_load_provider_from_settings` default.
func resolveProviderType() ProviderType {
	if v := os.Getenv("SANDBOX_PROVIDER_TYPE"); v != "" {
		return ProviderType(v)
	}
	return ProviderSelfManaged
}

// buildProvider constructs a provider by type. Adding a new provider
// is a single switch case here. E2B returns ErrE2BProviderNotImplemented
// from every operation, but we still construct the provider so the
// manager can report the configured type to health checks.
func buildProvider(t ProviderType) (SandboxProvider, error) {
	switch t {
	case ProviderSelfManaged:
		return newSelfManagedProviderFromEnv(), nil
	case ProviderAliyun:
		return newAliyunProviderFromEnv(), nil
	case ProviderE2B:
		return newE2BProviderFromEnv(), nil
	case ProviderLocal:
		return newLocalProviderFromEnv(), nil
	case ProviderSSH:
		return newSSHProviderFromEnv(), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q (known: self_managed, aliyun_codeinterpreter, e2b, local, ssh)", t)
	}
}

// buildProviderFromConfig is the settings-driven counterpart of
// buildProvider. The config map keys mirror the env-var names
// without the per-provider prefix (e.g. SANDBOX_EXECUTOR_MANAGER_URL
// on env == "EXECUTOR_MANAGER_URL" in the settings JSON).
func buildProviderFromConfig(t ProviderType, cfg map[string]any) (SandboxProvider, error) {
	switch t {
	case ProviderSelfManaged:
		return newSelfManagedProviderFromConfig(cfg), nil
	case ProviderAliyun:
		return newAliyunProviderFromConfig(cfg), nil
	case ProviderE2B:
		return newE2BProviderFromConfig(cfg), nil
	case ProviderLocal:
		return newLocalProviderFromConfig(cfg), nil
	case ProviderSSH:
		return newSSHProviderFromConfig(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q (known: self_managed, aliyun_codeinterpreter, e2b, local, ssh)", t)
	}
}
