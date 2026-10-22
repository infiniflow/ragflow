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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

func decodeAnySearchExtractEnvelope(t *testing.T, out string) anySearchExtractEnvelope {
	t.Helper()
	var env anySearchExtractEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", out, err)
	}
	return env
}

// anySearchExtractBody is a successful /v1/extract reply for one page.
func anySearchExtractBody(pageURL, title, content string) string {
	return anySearchExtractJSON(map[string]any{
		"code": 0, "message": "success", "request_id": "rid-ok",
		"data": map[string]any{"url": pageURL, "title": title, "content": content},
	})
}

func anySearchExtractJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// anySearchExtractPage answers every request with a successful page.
func anySearchExtractPage(pageURL, title, content string) func(http.ResponseWriter, *http.Request, []byte) {
	return func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, http.StatusOK, anySearchExtractBody(pageURL, title, content))
	}
}

func runAnySearchExtract(t *testing.T, tool *AnySearchExtractTool, args string) (string, anySearchExtractEnvelope) {
	t.Helper()
	out, err := tool.InvokableRun(t.Context(), args)
	if err != nil {
		t.Fatalf("Go error %v: recoverable errors must go back to the model", err)
	}
	return out, decodeAnySearchExtractEnvelope(t, out)
}

func anySearchExtractCall(pageURL string) string {
	return anySearchExtractJSON(map[string]any{"url": pageURL})
}

func TestAnySearchExtract_RejectedURLsMakeNoRequest(t *testing.T) {
	t.Parallel()
	const key = "secret-key-123"
	const notAbsolute = "absolute http or https"
	const notPublic = "not a public"
	cases := []struct{ name, url, wantErr string }{
		{"empty", "", "url is required"},
		{"blank", "   ", "url is required"},
		{"relative host and path", "example.com/a", notAbsolute},
		{"relative path", "/local/path", notAbsolute},
		{"scheme-relative", "//example.com/x", notAbsolute},
		{"ftp", "ftp://example.com/f", notAbsolute},
		{"javascript", "javascript:alert(1)", notAbsolute},
		{"file", "file:///etc/passwd", notAbsolute},
		{"data", "data:text/html,hi", notAbsolute},
		{"mailto", "mailto:a@example.com", notAbsolute},
		{"no host", "https://", notAbsolute},
		{"no host with path", "https:///path", notAbsolute},
		{"opaque", "http:example.com", notAbsolute},
		{"control character", "https://example.com/\x00", notAbsolute},
		{"space in host", "https://exa mple.com/", notAbsolute},
		{"unterminated IPv6", "http://[::1", notAbsolute},
		{"bad port", "https://example.com:abc/", notAbsolute},
		{"user and password", "https://user:hunter2@example.com/", "credentials"},
		{"user only", "https://user@example.com/", "credentials"},
		{"empty userinfo", "https://@example.com/", "credentials"},
		{"userinfo hiding a private host", "https://example.com@127.0.0.1/", "credentials"},
		{"localhost", "http://localhost/", notPublic},
		{"localhost with port", "http://localhost:8080/x", notPublic},
		{"localhost upper case and dot", "http://LOCALHOST./", notPublic},
		{"localhost subdomain", "http://app.localhost/", notPublic},
		{"mdns name", "http://printer.local/", notPublic},
		{"internal name", "http://db.internal/", notPublic},
		{"metadata name", "http://metadata.google.internal/", notPublic},
		{"single label", "http://intranet/", notPublic},
		{"single label with port", "http://wiki:8080/", notPublic},
		{"IPv4 loopback", "http://127.0.0.1:8080/", notPublic},
		{"IPv4 loopback range", "http://127.1.2.3/", notPublic},
		{"IPv4 10/8", "http://10.0.0.5/", notPublic},
		{"IPv4 172.16/12 low", "http://172.16.0.1/", notPublic},
		{"IPv4 172.16/12 high", "http://172.31.255.255/", notPublic},
		{"IPv4 192.168/16", "http://192.168.1.1/", notPublic},
		{"IPv4 link-local metadata", "http://169.254.169.254/latest/meta-data/", notPublic},
		{"IPv4 unspecified", "http://0.0.0.0/", notPublic},
		{"IPv4 multicast", "http://224.0.0.1/", notPublic},
		{"IPv4 shared address space", "http://100.64.0.1/", notPublic},
		{"IPv6 loopback", "http://[::1]/", notPublic},
		{"IPv6 unspecified", "http://[::]/", notPublic},
		{"IPv6 unique local", "http://[fd00::1]/", notPublic},
		{"IPv6 link-local", "http://[fe80::1]/", notPublic},
		{"IPv6 link-local with zone", "http://[fe80::1%25eth0]/", notPublic},
		{"IPv6 multicast", "http://[ff02::1]/", notPublic},
		{"IPv4-mapped loopback", "http://[::ffff:127.0.0.1]/", notPublic},
		{"decimal IPv4", "http://2130706433/", notPublic},
		{"short IPv4", "http://127.1/", notPublic},
		{"hex IPv4", "http://0x7f.0.0.1/", notPublic},
		{"octal IPv4", "http://0177.0.0.1/", notPublic},
		{"over-long", "https://example.com/" + strings.Repeat("a", 2048), "longer than 2048"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "T", "C"))
			out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, key), anySearchExtractCall(tc.url))
			if !strings.Contains(env.Error, tc.wantErr) {
				t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
			}
			if env.URL != "" || env.Title != "" || env.Content != "" || env.ContentChars != 0 || env.Truncated {
				t.Errorf("envelope = %+v, want every page field empty", env)
			}
			if n := len(rec.all()); n != 0 {
				t.Errorf("%d upstream requests, want 0", n)
			}
			for _, leak := range []string{key, "hunter2", "user:", "@127.0.0.1"} {
				if strings.Contains(out, leak) {
					t.Errorf("result leaks %q: %s", leak, out)
				}
			}
			if trimmed := strings.TrimSpace(tc.url); len(trimmed) > 12 && strings.Contains(out, trimmed) {
				t.Errorf("result echoes the rejected URL: %s", out)
			}
		})
	}
}

