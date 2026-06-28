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

	tool := NewEmailTool()
	args := map[string]any{
		"smtp_host": "127.0.0.1",
		"smtp_port": portInt,
		"from_addr": "alice@example.com",
		"to_addrs":  []string{"bob@example.com"},
		"subject":   "Test Subject",
		"body":      "Test body content.",
	}
	argsJSON, _ := json.Marshal(args)
	out, err := tool.InvokableRun(context.Background(), string(argsJSON))
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

	if !strings.Contains(receivedData.String(), "Subject: Test Subject") {
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

	tool := NewEmailTool()

	cases := []struct {
		name    string
		args    string
		wantErr string
	}{
		{
			name:    "missing smtp_host",
			args:    `{"smtp_port":587,"from_addr":"a@b","to_addrs":["c@d"],"subject":"s","body":"b"}`,
			wantErr: "smtp_host",
		},
		{
			name:    "missing to_addrs",
			args:    `{"smtp_host":"x","smtp_port":587,"from_addr":"a@b","subject":"s","body":"b"}`,
			wantErr: "to_addrs",
		},
		{
			name:    "bad smtp_port",
			args:    `{"smtp_host":"x","smtp_port":0,"from_addr":"a@b","to_addrs":["c@d"],"subject":"s","body":"b"}`,
			wantErr: "smtp_port",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tool.InvokableRun(context.Background(), tc.args)
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
}
