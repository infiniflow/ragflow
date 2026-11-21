// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestXiaomiOCRMarkdown(t *testing.T) {
	code := "```text\n<fcel>example<nl>\n```"
	table := "<fcel>A & B<lcel><fcel>C<nl>\n<ucel><xcel><fcel>D<nl>"
	got := xiaomiOCRMarkdown("Cost \\(x+y\\)\n\n\\[a=b\\]\n\n" + table + "\n\n" + code)
	for _, want := range []string{"$x+y$", "$$a=b$$", `<td rowspan="2" colspan="2">A &amp; B</td><td>C</td>`, "<tr><td>D</td></tr>", code} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}

func TestXiaomiOCRTableBoundaries(t *testing.T) {
	fence := "~~~text\n" + `<fcel>\(literal\)<nl>` + "\n~~~"
	for _, tc := range []struct{ name, input, want string }{
		{
			"multiline cells",
			"Before\n<fcel>A\nB<fcel>C<nl>\n<fcel>D\n\nE\n<nl>\nAfter",
			"Before\n\n<table><tr><td>A<br>B</td><td>C</td></tr><tr><td>D<br><br>E</td></tr></table>\n\nAfter",
		},
		{
			"cell math",
			`<fcel>\(x<y\)<fcel>\[a=b\]<nl>` + "\n" + fence,
			"\n" + `<table><tr><td>$x&lt;y$</td><td>$$a=b$$</td></tr></table>` + "\n\n" + fence,
		},
		{
			"unterminated row next to prose and a fence",
			"<fcel>A\nAfter\n" + fence,
			"\n<table><tr><td>A</td></tr></table>\n\nAfter\n" + fence,
		},
		{
			"separate tables around a paragraph",
			"<fcel>A\n\nBetween tables.\n\n<fcel>B<nl>",
			"\n<table><tr><td>A</td></tr></table>\n\n\nBetween tables.\n\n\n<table><tr><td>B</td></tr></table>\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := xiaomiOCRMarkdown(tc.input); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestXiaomiOCRRejectsIncompletePage(t *testing.T) {
	for _, body := range []string{`{"choices":[]}`, `{"choices":[{"finish_reason":"length","message":{"content":"partial"}}]}`, `{"choices":[{"message":{}}]}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if key := r.Header.Get("Authorization"); key != "" {
					t.Errorf("unexpected credentials: %q", key)
				}
				_, _ = fmt.Fprint(w, body)
			}))
			defer srv.Close()
			key := fmt.Sprintf(`{"%s":%q}`, common.EnvXiaomiOCRServerURL, srv.URL)
			config := xiaomiOCRConfig(&APIConfig{ApiKey: &key})
			driver := &xiaomiOCRModel{NewVllmModel(nil, URLSuffix{})}
			driver.baseModel.httpClient = srv.Client()
			if text, err := driver.recognizePage(t.Context(), *config.BaseURL, "served-model", "data:image/png;base64,test", config, &TokenUsage{}); err == nil || text != "" {
				t.Fatalf("text=%q, err=%v", text, err)
			}
		})
	}
}