func TestAnySearchExtract_AcceptedURLsAreSentTrimmedAndUnchanged(t *testing.T) {
	t.Parallel()
	longURL := "https://example.com/" + strings.Repeat("a", 2048-len("https://example.com/"))
	cases := []struct{ name, in, sent string }{
		{"https", "https://example.com/", "https://example.com/"},
		{"http with query and fragment", "http://example.com/a?b=c#frag", "http://example.com/a?b=c#frag"},
		{"surrounding whitespace", "  https://example.com/x \n", "https://example.com/x"},
		{"IDN", "https://münchen.de/straße", "https://münchen.de/straße"},
		{"punycode", "https://xn--mnchen-3ya.de/", "https://xn--mnchen-3ya.de/"},
		{"public IPv4", "https://8.8.8.8/", "https://8.8.8.8/"},
		{"public IPv6", "https://[2606:4700:4700::1111]/", "https://[2606:4700:4700::1111]/"},
		{"explicit port", "https://example.com:8443/x", "https://example.com:8443/x"},
		{"upper-case scheme and host", "HTTPS://Example.COM/", "HTTPS://Example.COM/"},
		{"fully qualified trailing dot", "https://example.com./", "https://example.com./"},
		{"two labels", "https://a.b/", "https://a.b/"},
		{"exactly 2048 bytes", longURL, longURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "T", "C"))
			_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), anySearchExtractCall(tc.in))
			if env.Error != "" {
				t.Fatalf("_ERROR = %q, want success", env.Error)
			}
			seen := rec.all()
			if len(seen) != 1 {
				t.Fatalf("%d requests, want 1", len(seen))
			}
			body := decodeAnySearchBody(t, seen[0].Body)
			if len(body) != 1 || body["url"] != tc.sent {
				t.Errorf("body = %v, want only url %q", body, tc.sent)
			}
		})
	}
}

func TestAnySearchExtract_AnonymousSendsNoAuthorizationHeader(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"", "   ", "\t\n"} {
		helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "T", "C"))
		_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, key), `{"url":"https://example.com/"}`)
		if env.Error != "" {
			t.Fatalf("key %q: unexpected _ERROR %q", key, env.Error)
		}
		seen := rec.all()
		if len(seen) != 1 {
			t.Fatalf("key %q: %d requests, want 1", key, len(seen))
		}
		if seen[0].HasAuthorization {
			t.Errorf("key %q: Authorization header present (%q), want absent", key, seen[0].Authorization)
		}
		if seen[0].Method != http.MethodPost || seen[0].Path != "/v1/extract" || seen[0].ContentType != "application/json" || seen[0].RawQuery != "" {
			t.Errorf("key %q: request = %s %s?%s (%s)", key, seen[0].Method, seen[0].Path, seen[0].RawQuery, seen[0].ContentType)
		}
	}
}

func TestAnySearchExtract_KeyedSendsTrimmedBearer(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "T", "C"))
	runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, " key-xyz "), `{"url":"https://example.com/"}`)
	seen := rec.all()
	if len(seen) != 1 || seen[0].Authorization != "Bearer key-xyz" {
		t.Fatalf("requests = %+v, want one with Authorization %q", seen, "Bearer key-xyz")
	}
}

func TestAnySearchExtract_SuccessEnvelope(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "Example Domain", "This domain is for use in documentation examples."))
	out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), `{"url":"https://example.com/"}`)
	want := anySearchExtractEnvelope{
		URL: "https://example.com/", Title: "Example Domain", Content: "This domain is for use in documentation examples.",
		ContentChars: 49, Truncated: false, RequestID: "rid-ok",
	}
	if env != want {
		t.Errorf("envelope = %+v, want %+v", env, want)
	}
	if strings.Contains(out, `"_ERROR"`) {
		t.Errorf("a success must not carry an _ERROR field: %s", out)
	}
	for _, field := range []string{`"url"`, `"title"`, `"content"`, `"content_chars"`, `"truncated"`, `"request_id"`} {
		if !strings.Contains(out, field) {
			t.Errorf("result lacks %s: %s", field, out)
		}
	}
}

