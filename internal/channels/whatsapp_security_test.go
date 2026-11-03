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
