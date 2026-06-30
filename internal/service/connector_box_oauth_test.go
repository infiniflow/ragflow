package service

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ragflow/internal/common"
	redisengine "ragflow/internal/engine/redis"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func forceNilConnectorRedis(t *testing.T) {
	t.Helper()
	previous := connectorRedisGet
	connectorRedisGet = func() *redisengine.RedisClient { return nil }
	t.Cleanup(func() {
		connectorRedisGet = previous
	})
}

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

func TestExchangeBoxAuthorizationCodeRequestBody(t *testing.T) {
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })

	var form url.Values
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method=%s", req.Method)
			}
			if req.URL.String() != boxOAuthTokenURL {
				t.Fatalf("url=%s", req.URL.String())
			}
			if got := req.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
				t.Fatalf("content-type=%q", got)
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			form, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"access-1","refresh_token":"refresh-1"}`)),
			}, nil
		}),
	}

	token, err := exchangeBoxAuthorizationCode("client-1", "secret-1", "http://localhost/callback", "code-1")
	if err != nil {
		t.Fatalf("exchangeBoxAuthorizationCode: %v", err)
	}
	if token.AccessToken != "access-1" || token.RefreshToken != "refresh-1" {
		t.Fatalf("token=%#v", token)
	}

	want := map[string]string{
		"grant_type":    "authorization_code",
		"code":          "code-1",
		"client_id":     "client-1",
		"client_secret": "secret-1",
	}
	for key, value := range want {
		if form.Get(key) != value {
			t.Fatalf("%s=%q want=%q", key, form.Get(key), value)
		}
	}
	if got := form.Get("redirect_uri"); got != "" {
		t.Fatalf("redirect_uri should not be sent to Box token endpoint, got %q", got)
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
	forceNilConnectorRedis(t)

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