func TestAnySearchExtract_ResponseStrictness(t *testing.T) {
	t.Parallel()
	page := func(data string) string {
		return `{"code":0,"message":"success","request_id":"rid-ok","data":` + data + `}`
	}
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string // substring of _ERROR; empty means success
		title   string
		chars   int
	}{
		{"blank title", 200, page(`{"url":"https://e.example/","title":"","content":"abc"}`), "", "", 3},
		{"missing title", 200, page(`{"url":"https://e.example/","content":"abc"}`), "", "", 3},
		{"null title", 200, page(`{"url":"https://e.example/","title":null,"content":"abc"}`), "", "", 3},
		{"title with text", 200, page(`{"url":"https://e.example/","title":"Hi","content":"abc"}`), "", "Hi", 3},
		{"empty content is a valid page", 200, page(`{"url":"https://e.example/","title":"T","content":""}`), "", "T", 0},
		{"missing content", 200, page(`{"url":"https://e.example/","title":"T"}`), "anysearch: invalid extract result (request_id rid-ok)", "", 0},
		{"null content", 200, page(`{"url":"https://e.example/","title":"T","content":null}`), "invalid extract result", "", 0},
		{"numeric content", 200, page(`{"url":"https://e.example/","title":"T","content":5}`), "invalid extract result", "", 0},
		{"object content", 200, page(`{"url":"https://e.example/","title":"T","content":{"a":"b"}}`), "invalid extract result", "", 0},
		{"numeric title", 200, page(`{"url":"https://e.example/","title":5,"content":"abc"}`), "invalid extract result", "", 0},
		{"missing url", 200, page(`{"title":"T","content":"abc"}`), "invalid extract result", "", 0},
		{"numeric url", 200, page(`{"url":5,"title":"T","content":"abc"}`), "invalid extract result", "", 0},
		{"null data", 200, `{"code":0,"request_id":"rid-ok","data":null}`, "missing data", "", 0},
		{"array data", 200, `{"code":0,"request_id":"rid-ok","data":[]}`, "missing data", "", 0},
		{"missing code", 200, `{"data":{"url":"u","title":"","content":"c"}}`, "missing business status", "", 0},
		{"string code", 200, `{"code":"0","data":{"url":"u","title":"","content":"c"}}`, "missing business status", "", 0},
		{"business failure on 200", 200, `{"code":1002,"message":"quota","request_id":"rid-b","data":{}}`, "business error 1002: quota (request_id rid-b)", "", 0},
		{"vendor 400 invalid url", 400, `{"code":-1,"message":"URL is invalid.","request_id":"rid-400","error_code":"invalid_extract_url"}`, "anysearch: HTTP 400: URL is invalid. (request_id rid-400)", "", 0},
		{"vendor 422 extract failed", 422, `{"code":-1,"message":"Unable to extract content from the URL.","request_id":"rid-422","error_code":"extract_failed"}`, "anysearch: HTTP 422: Unable to extract content from the URL. (request_id rid-422)", "", 0},
		{"403 with an HTML body", 403, `<!DOCTYPE HTML><HTML><HEAD><TITLE>ERROR: The request could not be satisfied</TITLE></HEAD></HTML>`, "anysearch: HTTP 403", "", 0},
		{"401", 401, `{"code":401,"message":"invalid api key","request_id":"rid-401"}`, "HTTP 401", "", 0},
		{"429", 429, `{"code":429,"message":"quota exceeded","request_id":"rid-429"}`, "HTTP 429", "", 0},
		{"500 is not retried", 500, `{"message":"boom"}`, "HTTP 500", "", 0},
		{"non-JSON gateway error", 502, `<html>bad gateway</html>`, "HTTP 502", "", 0},
		{"invalid JSON", 200, `{"code":0,`, "invalid JSON", "", 0},
		{"trailing data", 200, page(`{"url":"u","title":"","content":"c"}`) + ` {}`, "invalid JSON", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
				writeAnySearch(w, tc.status, tc.body)
			})
			out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, "key"), `{"url":"https://e.example/"}`)
			if tc.wantErr == "" {
				if env.Error != "" || env.Title != tc.title || env.ContentChars != tc.chars || env.URL != "https://e.example/" {
					t.Fatalf("envelope = %+v, want a success with title %q and %d chars", env, tc.title, tc.chars)
				}
			} else {
				if !strings.Contains(env.Error, tc.wantErr) {
					t.Errorf("_ERROR = %q, want it to contain %q", env.Error, tc.wantErr)
				}
				if env.URL != "" || env.Title != "" || env.Content != "" || env.ContentChars != 0 || env.Truncated {
					t.Errorf("a failure must leave every page field empty: %+v", env)
				}
			}
			if strings.Contains(out, "could not be satisfied") || strings.Contains(out, "<html") {
				t.Errorf("the raw upstream body reached the model: %s", out)
			}
			if n := len(rec.all()); n != 1 {
				t.Errorf("%d upstream requests, want 1 (no retry of a POST)", n)
			}
		})
	}
}

