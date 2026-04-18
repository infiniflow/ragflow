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

package fuse

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"

	"ragflow/internal/cli/mount"
)

// Unmount unmounts the RAGFlow filesystem
func Unmount(mountpoint string) error {
	// Check if mounted via registry
	registry := mount.NewRegistry("")
	entry, ok := registry.Get(mountpoint)
	if ok {
		// Try to kill the process first
		if entry.PID > 0 {
			_ = syscall.Kill(entry.PID, syscall.SIGTERM)
		}
		registry.Remove(mountpoint)
	}

	// Unmount using system command
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("fusermount", "-u", mountpoint)
	case "darwin":
		cmd = exec.Command("diskutil", "unmount", mountpoint)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unmount: %v (output: %s)", err, output)
	}

	fmt.Printf("Unmounted %s\n", mountpoint)
	return nil
}

// ListMounts lists all active RAGFlow mounts
func ListMounts() ([]*mount.Entry, error) {
	registry := mount.NewRegistry("")
	return registry.List()
}

// PrintMounts prints all active mounts in a table format
func PrintMounts() {
	entries, err := ListMounts()
	if err != nil {
		fmt.Printf("Error listing mounts: %v\n", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No active RAGFlow mounts")
		return
	}

	fmt.Printf("%-30s %-10s %-30s %-20s\n", "MOUNTPOINT", "PID", "SERVER", "STARTED")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, entry := range entries {
		started := entry.StartTime.Format("2006-01-02 15:04:05")
		fmt.Printf("%-30s %-10d %-30s %-20s\n", entry.Mountpoint, entry.PID, entry.ServerURL, started)
	}
}

// IsMounted checks if a path is currently mounted
func IsMounted(mountpoint string) bool {
	registry := mount.NewRegistry("")
	_, ok := registry.Get(mountpoint)
	return ok
}

// CleanupMounts removes stale mount entries
func CleanupMounts() error {
	registry := mount.NewRegistry("")
	return registry.Cleanup()
}
