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
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestAdminUserListRequests(t *testing.T) {
	for _, tt := range []struct{ input, query string }{
		{"LIST USERS;", ""},
		{"LIST USERS WITH keyword 'Alice + 日志' status 'active' order 'create_time' sort 'asc' page 2 page_size 5;", "keyword=Alice+%2B+%E6%97%A5%E5%BF%97&order=create_time&page=2&page_size=5&sort=asc&status=active"},
		{"LIST USERS WITH role 'Reader' plan 'Pro' quota 0;", "plan=Pro&quota=0&role=Reader"},
		{"LIST USERS WITH plan 'Pro' days 30 page 2 page_size 10;", "days=30&page=2&page_size=10&plan=Pro"},
		{"LIST USERS WITH order 'storage' top 50;", "order=storage&top=50"},
		{"LIST USERS WITH days 0;", "days=0"},
		{"LIST USERS PAGE 2 PAGE_SIZE 5;", "page=2&page_size=5"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/users" || r.URL.RawQuery != tt.query || len(body) != 0 || r.Header.Get("Authorization") != "admin-token" {
					t.Errorf("request = %s %s body=%q", r.Method, r.URL, body)
				}
				_, _ = io.WriteString(w, `{"code":0,"data":[{"email":"alice@example.com","create_date":123}],"total":17}`)
			}))
			token := "admin-token"
			client.LoginToken = &token
			c := &CLI{Config: &CommandLineConfig{CLIMode: AdminMode, OutputFormat: OutputFormatJSON}, AdminServerClient: client}
			cmd, err := NewParser(tt.input).Parse(AdminMode)
			if err != nil {
				t.Fatal(err)
			}
			response, err := c.ExecuteCommand(1, cmd)
			if err != nil {
				t.Fatal(err)
			}
			result := response.(*adminUserListResponse)
			if result.Total != 17 || len(result.Data) != 1 || result.Data[0]["create_date"] == nil || result.OutputFormat != OutputFormatJSON {
				t.Fatalf("unexpected response: %#v", result)
			}
		})
	}
}

func TestAdminUserReportsKeepListEndpoint(t *testing.T) {
	if _, ok := any(&CLI{}).(adminUserReportRunner); ok {
		t.Skip("enterprise report routing is covered by enterprise command tests")
	}
	for _, tt := range []struct{ input, query string }{
		{"LIST USERS INACTIVE DAYS 30;", "days=30&status=INACTIVE"},
		{"LIST USERS ACTIVE DAYS 7;", "days=7&status=ACTIVE"},
		{"LIST USERS STORAGE TOP 3;", "order=STORAGE&top=3"},
		{"LIST USERS DOCUMENTS TOP 2;", "order=DOCUMENTS&top=2"},
		{"LIST USERS INDEX TOP 2;", "order=INDEX&top=2"},
		{"LIST USERS PLAN 'Pro' QUOTA 80 DAYS 7;", "days=7&plan=Pro&quota=80"},
		{"LIST USERS INACTIVE DAYS 30 PAGE 2 PAGE_SIZE 10;", "days=30&page=2&page_size=10&status=INACTIVE"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/users" || r.URL.RawQuery != tt.query || len(body) != 0 {
					t.Errorf("request = %s %s body=%q", r.Method, r.URL, body)
				}
				_, _ = io.WriteString(w, `{"code":0,"data":[{"user":"alice@example.com"}],"total":1}`)
			}))
			token := "admin-token"
			client.LoginToken = &token
			c := &CLI{Config: &CommandLineConfig{CLIMode: AdminMode}, AdminServerClient: client}
			cmd, err := NewParser(tt.input).Parse(AdminMode)
			if err != nil {
				t.Fatal(err)
			}
			response, err := c.ExecuteCommand(1, cmd)
			if err != nil {
				t.Fatal(err)
			}
			result := response.(*OrderedCommonResponse)
			if len(result.Data) != 1 || result.Data[0]["user"] != "alice@example.com" {
				t.Fatalf("unexpected report response: %#v", result)
			}
		})
	}
}