func TestAnySearchExtract_FailureKeepsTheRequestID(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeAnySearch(w, 422, `{"code":-1,"message":"Unable to extract content from the URL.","request_id":"rid-422","error_code":"extract_failed"}`)
	})
	_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), `{"url":"https://e.example/doc.pdf"}`)
	if env.RequestID != "rid-422" {
		t.Errorf("request_id = %q, want rid-422 kept for diagnostics", env.RequestID)
	}
}

func TestAnySearchExtract_ArgumentsAndNodeDefault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, args, nodeURL string
		wantURL             string // empty means no request and an error
		wantErr             string
	}{
		{"model url", `{"url":"https://a.example/x"}`, "", "https://a.example/x", ""},
		{"model url wins over the node url", `{"url":"https://a.example/x"}`, "https://node.example/", "https://a.example/x", ""},
		{"node url when the model gives none", `{}`, "https://node.example/", "https://node.example/", ""},
		{"node url when the argument is blank", `{"url":"  "}`, " https://node.example/ ", "https://node.example/", ""},
		{"blank arguments use the node url", ``, "https://node.example/", "https://node.example/", ""},
		{"null arguments use the node url", `null`, "https://node.example/", "https://node.example/", ""},
		{"neither", `{}`, "", "", "url is required"},
		{"node url is validated at run time", `{}`, "{begin@url}", "", "absolute http or https"},
		{"private node url", `{}`, "http://127.0.0.1/", "", "not a public"},
		{"unparseable arguments", `{"url":`, "https://node.example/", "", "parse arguments"},
		{"trailing data", `{"url":"https://a.example/"}{"url":"https://b.example/"}`, "", "", "parse arguments"},
		{"url of the wrong type", `{"url":5}`, "https://node.example/", "", "parse arguments"},
		{"unknown fields are ignored", `{"url":"https://a.example/","state":{"x":1},"query":"q"}`, "", "https://a.example/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "T", "C"))
			tool := newAnySearchExtractTool(helper, "", tc.nodeURL)
			_, env := runAnySearchExtract(t, tool, tc.args)
			seen := rec.all()
			if tc.wantURL == "" {
				if !strings.Contains(env.Error, tc.wantErr) || len(seen) != 0 {
					t.Fatalf("_ERROR = %q with %d requests, want %q and 0 requests", env.Error, len(seen), tc.wantErr)
				}
				return
			}
			if env.Error != "" || len(seen) != 1 {
				t.Fatalf("_ERROR = %q with %d requests, want a success with 1 request", env.Error, len(seen))
			}
			if body := decodeAnySearchBody(t, seen[0].Body); body["url"] != tc.wantURL {
				t.Errorf("sent url = %v, want %q", body["url"], tc.wantURL)
			}
		})
	}
}

func TestAnySearchExtract_OversizedResponseIsRejected(t *testing.T) {
	t.Parallel()
	helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"url":"u","title":"","content":"`))
		_, _ = w.Write(bytes.Repeat([]byte("a"), anySearchMaxResponseBytes+16))
		_, _ = w.Write([]byte(`"}}`))
	})
	_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, "key"), `{"url":"https://e.example/"}`)
	if !strings.Contains(env.Error, "4 MiB") || env.Content != "" {
		t.Fatalf("envelope = %+v, want a 4 MiB limit error and no content", env)
	}
}

func TestAnySearchExtract_RedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		http.Redirect(w, r, "https://elsewhere.invalid/steal", http.StatusFound)
	})
	_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, "secret-key"), `{"url":"https://e.example/"}`)
	if !strings.Contains(env.Error, "HTTP 302") {
		t.Fatalf("_ERROR = %q, want an HTTP 302 error", env.Error)
	}
	if n := len(rec.all()); n != 1 {
		t.Errorf("%d requests, want 1: the redirect must not be followed", n)
	}
}

func TestAnySearchExtract_ErrorsNeverLeakTheKeyOrTheURL(t *testing.T) {
	t.Parallel()
	const key = "secret-key-123"
	const pageURL = "https://example.com/page?token=abc123&x=1"
	t.Run("vendor message is redacted", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("é", 500)
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 422, `{"code":-1,"message":"bad key `+key+` `+long+`","request_id":"rid-`+key+`"}`)
		})
		out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, key), anySearchExtractCall(pageURL))
		if strings.Contains(out, key) {
			t.Fatalf("the key leaked into the result: %s", out)
		}
		if !strings.Contains(env.Error, "[redacted]") || !utf8.ValidString(env.Error) || utf8.RuneCountInString(env.Error) > 400 {
			t.Errorf("error = %q, want a redacted, valid, truncated message", env.Error)
		}
	})
	t.Run("vendor message echoing the url", func(t *testing.T) {
		t.Parallel()
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			writeAnySearch(w, 422, `{"code":-1,"message":"Unable to extract `+pageURL+`; its query was token=abc123&x=1","request_id":"rid-1"}`)
		})
		out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), anySearchExtractCall(pageURL))
		for _, leak := range []string{"abc123", "token=", "example.com/page"} {
			if strings.Contains(out, leak) {
				t.Errorf("result leaks %q: %s", leak, out)
			}
		}
		if !strings.Contains(env.Error, "HTTP 422") || env.URL != "" {
			t.Errorf("envelope = %+v, want the HTTP 422 error with no page", env)
		}
	})
	t.Run("transport failure hides the address and the url", func(t *testing.T) {
		t.Parallel()
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("recorder cannot hijack")
				return
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		})
		out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, key), anySearchExtractCall(pageURL))
		if !strings.Contains(env.Error, "request failed") {
			t.Fatalf("envelope = %s, want a generic request failure", out)
		}
		for _, leak := range []string{key, "127.0.0.1", "abc123", "http://", "https://"} {
			if strings.Contains(out, leak) {
				t.Errorf("result leaks %q: %s", leak, out)
			}
		}
	})
}

