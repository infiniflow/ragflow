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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// newAuthTestServer serves the ping and login endpoints the CLI touches while
// acquiring credentials.
func newAuthTestServer(t *testing.T) (string, int) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/system/ping":
			_, _ = w.Write([]byte("pong"))
		case "/api/v1/admin/ping":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"Pong"}`))
		case "/api/v1/auth/login", "/api/v1/admin/login":
			w.Header().Set("Authorization", "token-1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

// Command mode is the default CLI mode, so -u/-p without an API key has to
// reach the user login endpoint; only API and admin modes were handled.
func TestCommandModeInitializationLogsInWithoutAPIKey(t *testing.T) {
	host, port := newAuthTestServer(t)
	user, password := "alice", "pw"

	config := &CommandLineConfig{
		CLIMode:      CommandMode,
		OutputFormat: OutputFormatPlain,
		APIClientConfig: &APIModeConfig{
			CurrentAPIServer: DefaultAPIServer,
			APIServerMap: map[string]*APIServerConfig{
				DefaultAPIServer: {IP: host, Port: port, UserName: &user, UserPassword: &password},
			},
		},
	}

	client, err := NewCLIWithConfig(config)
	if err != nil {
		t.Fatalf("NewCLIWithConfig: %v", err)
	}
	t.Cleanup(client.Cleanup)

	token := client.APIServerClientMap[DefaultAPIServer].LoginToken
	if token == nil || *token != "token-1" {
		t.Fatalf("login token = %v, want token-1", token)
	}
}

// A single command used to skip the login step the interactive loop performs,
// so an admin command ran without a LoginToken and was rejected.
func TestRunSingleCommandPreparesAdminAuth(t *testing.T) {
	host, port := newAuthTestServer(t)
	adminName := "admin"

	config := &CommandLineConfig{
		CLIMode:      AdminMode,
		OutputFormat: OutputFormatPlain,
		AdminClientConfig: &AdminModeConfig{
			AdminHost: host,
			AdminPort: port,
			AdminName: &adminName,
		},
	}

	client, err := NewCLIWithConfig(config)
	if err != nil {
		t.Fatalf("NewCLIWithConfig: %v", err)
	}
	client.PasswordPrompt = func(string) (string, error) { return "secret", nil }

	command := "ping"
	if err := client.RunSingleCommand(&command); err != nil {
		t.Fatalf("RunSingleCommand: %v", err)
	}
	if client.AdminServerClient.LoginToken == nil {
		t.Fatal("admin token was not acquired before the command ran")
	}
}
