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

package mount

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single mount entry
type Entry struct {
	Mountpoint  string    `json:"mountpoint"`
	PID         int       `json:"pid"`
	ConfigPath  string    `json:"config_path"`
	ServerURL   string    `json:"server_url"`
	StartTime   time.Time `json:"start_time"`
	AutoCreated bool      `json:"auto_created"`
}

// Registry manages mount entries
type Registry struct {
	path string
	mu   sync.RWMutex
}

// DefaultRegistryPath returns the default path for the registry file
func DefaultRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ragflow_mounts.json"
	}
	return filepath.Join(home, ".ragflow_mounts.json")
}

// NewRegistry creates a new registry instance
func NewRegistry(path string) *Registry {
	if path == "" {
		path = DefaultRegistryPath()
	}
	return &Registry{path: path}
}

// Add adds a new mount entry
func (r *Registry) Add(entry *Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := r.load()
	if err != nil {
		entries = make(map[string]*Entry)
	}

	entries[entry.Mountpoint] = entry
	return r.save(entries)
}

// Remove removes a mount entry
func (r *Registry) Remove(mountpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := r.load()
	if err != nil {
		return nil
	}

	delete(entries, mountpoint)
	return r.save(entries)
}

// Get retrieves a mount entry by mountpoint
func (r *Registry) Get(mountpoint string) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := r.load()
	if err != nil {
		return nil, false
	}

	entry, ok := entries[mountpoint]
	return entry, ok
}

// List returns all mount entries
func (r *Registry) List() ([]*Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := r.load()
	if err != nil {
		return nil, err
	}

	result := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

// Cleanup removes entries for processes that no longer exist
func (r *Registry) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := r.load()
	if err != nil {
		return err
	}

	cleaned := make(map[string]*Entry)
	for mp, entry := range entries {
		if processExists(entry.PID) {
			cleaned[mp] = entry
		}
	}

	return r.save(cleaned)
}

func (r *Registry) load() (map[string]*Entry, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*Entry), nil
		}
		return nil, err
	}

	var entries map[string]*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *Registry) save(entries map[string]*Entry) error {
	// Ensure directory exists
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0600)
}

// processExists checks if a process with the given PID exists
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(os.Signal(nil))
	return err == nil
}
