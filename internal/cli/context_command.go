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

package cli

import (
	"fmt"

	"ragflow/internal/cli/fuse"
)

func (c *RAGFlowClient) ContextMount(cmd *Command) (ResponseIf, error) {
	if c.HTTPClient.APIToken == "" && c.HTTPClient.LoginToken == "" {
		return nil, fmt.Errorf("API token not set. Please login first")
	}
	if c.ServerType != "user" {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}

	mountpoint, ok := cmd.Params["mountpoint"].(string)
	if !ok || mountpoint == "" {
		return nil, fmt.Errorf("mountpoint is required")
	}

	// Get foreground option
	foreground := false
	if fg, ok := cmd.Params["foreground"].(bool); ok {
		foreground = fg
	}

	// Get config path
	configPath := ""
	if cp, ok := cmd.Params["config_path"].(string); ok {
		configPath = cp
	}

	// Build server URL
	serverURL := fmt.Sprintf("http://%s:%d", c.HTTPClient.Host, c.HTTPClient.Port)

	// Mount options
	opts := &fuse.MountOptions{
		Mountpoint: mountpoint,
		ConfigPath: configPath,
		ServerURL:  serverURL,
		Foreground: foreground,
	}

	// Start mount (this blocks in foreground mode)
	err := fuse.Mount(c.ContextEngine, opts)
	if err != nil {
		return nil, fmt.Errorf("mount failed: %w", err)
	}

	var response ContextMountResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Message = fmt.Sprintf("Mounted at %s", mountpoint)

	return &response, nil
}

func (c *RAGFlowClient) ContextUnmount(cmd *Command) (ResponseIf, error) {
	mountpoint, ok := cmd.Params["mountpoint"].(string)
	if !ok || mountpoint == "" {
		return nil, fmt.Errorf("mountpoint is required")
	}

	if err := fuse.Unmount(mountpoint); err != nil {
		return nil, err
	}

	var response ContextUnmountResponse
	response.OutputFormat = c.OutputFormat
	response.Code = 0
	response.Message = fmt.Sprintf("Unmounted %s", mountpoint)

	return &response, nil
}
