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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhatsAppRuntimeErrorDoesNotExposeGatewayResponseBody(t *testing.T) {
	const responseSecret = "CHANNEL_RESPONSE_SECRET_SENTINEL"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(responseSecret))
	}))
	defer server.Close()
	allowChannelURLForTest(t, server.URL)

	channel := newWhatsAppChannel(whatsappAccount{AccountID: "probe", GatewayBaseURL: server.URL})
	err := channel.requestJSON(context.Background(), http.MethodGet, "whatsapp/probe/status", nil, nil)
	if err == nil {
		t.Fatal("requestJSON() succeeded for HTTP 500")
	}
	channel.setError(err)
	if got := *channel.Snapshot().LastError; !strings.Contains(got, "500") {
		t.Fatalf("runtime last_error = %q, want the HTTP status", got)
	} else if strings.Contains(got, responseSecret) {
		t.Fatalf("runtime last_error exposed gateway response body: %q", got)
	}
}

func TestWhatsAppRejectsUntrustedGatewayDestination(t *testing.T) {
	t.Setenv(channelAllowedHostsEnv, "")
	channel := newWhatsAppChannel(whatsappAccount{
		AccountID:      "probe",
		GatewayBaseURL: "http://169.254.169.254/",
	})
	err := channel.requestJSON(context.Background(), http.MethodGet, "whatsapp/probe/status", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("requestJSON() error = %v, want an untrusted-destination rejection", err)
	}
}
