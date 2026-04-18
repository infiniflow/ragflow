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
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"ragflow/internal/cli/filesystem"
	"ragflow/internal/cli/mount"
)

// MountOptions holds mount configuration
type MountOptions struct {
	Mountpoint string
	ConfigPath string
	ServerURL  string
	Foreground bool
	Debug      bool
	AllowOther bool
}

// Mount mounts the RAGFlow filesystem
func Mount(engine *filesystem.Engine, opts *MountOptions) error {
	// Ensure mountpoint exists
	if err := os.MkdirAll(opts.Mountpoint, 0755); err != nil {
		return fmt.Errorf("failed to create mountpoint: %w", err)
	}

	// Create FUSE filesystem
	rfs := NewRAGFlowFS(engine)

	// Mount options - disable caching for real-time updates
	zeroTimeout := time.Duration(0)
	
	// AllowOther defaults to true so non-root users can access the mount
	allowOther := opts.AllowOther
	if !allowOther {
		allowOther = true
	}
	
	mountOpts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Name:          "ragflow",
			FsName:        "ragflow",
			Debug:         opts.Debug,
			AllowOther:    allowOther,
			DisableXAttrs: true,
			MaxWrite:      128 * 1024,
			DirectMount:   true, // Bypass fusermount
		},
		// Disable all caching for real-time updates from server
		EntryTimeout:    &zeroTimeout,
		AttrTimeout:     &zeroTimeout,
		NegativeTimeout: &zeroTimeout,
		// Enable proper inode tracking
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	}

	if opts.Foreground {
		// Foreground mode: block until interrupted
		return mountForeground(rfs, opts.Mountpoint, mountOpts, opts)
	}

	// Background mode: daemonize
	return mountBackground(rfs, opts.Mountpoint, mountOpts, opts)
}

func mountForeground(rfs *RAGFlowFS, mountpoint string, opts *fs.Options, mopts *MountOptions) error {
	server, err := fs.Mount(mountpoint, rfs, opts)
	if err != nil {
		return fmt.Errorf("failed to mount: %w", err)
	}

	// Register mount
	registry := mount.NewRegistry("")
	entry := &mount.Entry{
		Mountpoint: mountpoint,
		PID:        os.Getpid(),
		ConfigPath: mopts.ConfigPath,
		ServerURL:  mopts.ServerURL,
		StartTime:  time.Now(),
	}
	_ = registry.Add(entry)

	fmt.Printf("Mounted RAGFlow at %s (PID: %d)\n", mountpoint, os.Getpid())
	fmt.Println("Press Ctrl+C to unmount...")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		fmt.Println("\nUnmounting...")
		server.Unmount()
	}()

	server.Wait()
	registry.Remove(mountpoint)
	fmt.Println("Unmount completed.")
	return nil
}

func mountBackground(rfs *RAGFlowFS, mountpoint string, opts *fs.Options, mopts *MountOptions) error {
	// Go's runtime is multi-threaded; using syscall.ForkExec from a multi-threaded
	// program is unsafe. The reliable approach is to use exec.Command which
	// properly handles the file descriptor setup.
	
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Get absolute path for config file if specified
	configPath := mopts.ConfigPath
	if configPath != "" && !filepath.IsAbs(configPath) {
		absPath, err := filepath.Abs(configPath)
		if err == nil {
			configPath = absPath
		}
	}
	
	// Build args - filter out conflicting flags and add --foreground
	// Need to be careful: the original args are like: [-f, rf.yml, mount, /mnt/ragflow]
	// After processing: [-f, /abs/path/rf.yml, mount, /mnt/ragflow, --foreground]
	args := []string{}
	originalArgs := os.Args[1:]
	for i := 0; i < len(originalArgs); i++ {
		arg := originalArgs[i]
		
		// Handle -f/--file flag - convert to absolute path
		if arg == "-f" || arg == "--file" {
			args = append(args, arg)
			if i+1 < len(originalArgs) {
				args = append(args, configPath) // Use absolute path
				i++ // Skip the original config path value
			}
			continue
		}
		// Handle --file=value form
		if strings.HasPrefix(arg, "--file=") || strings.HasPrefix(arg, "-f=") {
			args = append(args, "--file="+configPath)
			continue
		}
		// Skip other conflicting flags
		if arg == "--foreground" || arg == "-foreground" ||
		   arg == "--daemon" || arg == "-daemon" ||
		   arg == "--background" || arg == "-background" {
			continue
		}
		args = append(args, arg)
	}
	args = append(args, "--foreground")
	
	// Create log file for the daemon
	logFile := fmt.Sprintf("/tmp/ragflow_mount_%d.log", time.Now().Unix())
	logFd, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFd.Close()
	
	// Open /dev/null for stdin
	devNull, err := os.Open("/dev/null")
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer devNull.Close()
	
	// Debug: log the command being executed
	fmt.Fprintf(logFd, "Daemon command: %s %v\n", execPath, args)
	fmt.Fprintf(logFd, "Original os.Args: %v\n", os.Args)
	fmt.Fprintf(logFd, "ConfigPath from mopts: %q, resolved: %q\n", mopts.ConfigPath, configPath)
	
	// Create command
	cmd := exec.Command(execPath, args...)
	cmd.Stdin = devNull
	cmd.Stdout = logFd
	cmd.Stderr = logFd
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session, detach from controlling terminal
	}
	
	// Start the daemon
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}
	
	pid := cmd.Process.Pid
	
	// Detach - we don't call cmd.Wait() so the process can outlive us
	// Just release resources associated with the cmd
	go func() {
		cmd.Wait() // Reap the child when it eventually exits
	}()
	
	// Give the daemon a moment to start and potentially fail
	time.Sleep(1 * time.Second)
	
	// Check if process still exists
	if err := syscall.Kill(pid, 0); err != nil {
		// Process died, read log for error
		if logData, err := os.ReadFile(logFile); err == nil && len(logData) > 0 {
			os.Remove(logFile)
			return fmt.Errorf("mount failed: %s", string(logData))
		}
		os.Remove(logFile)
		return fmt.Errorf("mount process exited unexpectedly")
	}
	
	fmt.Printf("Mounted RAGFlow at %s (PID: %d)\n", mountpoint, pid)
	fmt.Printf("Log file: %s\n", logFile)
	return nil
}


