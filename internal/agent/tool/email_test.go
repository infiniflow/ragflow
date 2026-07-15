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

package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

func TestEmail_BuildMessage(t *testing.T) {
	t.Parallel()

	msg := buildEmailMessage(
		"alice@example.com",
		[]string{"bob@example.com", "carol@example.com"},
		"Hello, world",
		"Body of the message.",
	)

	s := string(msg)
	for _, want := range []string{
		"From: alice@example.com",
		"To: bob@example.com, carol@example.com",
		"Subject: Hello, world",
		"Content-Type: text/plain; charset=UTF-8",
		"Body of the message.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q\n--- message ---\n%s\n---", want, s)
		}
	}
	// RFC 822 mandates a blank line between headers and body.
	if !strings.Contains(s, "\r\n\r\n") {
		t.Errorf("message missing blank line between headers and body\n%s", s)
	}
}

func TestEmail_SendAgainstMockSMTP(t *testing.T) {
	t.Parallel()

	// Spin up a minimal SMTP server: read commands, respond 250 to
	// everything, and copy the DATA payload bytes so the test can
	// inspect them.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	var receivedData strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		// Greeting
		_, _ = writer.WriteString("220 mock-smtp ready\r\n")
		_ = writer.Flush()
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			up := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				_, _ = writer.WriteString("250-mock-smtp\r\n250 OK\r\n")
				_ = writer.Flush()
			case strings.HasPrefix(up, "MAIL FROM:"), strings.HasPrefix(up, "RCPT TO:"):
				_, _ = writer.WriteString("250 OK\r\n")
				_ = writer.Flush()
			case strings.HasPrefix(up, "DATA"):
				_, _ = writer.WriteString("354 End data with <CR><LF>.<CR><LF>\r\n")
				_ = writer.Flush()
				inData = true
			case inData && strings.TrimSpace(line) == ".":
				_, _ = writer.WriteString("250 Queued\r\n")
				_ = writer.Flush()
				inData = false
			case inData:
				receivedData.WriteString(line)
			case strings.HasPrefix(up, "QUIT"):
				_, _ = writer.WriteString("221 Bye\r\n")
				_ = writer.Flush()
				return
			default:
				_, _ = writer.WriteString("250 OK\r\n")
				_ = writer.Flush()
			}
		}
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	_ = host
	var portInt int
	_, _ = fmt.Sscanf(port, "%d", &portInt)

	built, err := BuildByName("email", map[string]any{
		"smtp_server": "127.0.0.1",
		"smtp_port":   portInt,
		"email":       "alice@example.com",
		"sender_name": "Alice",
	})
	if err != nil {
		t.Fatalf("BuildByName(email): %v", err)
	}
	emailTool := built.(*EmailTool)
	args := map[string]any{
		"to_email": "bob@example.com",
		"subject":  "Test {sys.date}",
		"content":  "Test body content.",
	}
	argsJSON, _ := json.Marshal(args)
	state := runtime.NewCanvasState("run-email", "task-email")
	state.Sys["date"] = "2026-07-15"
	out, err := emailTool.InvokableRun(runtime.WithState(context.Background(), state), string(argsJSON))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var env emailEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if !env.OK {
		t.Errorf("OK = false, want true")
	}

	// Wait for the mock server to finish.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("mock SMTP server did not close in time")
	}

	if !strings.Contains(receivedData.String(), "Subject: Test 2026-07-15") {
		t.Errorf("mock server did not receive subject\n--- data ---\n%s\n---",
			receivedData.String())
	}
	if !strings.Contains(receivedData.String(), "bob@example.com") {
		t.Errorf("mock server did not receive recipient\n--- data ---\n%s\n---",
			receivedData.String())
	}
	if !strings.Contains(receivedData.String(), "Test body content.") {
		t.Errorf("mock server did not receive body\n--- data ---\n%s\n---",
			receivedData.String())
	}
}

func TestEmail_RequiresFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tool    *EmailTool
		args    string
		wantErr string
	}{
		{
			name:    "missing smtp_server",
			tool:    newEmailTool(emailParams{SMTPPort: 587, Email: "a@b"}),
			args:    `{"to_email":"c@d","subject":"s","content":"b"}`,
			wantErr: "smtp_server",
		},
		{
			name:    "missing to_email",
			tool:    newEmailTool(emailParams{SMTPServer: "x", SMTPPort: 587, Email: "a@b"}),
			args:    `{"subject":"s","content":"b"}`,
			wantErr: "to_email",
		},
		{
			name:    "bad smtp_port",
			tool:    newEmailTool(emailParams{SMTPServer: "x", SMTPPort: -1, Email: "a@b"}),
			args:    `{"to_email":"c@d","subject":"s","content":"b"}`,
			wantErr: "smtp_port",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.tool.InvokableRun(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestEmail_Info(t *testing.T) {
	t.Parallel()

	tool := NewEmailTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "email" {
		t.Errorf("Name = %q, want email", info.Name)
	}
	if !strings.Contains(info.Desc, "SMTP") {
		t.Errorf("Desc = %q, want to mention SMTP", info.Desc)
	}
	schemaJSON, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, err := json.Marshal(schemaJSON)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	for _, runtimeField := range []string{"to_email", "cc_email", "content", "subject"} {
		if !strings.Contains(string(raw), `"`+runtimeField+`"`) {
			t.Errorf("schema missing runtime field %q: %s", runtimeField, raw)
		}
	}
	for _, configField := range []string{"smtp_server", "smtp_port", "email", "password", "sender_name"} {
		if strings.Contains(string(raw), `"`+configField+`"`) {
			t.Errorf("schema leaked node config %q: %s", configField, raw)
		}
	}
}

func TestEmail_ComponentContractAndFactory(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("email", map[string]any{
		"smtp_server": "smtp.example.com",
		"smtp_port":   "587",
		"email":       "sender@example.com",
		"password":    "secret",
		"sender_name": "Sender",
		"outputs":     map[string]any{"success": map[string]any{}},
		"setups":      map[string]any{"to_email": "configured@example.com"},
	})
	if err != nil {
		t.Fatalf("BuildByName(email): %v", err)
	}
	emailTool := built.(*EmailTool)
	if emailTool.defaults.SMTPPort != 587 || emailTool.defaults.SMTPServer != "smtp.example.com" {
		t.Fatalf("node defaults = %#v", emailTool.defaults)
	}
	spec := emailTool.ComponentSpec()
	if _, ok := spec.Inputs["to_email"]; !ok {
		t.Fatalf("component inputs = %#v", spec.Inputs)
	}
	if _, ok := spec.Outputs["success"]; !ok {
		t.Fatalf("component outputs = %#v", spec.Outputs)
	}
	toEmail, ok := spec.InputForm["to_email"].(map[string]any)
	if !ok || toEmail["name"] != "To " || toEmail["type"] != "line" {
		t.Fatalf("to_email input form = %#v", spec.InputForm["to_email"])
	}
	if outputs := emailTool.BuildComponentOutputs(map[string]any{"ok": true, "provider": "smtp"}); outputs["success"] != true {
		t.Fatalf("component outputs = %#v", outputs)
	}
}

func TestEmail_BuildByNameRejectsInvalidNodeParams(t *testing.T) {
	t.Parallel()

	for _, params := range []map[string]any{
		{"smtp_port": float64(1.5)},
		{"smtp_port": 70000},
		{"smtp_server": 1},
	} {
		if _, err := BuildByName("email", params); err == nil {
			t.Fatalf("BuildByName(email, %#v) succeeded, want error", params)
		}
	}
}
