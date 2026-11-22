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

package channels

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWhatsAppEventsURLEncodesSessionKeyOnce(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		sessionKey  string
		wantScheme  string
		wantEscaped string
	}{
		{
			name:        "default key",
			baseURL:     "http://gateway.test:3005",
			sessionKey:  "default",
			wantScheme:  "ws",
			wantEscaped: "/whatsapp/default/events/ws",
		},
		{
			name:        "space",
			baseURL:     "http://gateway.test:3005",
			sessionKey:  "a b",
			wantScheme:  "ws",
			wantEscaped: "/whatsapp/a%20b/events/ws",
		},
		{
			name:        "percent",
			baseURL:     "http://gateway.test:3005",
			sessionKey:  "a%b",
			wantScheme:  "ws",
			wantEscaped: "/whatsapp/a%25b/events/ws",
		},
		{
			name:        "question mark",
			baseURL:     "http://gateway.test:3005",
			sessionKey:  "a?b",
			wantScheme:  "ws",
			wantEscaped: "/whatsapp/a%3Fb/events/ws",
		},
		{
			name:        "non ascii over tls",
			baseURL:     "https://gateway.test",
			sessionKey:  "会话1",
			wantScheme:  "wss",
			wantEscaped: "/whatsapp/%E4%BC%9A%E8%AF%9D1/events/ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := newWhatsAppChannel(whatsappAccount{
				AccountID:      "probe",
				GatewayBaseURL: tt.baseURL,
				SessionKey:     tt.sessionKey,
			})
			channel.eventCursor = 7

			raw, err := channel.eventsURL()
			if err != nil {
				t.Fatalf("eventsURL() error = %v", err)
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse eventsURL() = %q: %v", raw, err)
			}
			if parsed.Scheme != tt.wantScheme {
				t.Fatalf("eventsURL() scheme = %q, want %q", parsed.Scheme, tt.wantScheme)
			}
			if got := parsed.EscapedPath(); got != tt.wantEscaped {
				t.Fatalf("eventsURL() escaped path = %q, want %q (URL %q)", got, tt.wantEscaped, raw)
			}
			if got, want := parsed.Path, "/whatsapp/"+tt.sessionKey+"/events/ws"; got != want {
				t.Fatalf("eventsURL() decoded path = %q, want %q", got, want)
			}
			if got := parsed.Query().Get("after"); got != "7" {
				t.Fatalf("eventsURL() after = %q, want 7", got)
			}
		})
	}
}

func TestWhatsAppEventsURLReachesGatewaySession(t *testing.T) {
	const sessionKey = "a b"
	upgrader := websocket.Upgrader{}
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// internal/channels/whatsapp-gateway/index.js reads the request target with
		// `new URL(req.url).pathname`, splits it on "/", and decodes the session key once.
		parsed, err := url.Parse(r.RequestURI)
		if err != nil {
			t.Errorf("parse request target %q: %v", r.RequestURI, err)
			return
		}
		escaped := parsed.EscapedPath()
		parts := strings.FieldsFunc(escaped, func(r rune) bool { return r == '/' })
		if len(parts) != 4 || parts[0] != "whatsapp" || parts[2] != "events" || parts[3] != "ws" {
			t.Errorf("request path = %q, want /whatsapp/<session key>/events/ws", escaped)
			return
		}
		key, err := url.PathUnescape(parts[1])
		if err != nil {
			t.Errorf("decode session key %q: %v", parts[1], err)
			return
		}
		received <- key

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// The gateway writes 101 and then tears the socket down when the session is absent.
		_ = conn.Close()
	}))
	defer server.Close()

	allowChannelURLForTest(t, server.URL)
	channel := newWhatsAppChannel(whatsappAccount{
		AccountID:      "probe",
		GatewayBaseURL: server.URL,
		SessionKey:     sessionKey,
	})
	channel.eventCursor = 7

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := channel.runEvents(ctx); err == nil {
		t.Fatal("runEvents() succeeded, want the gateway close to end the event stream")
	}

	select {
	case key := <-received:
		if key != sessionKey {
			t.Fatalf("gateway decoded session key = %q, want %q", key, sessionKey)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not receive the events subscription")
	}
}

type whatsappRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f whatsappRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestWhatsAppStopWaitsForRunBeforeRestart(t *testing.T) {
	started := make(chan struct{}, 2)
	canceled := make(chan struct{}, 2)
	release := make(chan struct{})
	channel := newWhatsAppChannel(whatsappAccount{AccountID: "wait-for-run"})
	channel.client = &http.Client{Transport: whatsappRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/start") {
			started <- struct{}{}
			<-req.Context().Done()
			canceled <- struct{}{}
			<-release
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	if err := channel.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-started
	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- channel.Stop(stopCtx) }()
	<-canceled
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before the runtime exited: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	restarted := make(chan error, 1)
	go func() { restarted <- channel.Start(t.Context()) }()
	select {
	case err := <-restarted:
		t.Fatalf("Start returned before the old runtime stopped: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := <-restarted; err != nil {
		t.Fatal(err)
	}
	<-started
	if err := channel.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if snapshot := channel.Snapshot(); snapshot.Status != "stopped" || snapshot.QRDataURL != nil {
		t.Fatalf("stopped snapshot=%+v", snapshot)
	}
}

func TestWhatsAppStopRespectsCleanupDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	channel := newWhatsAppChannel(whatsappAccount{AccountID: "bounded-stop"})
	channel.client = &http.Client{Transport: whatsappRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/start") {
			close(started)
			<-req.Context().Done()
			<-release
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	if err := channel.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := channel.Stop(ctx)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error=%v, want deadline", err)
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(t.Context(), time.Second)
	defer cleanupCancel()
	if err := channel.Stop(cleanupCtx); err != nil {
		t.Fatal(err)
	}
}
