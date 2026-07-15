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

package component

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

func TestEmailComponentRegisteredAndSends(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	var receivedData strings.Builder
	done := make(chan struct{})
	go runEmailMockSMTP(t, ln, &receivedData, done)

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var portInt int
	_, _ = fmt.Sscanf(port, "%d", &portInt)

	c, err := New(componentNameEmail, map[string]any{
		"smtp_server":   "127.0.0.1",
		"smtp_port":     portInt,
		"email":         "alice@example.com",
		"smtp_username": "",
		"password":      "",
		"sender_name":   "Alice",
		"to_email":      "bob@example.com",
		"subject":       "Build status",
		"content":       "Build succeeded.",
	})
	if err != nil {
		t.Fatalf("New Email: %v", err)
	}

	state := runtime.NewCanvasState("run-email", "task-email")
	state.Sys["date"] = "2026-07-14 03:04:05"
	ctx := runtime.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["success"] != true {
		t.Fatalf("success = %v, want true; out=%v", out["success"], out)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("mock SMTP server did not close in time")
	}

	data := receivedData.String()
	for _, want := range []string{"Subject: Build status", "bob@example.com", "Build succeeded."} {
		if !strings.Contains(data, want) {
			t.Fatalf("mock SMTP payload missing %q\n--- data ---\n%s\n---", want, data)
		}
	}
}

func TestEmailComponentResolvesSysDateInSubject(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	var receivedData strings.Builder
	done := make(chan struct{})
	go runEmailMockSMTP(t, ln, &receivedData, done)

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var portInt int
	_, _ = fmt.Sscanf(port, "%d", &portInt)

	c, err := New(componentNameEmail, map[string]any{
		"smtp_server": "127.0.0.1",
		"smtp_port":   portInt,
		"email":       "alice@example.com",
		"to_email":    "bob@example.com",
		"subject":     "[noreply]{sys.date}",
		"content":     "body",
	})
	if err != nil {
		t.Fatalf("New Email: %v", err)
	}

	state := runtime.NewCanvasState("run-email", "task-email")
	state.Sys["date"] = "2026-07-14 03:04:05"
	out, err := c.Invoke(runtime.WithState(context.Background(), state), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["success"] != true {
		t.Fatalf("success = %v, want true; out=%v", out["success"], out)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("mock SMTP server did not close in time")
	}

	if strings.Contains(receivedData.String(), "{sys.date}") {
		t.Fatalf("subject still contains unresolved sys.date\n--- data ---\n%s\n---", receivedData.String())
	}
	if !strings.Contains(receivedData.String(), "Subject: [noreply]2026-07-14 03:04:05") {
		t.Fatalf("subject did not resolve sys.date\n--- data ---\n%s\n---", receivedData.String())
	}
}

func TestEmailComponentSoftFails(t *testing.T) {
	t.Parallel()

	c, err := New(componentNameEmail, map[string]any{
		"smtp_port": 465,
		"email":     "alice@example.com",
		"to_email":  "bob@example.com",
		"subject":   "Subject",
	})
	if err != nil {
		t.Fatalf("New Email: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke returned hard error: %v", err)
	}
	if out["success"] != false {
		t.Fatalf("success = %v, want false; out=%v", out["success"], out)
	}
	if _, ok := out["_ERROR"].(string); !ok {
		t.Fatalf("_ERROR missing or not string: %v", out)
	}
}

func runEmailMockSMTP(t *testing.T, ln net.Listener, receivedData *strings.Builder, done chan<- struct{}) {
	t.Helper()
	defer close(done)

	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
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
			_, _ = writer.WriteString("250-mock-smtp\r\n250-AUTH PLAIN\r\n250 OK\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(up, "AUTH PLAIN"):
			_, _ = writer.WriteString("235 Authentication successful\r\n")
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
}
