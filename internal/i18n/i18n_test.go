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

package i18n

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

func TestResolveLocalePriority(t *testing.T) {
	if got := ResolveLocale("ja", "Chinese", "zh_CN.UTF-8"); got != "ja" {
		t.Fatalf("header should win: got %q", got)
	}
	if got := ResolveLocale("zh-Hans", "English", "en_US.UTF-8"); got != "zh-Hans" {
		t.Fatalf("explicit app header should win: got %q", got)
	}
	if got := ResolveLocale("", "Chinese", "en_US.UTF-8"); got != "zh-Hans" {
		t.Fatalf("user language should win: got %q", got)
	}
	if got := ResolveLocale("", "zh-Hans", "C.UTF-8"); got != "zh-Hans" {
		t.Fatalf("user BCP47 should win: got %q", got)
	}
	if got := ResolveLocale("en-US,en;q=0.9", "zh-Hans", "C.UTF-8"); got != "zh-Hans" {
		t.Fatalf("browser list must not override user: got %q", got)
	}
	if got := ResolveLocale("en-US", "zh-Hans", "C.UTF-8"); got != "zh-Hans" {
		t.Fatalf("browser regional tag must not override user: got %q", got)
	}
	if got := ResolveLocale("en-US,en;q=0.9", "", "C.UTF-8"); got != "en" {
		t.Fatalf("browser list may apply without user: got %q", got)
	}
	if got := ResolveLocale("", "", "zh_CN.UTF-8"); got != "zh-Hans" {
		t.Fatalf("LANG should win: got %q", got)
	}
	if got := ResolveLocale("", "", "C.UTF-8"); got != DefaultLocale {
		t.Fatalf("C locale should fall back to en: got %q", got)
	}
	if got := ResolveLocale("", "", ""); got != DefaultLocale {
		t.Fatalf("empty should be en: got %q", got)
	}
}

func TestNormalizeDisplayNames(t *testing.T) {
	cases := map[string]string{
		"Chinese":             "zh-Hans",
		"简体中文":                "zh-Hans",
		"Traditional Chinese": "zh-Hant",
		"English":             "en",
		"zh-CN":               "zh-Hans",
		"pt-BR":               "pt-BR",
		"Portuguese BR":       "pt-BR",
	}
	for raw, want := range cases {
		if got := NormalizeLocale(raw); got != want {
			t.Errorf("NormalizeLocale(%q)=%q, want %q", raw, got, want)
		}
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	if got := ParseAcceptLanguage("zh-CN,zh;q=0.9,en;q=0.8"); got != "zh-Hans" {
		t.Fatalf("got %q", got)
	}
	if got := ParseAcceptLanguage("en-US"); got != "en" {
		t.Fatalf("got %q", got)
	}
	if got := ParseAcceptLanguage("fr;q=0"); got != "" {
		t.Fatalf("q=0 should be ignored, got %q", got)
	}
	if got := ParseAcceptLanguage("fr;q=0,en;q=0.5"); got != "en" {
		t.Fatalf("got %q", got)
	}
	if got := ParseAcceptLanguage("en;q=1.1,ja;q=0.8"); got != "ja" {
		t.Fatalf("invalid q should be ignored, got %q", got)
	}
}

func TestTranslate(t *testing.T) {
	if got := Translate("zh-Hans", "error.unauthorized"); got != "未授权！" {
		t.Fatalf("got %q", got)
	}
	if got := Translate("zh-Hans", "error.required", KV("field", "dataset_id")); got != "dataset_id 为必填项" {
		t.Fatalf("got %q", got)
	}
	if got := Translate("en", "Internal server error"); got != "Internal server error" {
		t.Fatalf("got %q", got)
	}
	if got := Translate("zh-Hans", "Internal server error"); got != "内部服务器错误" {
		t.Fatalf("got %q", got)
	}
	const datasetID = "710d27e67c0411f1b9aeed5d4991b280"
	want := "你不是该数据集 " + datasetID + " 的所有者"
	if got := Translate("zh-Hans", DatasetNotOwned, KV("id", datasetID)); got != want {
		t.Fatalf("enum lookup got %q", got)
	}
	if got := Translate("zh-Hans", "You don't own the dataset "+datasetID+"."); got != want {
		t.Fatalf("interpolated lookup got %q", got)
	}
}

func TestKeysMatchCatalog(t *testing.T) {
	en := loadCatalogs()[DefaultLocale]
	want := map[string]struct{}{}
	idRe := regexp.MustCompile(`^error(?:\.[a-z][a-z0-9_]*)+$`)
	for key := range en {
		if idRe.MatchString(key) {
			want[key] = struct{}{}
		}
	}
	got := map[string]struct{}{}
	for _, key := range Keys {
		got[key] = struct{}{}
	}
	if len(got) != len(want) {
		t.Fatalf("Keys=%d catalog ids=%d", len(got), len(want))
	}
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("missing Go const for %s", key)
		}
	}
}

func TestTUsesUserLanguageWhenHeaderMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	lang := "Chinese"
	c.Set("user", &entity.User{ID: "u1", Language: &lang})
	if got := T(c, "error.unauthorized"); got != "未授权！" {
		t.Fatalf("got %q", got)
	}
}

func TestTIgnoresBrowserAcceptLanguageWhenUserSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	lang := "zh-Hans"
	c.Set("user", &entity.User{ID: "u1", Language: &lang})
	if got := T(c, "error.unauthorized"); got != "未授权！" {
		t.Fatalf("got %q", got)
	}
}

func TestTUsesHeaderOverUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Accept-Language", "en")
	lang := "Chinese"
	c.Set("user", &entity.User{ID: "u1", Language: &lang})
	if got := T(c, "error.unauthorized"); got != "Unauthorized!" {
		t.Fatalf("got %q", got)
	}
}
