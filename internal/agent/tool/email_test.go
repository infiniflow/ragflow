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
		"Alice Sender",
		[]string{"bob@example.com"},
		[]string{"carol@example.com"},
		"Hello, world",
		"Body of the message.",
	)

	s := string(msg)
	for _, want := range []string{
		`From: "Alice Sender" <alice@example.com>`,
		"To: bob@example.com\r\n",
		"Cc: carol@example.com\r\n",
		"Subject: Hello, world",
		"Content-Type: text/plain; charset=UTF-8",
		"Body of the message.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q\n--- message ---\n%s\n---", want, s)
		}
	}
	if strings.Contains(s, "To: bob@example.com, carol@example.com") {
		t.Fatalf("CC recipient leaked into To header:\n%s", s)
	}
	// RFC 822 mandates a blank line between headers and body.
	if !strings.Contains(s, "\r\n\r\n") {
		t.Errorf("message missing blank line between headers and body\n%s", s)
	}
}

func TestEmail_SendBuildsDistinctHeadersAndEnvelopeRecipients(t *testing.T) {
	originalSendEmail := sendEmail
	t.Cleanup(func() { sendEmail = originalSendEmail })
	var sentParams emailParams
	var sentMessage []byte
	sendEmail = func(_ context.Context, p emailParams, msg []byte) error {
		sentParams = p
		sentMessage = append([]byte(nil), msg...)
		return nil
	}

	built, err := BuildByName("email", map[string]any{
		"smtp_server": "smtp.example.com",
		"smtp_port":   587,
		"email":       "alice@example.com",
		"sender_name": "Alice Sender",
	})
	if err != nil {
		t.Fatalf("BuildByName(email): %v", err)
	}
	args := map[string]any{
		"to_email": "bob@example.com",
		"cc_email": "carol@example.com, dave@example.com",
		"subject":  "Test {sys.date}",
		"content":  "Test body content.",
	}
	argsJSON, _ := json.Marshal(args)
	state := runtime.NewCanvasState("run-email", "task-email")
	state.Sys["date"] = "2026-07-15"
	out, err := built.(*EmailTool).InvokableRun(runtime.WithState(context.Background(), state), string(argsJSON))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var env emailEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK || env.Error != "" {
		t.Fatalf("output = %s, decode error = %v", out, err)
	}

	message := string(sentMessage)
	for _, want := range []string{
		`From: "Alice Sender" <alice@example.com>`,
		"To: bob@example.com\r\n",
		"Cc: carol@example.com, dave@example.com\r\n",
		"Subject: Test 2026-07-15",
		"Test body content.",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if got := emailRecipients(sentParams.ToEmail, sentParams.CCEmail); strings.Join(got, ",") != "bob@example.com,carol@example.com,dave@example.com" {
		t.Fatalf("SMTP envelope recipients = %#v", got)
	}
}

func TestEmail_STARTTLSRequiredBeforeSubmission(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer listener.Close()
	commands := make(chan []string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			commands <- nil
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		_, _ = writer.WriteString("220 mock-smtp ready\r\n")
		_ = writer.Flush()
		var received []string
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				commands <- received
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			received = append(received, command)
			if strings.HasPrefix(command, "EHLO") || strings.HasPrefix(command, "HELO") {
				_, _ = writer.WriteString("250 mock-smtp\r\n")
				_ = writer.Flush()
			}
		}
	}()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var portNumber int
	_, _ = fmt.Sscanf(port, "%d", &portNumber)
	err = sendEmailSTARTTLS(context.Background(), emailParams{
		SMTPServer: host, SMTPPort: portNumber, Email: "alice@example.com",
		ToEmail: "bob@example.com",
	}, []byte("message"))
	if err == nil || !strings.Contains(err.Error(), "does not advertise STARTTLS") {
		t.Fatalf("err = %v", err)
	}

	select {
	case received := <-commands:
		for _, command := range received {
			if strings.HasPrefix(command, "MAIL FROM") || strings.HasPrefix(command, "RCPT TO") || command == "DATA" {
				t.Fatalf("message submission started without STARTTLS: %#v", received)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mock SMTP server did not finish")
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
	meta := tool.ToolMeta()
	if meta.Name != "email" {
		t.Errorf("Name = %q, want email", meta.Name)
	}
	if !strings.Contains(meta.Description, "SMTP") {
		t.Errorf("Desc = %q, want to mention SMTP", meta.Description)
	}
	raw, err := json.Marshal(meta.Parameters)
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