func TestAdminUserListRejectsInvalidOptions(t *testing.T) {
	for _, input := range []string{
		"LIST USERS WITH;", "LIST USERS WITH unknown 'value';",
		"LIST USERS WITH keyword;", "LIST USERS WITH keyword 'a' keyword 'b';",
		"LIST USERS WITH page 0;", "LIST USERS WITH page_size 0;", "LIST USERS WITH page 1.5;",
		"LIST USERS WITH quota 101;", "LIST USERS WITH status active;",
		"LIST USERS WITH days -1;", "LIST USERS WITH top -1;",
		"LIST USERS WITH top 0;", "LIST USERS STORAGE TOP 0;",
		"LIST USERS WITH top 10;", "LIST USERS QUOTA 80 TOP 10;", "LIST USERS ACTIVE TOP 10;",
		"LIST USERS WITH order 'storage' top 10 page 1;", "LIST USERS WITH order 'index' page_size 10 top 10;",
		"LIST USERS STORAGE TOP 10 PAGE 1;", "LIST USERS DOCUMENTS PAGE_SIZE 10 TOP 10;",
		"LIST USERS ACTIVE PAGE 0;", "LIST USERS ACTIVE PAGE_SIZE 0;",
		"LIST USERS ACTIVE PAGE 1 PAGE 2;",
		"LIST USERS WITH keyword 'a'; trailing", "LIST USERS ACTIVE WITH keyword 'a';",
	} {
		t.Run(input, func(t *testing.T) {
			if cmd, err := NewParser(input).Parse(AdminMode); cmd != nil || err == nil {
				t.Fatalf("Parse() = (%v, %v)", cmd, err)
			}
		})
	}
}

func TestAdminUserListErrors(t *testing.T) {
	for _, tt := range []struct {
		status        int
		body, message string
	}{
		{http.StatusForbidden, `{"code":403}`, "HTTP 403"},
		{http.StatusOK, `{"code":400,"message":"unsupported order"}`, "unsupported order"},
		{http.StatusOK, `{"code":0,"data":`, "invalid JSON"},
	} {
		t.Run(tt.message, func(t *testing.T) {
			client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			token := "admin-token"
			client.LoginToken = &token
			c := &CLI{Config: &CommandLineConfig{CLIMode: AdminMode}, AdminServerClient: client}
			_, err := c.AdminListUsersCommand(1, NewCommand("admin_list_users_command"))
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := (&CLI{}).AdminListUsersCommand(1, NewCommand("admin_list_users_command")); err == nil {
		t.Fatal("unauthenticated client was accepted")
	}
}

func TestAdminUserListOutputIncludesTotal(t *testing.T) {
	for _, format := range []OutputFormat{OutputFormatJSON, OutputFormatPlain, OutputFormatTable} {
		t.Run(string(format), func(t *testing.T) {
			result := &adminUserListResponse{CommonResponse: CommonResponse{Data: []map[string]interface{}{{"email": "alice@example.com"}}, OutputFormat: format}, Total: 17}
			file, err := os.CreateTemp(t.TempDir(), "user-list-output-*")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			stdout := os.Stdout
			os.Stdout = file
			defer func() { os.Stdout = stdout }()
			result.PrintOut()
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if format == OutputFormatJSON {
				var value struct {
					Users []map[string]interface{} `json:"users"`
					Total int64                    `json:"total"`
				}
				if err := json.Unmarshal(output, &value); err != nil || value.Total != 17 || len(value.Users) != 1 {
					t.Fatalf("output = %s, err = %v", output, err)
				}
			} else if !strings.Contains(string(output), "Total: 17") || !strings.Contains(string(output), "alice@example.com") {
				t.Fatalf("output = %s", output)
			}
		})
	}
}