func TestAnySearchExtract_TransportTimeoutAndCancel(t *testing.T) {
	t.Parallel()
	t.Run("timeout is reported without details", func(t *testing.T) {
		t.Parallel()
		helper := NewHTTPHelper().WithClient(&http.Client{Transport: anySearchRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, anySearchTimeoutError{}
		})})
		out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, "secret"), `{"url":"https://e.example/"}`)
		if env.Error != "anysearch: request timed out" || strings.Contains(out, "secret") || strings.Contains(out, "api.anysearch.com") {
			t.Fatalf("envelope = %s, want the generic timeout error", out)
		}
	})
	t.Run("a cancelled context returns promptly", func(t *testing.T) {
		t.Parallel()
		helper, _ := anySearchTestServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
			<-r.Context().Done()
		})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		done := make(chan string, 1)
		go func() {
			out, _ := NewAnySearchExtractToolWith(helper, "key").InvokableRun(ctx, `{"url":"https://e.example/"}`)
			done <- out
		}()
		select {
		case out := <-done:
			if env := decodeAnySearchExtractEnvelope(t, out); !strings.Contains(env.Error, "canceled") {
				t.Errorf("envelope = %s, want a canceled error", out)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a cancelled context did not return")
		}
	})
}

func TestAnySearchExtract_TruncationDetection(t *testing.T) {
	t.Parallel()
	marker := anySearchExtractTruncationMarker
	// filler uses a two-byte rune so the count is by characters, not bytes.
	filler := func(runes int) string { return strings.Repeat("é", runes) }
	cases := []struct {
		name    string
		content string
		chars   int
		want    bool
	}{
		{"empty", "", 0, false},
		{"short page", "hello", 5, false},
		{"marker on a short page", "partial text\n\n" + marker, utf8.RuneCountInString("partial text\n\n" + marker), true},
		{"marker alone", marker, utf8.RuneCountInString(marker), true},
		{"marker in the middle only", "a " + marker + " b", utf8.RuneCountInString("a " + marker + " b"), false},
		{"vendor limit with marker", filler(50000-len([]rune(marker))) + marker, 50000, true},
		{"vendor limit without marker", filler(50000), 50000, true},
		{"one above the floor", filler(anySearchExtractTruncationFloor + 1), anySearchExtractTruncationFloor + 1, true},
		{"exactly the floor", filler(anySearchExtractTruncationFloor), anySearchExtractTruncationFloor, true},
		{"one below the floor", filler(anySearchExtractTruncationFloor - 1), anySearchExtractTruncationFloor - 1, false},
		{"large page below the floor", filler(30000), 30000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			helper, _ := anySearchTestServer(t, anySearchExtractPage("https://e.example/", "T", tc.content))
			_, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), `{"url":"https://e.example/"}`)
			if env.Error != "" {
				t.Fatalf("_ERROR = %q", env.Error)
			}
			if env.Truncated != tc.want || env.ContentChars != tc.chars {
				t.Errorf("truncated=%v content_chars=%d, want truncated=%v content_chars=%d", env.Truncated, env.ContentChars, tc.want, tc.chars)
			}
			if env.Content != tc.content {
				t.Errorf("the content was changed: got %d bytes, want %d", len(env.Content), len(tc.content))
			}
		})
	}
	if anySearchExtractTruncationFloor != 49000 {
		t.Errorf("anySearchExtractTruncationFloor = %d, want 49000", anySearchExtractTruncationFloor)
	}
}

func TestAnySearchExtract_ModelResultIsOneCopyOfThePage(t *testing.T) {
	t.Parallel()
	var page strings.Builder
	for i := 0; page.Len() < 50000; i++ {
		fmt.Fprintf(&page, "line %d of the page\n", i)
	}
	content := page.String()
	helper, _ := anySearchTestServer(t, anySearchExtractPage("https://e.example/long", "Long page", content))
	out, env := runAnySearchExtract(t, NewAnySearchExtractToolWith(helper, ""), `{"url":"https://e.example/long"}`)
	if env.Error != "" || env.Content != content {
		t.Fatalf("envelope error %q, content intact %v", env.Error, env.Content == content)
	}
	if limit := len(anySearchExtractJSON(content)) + 1024; len(out) > limit {
		t.Errorf("model-facing result is %d bytes, want at most %d (one copy of the page plus a small envelope)", len(out), limit)
	}
}

