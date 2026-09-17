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
//     / E2B_* / UCLOUD_SANDBOX_* env vars for the per-provider knobs. The
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
	"ragflow/internal/common"
	"sync"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// ProviderManager is the Go equivalent of
// `agent/sandbox/providers/manager.py::ProviderManager`. It is
// goroutine-safe and lazily initialized.
type ProviderManager struct {
	refreshMu          sync.Mutex
	mu                 sync.RWMutex
	provider           SandboxProvider
	loaded             bool
	override           bool
	snapshot           string
	settingsConfigured bool
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

// SetProvider installs an explicit override, bypassing settings refresh.
func (m *ProviderManager) SetProvider(p SandboxProvider) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = p
	m.loaded = true
	m.override = true
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
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = nil
	m.loaded = false
	m.override = false
	m.snapshot = ""
	m.settingsConfigured = false
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
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	return m.initFromEnv(ctx)
}

func (m *ProviderManager) initFromEnv(ctx context.Context) error {
	if m.IsConfigured() {
		return nil
	}

	ptype := resolveProviderType()
	p, err := buildProvider(ptype)
	if err != nil {
		return fmt.Errorf("sandbox: build provider %q: %w", ptype, err)
	}
	if err := p.Initialize(ctx); err != nil {
		return fmt.Errorf("sandbox: initialize provider %q: %w", ptype, err)
	}
	m.mu.Lock()
	m.provider, m.loaded = p, true
	m.mu.Unlock()
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
	GetByNamePrefix(ctx context.Context, db *gorm.DB, prefix string) ([]entity.SystemSettings, error)
}

// LoadFromSettings resolves the active provider from the admin-panel
// settings snapshot before each execution. Only absent settings permit
// environment bootstrap; explicit overrides bypass settings entirely.
func (m *ProviderManager) LoadFromSettings(ctx context.Context) error {
	return m.LoadFromSettingsWithReader(ctx, dao.DB, dao.NewSystemSettingsDAO())
}

// LoadFromSettingsWithReader is the testable seam for
// LoadFromSettings. Production code calls LoadFromSettings (which
// uses the real *dao.SystemSettingsDAO); tests inject a fake
// SettingsReader.
func (m *ProviderManager) LoadFromSettingsWithReader(ctx context.Context, db *gorm.DB, r SettingsReader) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.override {
		return nil
	}

	ptype, cfg, err := loadSettingsConfig(ctx, db, r)
	if ptype != "" {
		m.settingsConfigured = true
	}
	if err != nil {
		if errors.Is(err, errSettingsNotConfigured) && !m.settingsConfigured {
			return m.initFromEnv(ctx)
		}
		return fmt.Errorf("sandbox: read settings: %w", err)
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("sandbox: encode settings: %w", err)
	}
	snapshot := string(ptype) + ":" + string(encoded)
	if snapshot == m.snapshot && m.IsConfigured() {
		return nil
	}

	p, err := buildProviderFromConfig(ptype, cfg)
	if err != nil {
		return fmt.Errorf("sandbox: build provider %q from settings: %w", ptype, err)
	}
	if err := p.Initialize(ctx); err != nil {
		return fmt.Errorf("sandbox: initialize provider %q from settings: %w", ptype, err)
	}
	m.mu.Lock()
	m.provider, m.loaded = p, true
	m.mu.Unlock()
	m.snapshot = snapshot
	return nil
}

// ReloadFromSettings resets the manager and re-reads the admin-panel
// settings. Mirrors Python's `reload_provider()` in
// `agent/sandbox/client.py` — call after the operator updates the
// sandbox settings.
func (m *ProviderManager) ReloadFromSettings(ctx context.Context, db *gorm.DB) error {
	return m.ReloadFromSettingsWithReader(ctx, db, dao.NewSystemSettingsDAO())
}

// ReloadFromSettingsWithReader is the testable seam for
// ReloadFromSettings.
func (m *ProviderManager) ReloadFromSettingsWithReader(ctx context.Context, db *gorm.DB, r SettingsReader) error {
	m.Reset()
	return m.LoadFromSettingsWithReader(ctx, db, r)
}

// loadSettingsConfig reads `sandbox.provider_type` and the
// matching `sandbox.{type}` JSON config from MySQL. Returns
// (ProviderType, nil) when the settings table has no rows for
// these keys (caller falls back to env).
func loadSettingsConfig(ctx context.Context, db *gorm.DB, r SettingsReader) (ProviderType, map[string]any, error) {
	rows, err := r.GetByNamePrefix(ctx, db, "sandbox.")
	if err != nil {
		return "", nil, err
	}
	settings := make(map[string]string, len(rows))
	for _, row := range rows {
		if _, exists := settings[row.Name]; exists {
			return "", nil, fmt.Errorf("duplicate setting %q", row.Name)
		}
		settings[row.Name] = row.Value
	}
	selected, explicit := settings["sandbox.provider_type"]
	ptype := ProviderSelfManaged
	if explicit {
		if selected == "" {
			return "", nil, errors.New("sandbox provider type is empty")
		}
		ptype = ProviderType(selected)
	}
	raw, exists := settings["sandbox."+string(ptype)]
	if !exists {
		if !explicit {
			return "", nil, errSettingsNotConfigured
		}
		return ptype, nil, fmt.Errorf("missing configuration for sandbox provider %q", ptype)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil || cfg == nil {
		return ptype, nil, errSettingsMalformed
	}
	return ptype, cfg, nil
}

// errSettingsNotConfigured permits environment bootstrap before the first save.
var errSettingsNotConfigured = errors.New("sandbox: admin-panel settings not configured")

// errSettingsMalformed rejects invalid persisted configuration without fallback.
var errSettingsMalformed = errors.New("sandbox: admin-panel settings JSON malformed")

// resolveProviderType reads SANDBOX_PROVIDER_TYPE. Defaults to
// "self_managed" to match the Python
// `_load_provider_from_settings` default.
func resolveProviderType() ProviderType {
	if v := common.GetEnv(common.EnvSandboxProviderType); v != "" {
		return ProviderType(v)
	}
	return ProviderSelfManaged
}

// buildProvider constructs an environment-configured provider by type.
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
	case ProviderTenki:
		return newTenkiProviderFromEnv(), nil
	case ProviderUCloudAgentSandbox:
		return newUCloudAgentSandboxProviderFromEnv(), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q (known: self_managed, aliyun_codeinterpreter, e2b, local, ssh, tenki, ucloud_agent_sandbox)", t)
	}
}

// buildProviderFromConfig is the settings-driven counterpart of
// buildProvider. Configuration uses the canonical lowercase Python schema keys.
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
	case ProviderTenki:
		return newTenkiProviderFromConfig(cfg), nil
	case ProviderUCloudAgentSandbox:
		return newUCloudAgentSandboxProviderFromConfig(cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q (known: self_managed, aliyun_codeinterpreter, e2b, local, ssh, tenki, ucloud_agent_sandbox)", t)
	}
}
