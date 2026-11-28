//go:build desktop

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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/webui"
)

// uiServer holds the once-per-process UI startup: every service calls the ready
// hook, but the listener and the ready file must exist exactly once.
var uiServer struct {
	once sync.Once
	err  error
}

// startUIServer serves the bundled frontend on a loopback port and, when
// --ui-ready-file was given, writes the URL there so the window shell knows what
// to load. It runs from the ready hook: the UI proxies to the API and admin
// servers, so it must start after they listen.
func startUIServer(ctx context.Context, arguments *bootstrap.ServerArgs) error {
	if arguments.UI == nil || !*arguments.UI {
		return nil
	}
	uiServer.once.Do(func() {
		cfg := server.GetConfig()
		uiURL, shutdown, err := webui.Listen("127.0.0.1:0", webui.Options{
			APIAddr:   loopback(cfg.GetAPIServerConfig().HTTPPort),
			AdminAddr: loopback(cfg.GetAdminServerConfig().HTTPPort),
		})
		if err != nil {
			uiServer.err = fmt.Errorf("serve the web UI: %w", err)
			return
		}
		go func() {
			<-ctx.Done()
			shutdown()
		}()
		common.Info("Web UI served", zap.String("url", uiURL))
		if arguments.UIReadyFile == nil || *arguments.UIReadyFile == "" {
			return
		}
		if err := writeUIReadyFile(*arguments.UIReadyFile, uiURL); err != nil {
			uiServer.err = fmt.Errorf("write the UI ready file: %w", err)
		}
	})
	return uiServer.err
}

func loopback(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// uiReady is what the shell reads to find the window's URL.
type uiReady struct {
	URL string `json:"url"`
	PID int    `json:"pid"`
}

// writeUIReadyFile publishes the URL. The shell watches for this file and
// re-parses it until it is complete, so a torn write only costs one poll; the
// temp-and-rename keeps that from happening in the first place.
func writeUIReadyFile(path, uiURL string) error {
	payload, err := json.Marshal(uiReady{URL: uiURL, PID: os.Getpid()})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	// Windows refuses the rename when the reader has the target open; the shell
	// deletes the file before spawning, so a plain write is a correct fallback.
	_ = os.Remove(tmp)
	return os.WriteFile(path, payload, 0o644)
}