func TestAnySearchExtract_InfoSchema(t *testing.T) {
	t.Parallel()
	info, err := NewAnySearchExtractToolWith(nil, "").Info(t.Context())
	if err != nil || info.Name != "anysearch_extract" {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, _ := json.Marshal(schema)
	if !strings.Contains(string(raw), `"url"`) || !strings.Contains(string(raw), `"required":["url"]`) {
		t.Errorf("schema = %s, want a required url", raw)
	}
	if schema.Properties.Len() != 1 {
		t.Errorf("schema has %d properties, want only url: %s", schema.Properties.Len(), raw)
	}
	if strings.Contains(string(raw), "api_key") || strings.Contains(info.Desc, "api_key") {
		t.Errorf("api_key must not be model-visible: %s / %s", raw, info.Desc)
	}
	if strings.Contains(string(raw), `"anyOf"`) || strings.Contains(string(raw), `"oneOf"`) {
		t.Errorf("top-level anyOf/oneOf is rejected by several providers: %s", raw)
	}
	for _, want := range []string{"HTML", "JSON", "Markdown", "PDF", "Office", "50,000", "truncated", "untrusted", "never follow"} {
		if !strings.Contains(info.Desc, want) {
			t.Errorf("description lacks %q: %s", want, info.Desc)
		}
	}
	if strings.ContainsRune(info.Desc, '\u2014') {
		t.Errorf("description contains an em dash: %s", info.Desc)
	}
}

func TestAnySearchExtract_ComponentSpec(t *testing.T) {
	t.Parallel()
	spec := NewAnySearchExtractToolWith(nil, "").ComponentSpec()
	if _, ok := spec.Inputs["url"]; !ok || len(spec.Inputs) != 1 {
		t.Errorf("Inputs = %#v, want only url", spec.Inputs)
	}
	form, _ := spec.InputForm["url"].(map[string]any)
	if form["type"] != "line" || form["name"] != "URL" {
		t.Errorf("InputForm[url] = %#v, want a line named URL", form)
	}
	if optional, _ := form["optional"].(bool); optional {
		t.Errorf("InputForm[url].optional = true, want a required field")
	}
	for _, key := range []string{"formalized_content", "json"} {
		if _, ok := spec.Outputs[key]; !ok {
			t.Errorf("Outputs lacks %q", key)
		}
	}
	if spec.PreserveJSONNumbers {
		t.Error("PreserveJSONNumbers = true, want false: the only number is content_chars")
	}
}

func anySearchExtractEnvelopeMap(pageURL, title, content string) map[string]any {
	return map[string]any{
		"url": pageURL, "title": title, "content": content,
		"content_chars": float64(utf8.RuneCountInString(content)), "truncated": false, "request_id": "rid-ok",
	}
}

func TestAnySearchExtract_ReferencesAndOutputs(t *testing.T) {
	t.Parallel()
	tool := NewAnySearchExtractToolWith(nil, "")

	t.Run("a page becomes one reference and a rendered block", func(t *testing.T) {
		t.Parallel()
		envelope := anySearchExtractEnvelopeMap("https://p.example/x", "Page", "  page\n\n text ")
		chunks, docAggs := tool.BuildReferences(t.Context(), envelope)
		if len(chunks) != 1 || len(docAggs) != 1 {
			t.Fatalf("got %d chunks / %d docs, want 1 / 1", len(chunks), len(docAggs))
		}
		if chunks[0]["content"] != "page text" || chunks[0]["url"] != "https://p.example/x" || chunks[0]["docnm_kwd"] != "Page" || chunks[0]["id"] == "" {
			t.Errorf("chunk = %#v, want content with whitespace collapsed, url, title and an id", chunks[0])
		}
		if docAggs[0]["doc_name"] != "Page" || docAggs[0]["url"] != "https://p.example/x" || docAggs[0]["doc_id"] != chunks[0]["id"] {
			t.Errorf("docAgg = %#v", docAggs[0])
		}
		outputs := tool.BuildComponentOutputs(envelope)
		rendered, _ := outputs["formalized_content"].(string)
		for _, want := range []string{"Title: Page", "URL: https://p.example/x", "Content:\npage\n\n text"} {
			if !strings.Contains(rendered, want) {
				t.Errorf("formalized_content missing %q:\n%s", want, rendered)
			}
		}
		page, _ := outputs["json"].(map[string]any)
		if page["title"] != "Page" || page["url"] != "https://p.example/x" || page["request_id"] != "rid-ok" || page["truncated"] != false {
			t.Errorf("json = %#v, want the page object", outputs["json"])
		}
		if _, has := page["_ERROR"]; has {
			t.Errorf("json carries _ERROR: %#v", page)
		}
	})

	t.Run("no reference without text or an http(s) url", func(t *testing.T) {
		t.Parallel()
		for name, envelope := range map[string]map[string]any{
			"empty content":  anySearchExtractEnvelopeMap("https://p.example/", "T", ""),
			"blank content":  anySearchExtractEnvelopeMap("https://p.example/", "T", " \n\t "),
			"script url":     anySearchExtractEnvelopeMap("javascript:alert(1)", "T", "text"),
			"relative url":   anySearchExtractEnvelopeMap("/local", "T", "text"),
			"no host":        anySearchExtractEnvelopeMap("https://", "T", "text"),
			"failed extract": {"url": "", "title": "", "content": "", "content_chars": float64(0), "truncated": false, "_ERROR": "anysearch: HTTP 422"},
			"empty":          {},
		} {
			if chunks, docAggs := tool.BuildReferences(t.Context(), envelope); len(chunks) != 0 || len(docAggs) != 0 {
				t.Errorf("%s: got %d chunks / %d docs, want none", name, len(chunks), len(docAggs))
			}
		}
	})

	t.Run("reference ids are unique per url and content", func(t *testing.T) {
		t.Parallel()
		ids := map[string]bool{}
		for i := range 50 {
			chunks, _ := tool.BuildReferences(t.Context(), anySearchExtractEnvelopeMap(fmt.Sprintf("https://p.example/%d", i), "T", fmt.Sprintf("distinct text %d", i)))
			if len(chunks) != 1 {
				t.Fatalf("page %d produced %d chunks, want 1", i, len(chunks))
			}
			id, _ := chunks[0]["id"].(string)
			if id == "" || ids[id] {
				t.Fatalf("id %q is empty or repeated at %d", id, i)
			}
			ids[id] = true
		}
		again, _ := tool.BuildReferences(t.Context(), anySearchExtractEnvelopeMap("https://p.example/3", "T", "distinct text 3"))
		if len(again) != 1 || !ids[again[0]["id"].(string)] {
			t.Error("the same page produced a different id")
		}
	})

	t.Run("a failure renders nothing and keeps the page shape", func(t *testing.T) {
		t.Parallel()
		outputs := tool.BuildComponentOutputs(map[string]any{
			"url": "", "title": "", "content": "", "content_chars": float64(0), "truncated": false, "request_id": "rid-422", "_ERROR": "anysearch: HTTP 422",
		})
		if outputs["formalized_content"] != "" {
			t.Errorf("formalized_content = %q, want empty", outputs["formalized_content"])
		}
		page, ok := outputs["json"].(map[string]any)
		if !ok || page["content"] != "" || page["request_id"] != "rid-422" {
			t.Errorf("json = %#v, want the empty page object", outputs["json"])
		}
		if _, has := page["_ERROR"]; has {
			t.Errorf("json carries _ERROR: %#v", page)
		}
		if empty, _ := tool.BuildComponentOutputs(map[string]any{})["json"].(map[string]any); empty == nil {
			t.Error("json must be an object, never nil")
		}
	})
}

func TestAnySearchExtract_RenderCountsTheWholeBlock(t *testing.T) {
	t.Parallel()
	content := strings.TrimSpace(strings.Repeat("alpha ", 200))
	page := map[string]any{"title": "A title", "url": "https://x.example/a", "content": content}

	full := renderAnySearchExtract(page, 0)
	if !strings.Contains(full, content) || !strings.Contains(full, "Title: A title") {
		t.Fatalf("no budget should render everything:\n%s", full)
	}
	contentTokens := tokenizer.NumTokensFromString(content)
	blockTokens := tokenizer.NumTokensFromString(full)
	if blockTokens <= contentTokens {
		t.Fatalf("test setup: block (%d tokens) must be larger than its content (%d)", blockTokens, contentTokens)
	}
	// A budget the content alone fits but the whole block, header included, does not.
	budget := int(float64(contentTokens)/0.97) + 1
	if float64(blockTokens) <= float64(budget)*0.97 {
		t.Fatalf("test setup: budget %d must be below the whole block (%d tokens)", budget, blockTokens)
	}
	limited := renderAnySearchExtract(page, budget)
	if strings.Contains(limited, content) {
		t.Errorf("a block over the budget must not carry the content:\n%s", limited)
	}
	if !strings.Contains(limited, "Title: A title") || !strings.Contains(limited, "URL: https://x.example/a") || !strings.Contains(limited, "larger than the prompt budget") {
		t.Errorf("an omitted page must still name its title and url and say why:\n%s", limited)
	}
	if fits := renderAnySearchExtract(page, blockTokens*2); !strings.Contains(fits, content) {
		t.Errorf("a block inside the budget must be rendered in full:\n%s", fits)
	}
}

func TestAnySearchExtract_BuildByNameUsesNodeConfig(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"AnySearchExtract", "anysearch_extract", "anysearchextract"} {
		built, err := BuildByName(name, map[string]any{
			"api_key": " stored-key ", "url": "https://node.example/", "outputs": map[string]any{}, "function_name": "x", "top_n": float64(3),
		})
		if err != nil {
			t.Fatalf("BuildByName(%q): %v", name, err)
		}
		tool, ok := built.(*AnySearchExtractTool)
		if !ok {
			t.Fatalf("BuildByName(%q) = %T, want *AnySearchExtractTool", name, built)
		}
		if tool.client.apiKey != "stored-key" || tool.defaultURL != "https://node.example/" {
			t.Errorf("BuildByName(%q) = key %q url %q", name, tool.client.apiKey, tool.defaultURL)
		}
		if _, ok := built.(ToolComponent); !ok {
			t.Errorf("BuildByName(%q) does not implement ToolComponent", name)
		}
		if _, ok := built.(ReferenceBuilder); !ok {
			t.Errorf("BuildByName(%q) does not implement ReferenceBuilder", name)
		}
	}
	for _, params := range []map[string]any{nil, {}, {"api_key": nil}, {"api_key": ""}, {"api_key": "  "}, {"url": nil}} {
		built, err := BuildByName("anysearch_extract", params)
		if err != nil {
			t.Errorf("anonymous config %v rejected: %v", params, err)
			continue
		}
		if tool := built.(*AnySearchExtractTool); tool.client.apiKey != "" || tool.defaultURL != "" {
			t.Errorf("anonymous config %v = key %q url %q", params, tool.client.apiKey, tool.defaultURL)
		}
	}
	if built, err := BuildByName("anysearch_extract", map[string]any{"api_key": "  abc \n"}); err != nil || built.(*AnySearchExtractTool).client.apiKey != "abc" {
		t.Errorf("a key pasted with a trailing newline: %v", err)
	}
}

