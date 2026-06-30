package service

import (
	"net/url"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestBuildBoxAuthorizationURL(t *testing.T) {
	authURL, err := buildBoxAuthorizationURL("client-1", "http://localhost/callback", "flow-1")
	if err != nil {
		t.Fatalf("buildBoxAuthorizationURL: %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != boxOAuthAuthorizeURL {
		t.Fatalf("base url=%q want=%q", got, boxOAuthAuthorizeURL)
	}

	query := parsed.Query()
	if query.Get("client_id") != "client-1" {
		t.Fatalf("client_id=%q", query.Get("client_id"))
	}
	if query.Get("redirect_uri") != "http://localhost/callback" {
		t.Fatalf("redirect_uri=%q", query.Get("redirect_uri"))
	}
	if query.Get("response_type") != "code" {
		t.Fatalf("response_type=%q", query.Get("response_type"))
	}
	if query.Get("state") != "flow-1" {
		t.Fatalf("state=%q", query.Get("state"))
	}
}

func TestRenderWebOAuthPopupBoxPayload(t *testing.T) {
	html := renderWebOAuthPopup("flow-1", true, "Authorization completed successfully.", "box")

	for _, want := range []string{
		"Box Authorization",
		"Authorization complete",
		"ragflow-box-oauth",
		`"status":"success"`,
		`"flowId":"flow-1"`,
		"window.close();",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("popup html missing %q:\n%s", want, html)
		}
	}
}

func TestDefaultBoxWebOAuthRedirectURI(t *testing.T) {
	t.Setenv("BOX_WEB_OAUTH_REDIRECT_URI", "http://example.test/box/callback")

	if got := defaultBoxWebOAuthRedirectURI(); got != "http://example.test/box/callback" {
		t.Fatalf("redirect uri=%q", got)
	}
}

func TestStartBoxWebOAuthRequiresClientCredentials(t *testing.T) {
	svc := NewConnectorService()

	_, code, err := svc.StartBoxWebOAuth("user-1", &StartBoxWebOAuthRequest{ClientID: "client-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if code != common.CodeArgumentError {
		t.Fatalf("code=%v want=%v", code, common.CodeArgumentError)
	}
	if err.Error() != "Box client_id and client_secret are required." {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestPollBoxWebOAuthResultPendingWithoutRedis(t *testing.T) {
	svc := NewConnectorService()

	_, code, err := svc.PollBoxWebOAuthResult("user-1", &PollBoxWebOAuthResultRequest{FlowID: "flow-1"})
	if err == nil {
		t.Fatal("expected pending error")
	}
	if code != common.CodeRunning {
		t.Fatalf("code=%v want=%v", code, common.CodeRunning)
	}
	if err.Error() != "Authorization is still pending." {
		t.Fatalf("error=%q", err.Error())
	}
}