func TestAnySearchExtract_BuildByNameRejectsInvalidNodeConfig(t *testing.T) {
	t.Parallel()
	for name, params := range map[string]map[string]any{
		"api_key not a string": {"api_key": 42},
		"key with newline":     {"api_key": "abc\ndef"},
		"key with CR":          {"api_key": "abc\rdef"},
		"url not a string":     {"url": []any{"https://example.com"}},
		"url a number":         {"url": 5},
	} {
		if _, err := BuildByName("anysearch_extract", params); err == nil {
			t.Errorf("%s: accepted the config, want an error", name)
		}
	}
}

// TestAnySearchExtract_RealReactAgentRunsTheTool drives a real eino ReAct agent
// with all three AnySearch tools bound and a scripted model, so the tool name,
// the schema the model sees, argument decoding and the result fed back are
// exercised without depending on a real model's tool-calling quality.
func TestAnySearchExtract_RealReactAgentRunsTheTool(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, anySearchExtractPage("https://example.com/", "Example Domain", "This domain is for use in documentation examples."))
	extract := NewAnySearchExtractToolWith(helper, "")
	mdl := newReactScriptedModel(anySearchExtractToolName, `{"url":"https://example.com/"}`, "done")
	agent, err := react.NewAgent(t.Context(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig: compose.ToolsNodeConfig{Tools: []einotool.BaseTool{
			NewAnySearchToolWith(helper, ""), NewAnySearchDiscoverToolWith(helper, ""), extract,
		}},
		MaxStep: 5,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}
	out, err := agent.Generate(t.Context(), []*schema.Message{schema.UserMessage("read example.com")})
	if err != nil {
		t.Fatalf("agent.Generate: %v", err)
	}
	if out.Content != "done" || mdl.turn != 2 {
		t.Fatalf("content = %q after %d model turns, want %q after 2", out.Content, mdl.turn, "done")
	}

	bound := map[string]bool{}
	for _, info := range mdl.boundTools {
		bound[info.Name] = true
		jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			t.Fatalf("tool %s: ToJSONSchema: %v", info.Name, err)
		}
		raw, _ := json.Marshal(jsonSchema)
		if strings.Contains(string(raw), "api_key") {
			t.Errorf("tool %s exposes api_key to the model: %s", info.Name, raw)
		}
	}
	if !bound[anySearchExtractToolName] || !bound[anySearchToolName] || !bound[anySearchDiscoverToolName] || len(bound) != 3 {
		t.Errorf("tools bound to the model = %v, want the three AnySearch tools", bound)
	}

	var toolMessage string
	for _, msg := range mdl.rounds[1] {
		if msg.Role == schema.Tool {
			toolMessage = msg.Content
		}
	}
	env := decodeAnySearchExtractEnvelope(t, toolMessage)
	if env.Error != "" || env.Title != "Example Domain" || env.URL != "https://example.com/" || env.RequestID != "rid-ok" || env.ContentChars != 49 {
		t.Fatalf("tool message = %s, want the extracted page", toolMessage)
	}
	seen := rec.all()
	if len(seen) != 1 || seen[0].HasAuthorization || seen[0].Path != "/v1/extract" || seen[0].Method != http.MethodPost {
		t.Fatalf("requests = %+v, want one anonymous POST /v1/extract", seen)
	}
	if body := decodeAnySearchBody(t, seen[0].Body); len(body) != 1 || body["url"] != "https://example.com/" {
		t.Errorf("body = %v", body)
	}
}

func TestAnySearchExtract_OneToolServesSeveralCalls(t *testing.T) {
	t.Parallel()
	helper, rec := anySearchTestServer(t, func(w http.ResponseWriter, _ *http.Request, body []byte) {
		var sent struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(body, &sent)
		writeAnySearch(w, 200, anySearchExtractBody(sent.URL, "T for "+sent.URL, "text of "+sent.URL))
	})
	tool := NewAnySearchExtractToolWith(helper, "")
	for _, pageURL := range []string{"https://one.example/", "https://two.example/"} {
		_, env := runAnySearchExtract(t, tool, anySearchExtractCall(pageURL))
		if env.URL != pageURL || env.Content != "text of "+pageURL {
			t.Errorf("envelope = %+v, want the page for %s", env, pageURL)
		}
	}
	if n := len(rec.all()); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
}
