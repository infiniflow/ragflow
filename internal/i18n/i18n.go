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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/entity"
	"ragflow/locales"

	"github.com/gin-gonic/gin"
)

const (
	DefaultLocale = "en"
	localeCtxKey  = "locale"
	headerCtxKey  = "locale_from_header"
)

var supported = map[string]struct{}{
	"en": {}, "zh-Hans": {}, "zh-Hant": {}, "id": {}, "ja": {}, "es": {},
	"vi": {}, "ru": {}, "pt-BR": {}, "de": {}, "fr": {}, "it": {},
	"bg": {}, "ar": {}, "tr": {}, "ko": {},
}

var displayNameMap = map[string]string{
	"english":             "en",
	"chinese":             "zh-Hans",
	"简体中文":               "zh-Hans",
	"traditional chinese": "zh-Hant",
	"繁體中文":               "zh-Hant",
	"russian":             "ru",
	"indonesian":          "id",
	"indonesia":           "id",
	"spanish":             "es",
	"vietnamese":          "vi",
	"japanese":            "ja",
	"korean":              "ko",
	"portuguese br":       "pt-BR",
	"pt-br":               "pt-BR",
	"german":              "de",
	"french":              "fr",
	"italian":             "it",
	"bulgarian":           "bg",
	"arabic":              "ar",
	"turkish":             "tr",
	"dutch":               "nl",
}

var alias = map[string]string{
	"zh":      "zh-Hans",
	"zh-cn":   "zh-Hans",
	"zh-sg":   "zh-Hans",
	"zh-hans": "zh-Hans",
	"zh-tw":   "zh-Hant",
	"zh-hk":   "zh-Hant",
	"zh-mo":   "zh-Hant",
	"zh-hant": "zh-Hant",
	"pt":      "pt-BR",
	"pt-br":   "pt-BR",
}

var (
	catalogOnce sync.Once
	catalogs    map[string]map[string]string
)

type Arg struct {
	Name  string
	Value string
}

func KV(name string, value any) Arg {
	return Arg{Name: name, Value: fmt.Sprint(value)}
}

func loadCatalogs() map[string]map[string]string {
	catalogOnce.Do(func() {
		catalogs = map[string]map[string]string{}
		entries, err := locales.FS.ReadDir("api")
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			raw, err := locales.FS.ReadFile("api/" + name)
			if err != nil {
				continue
			}
			var data map[string]string
			if err := json.Unmarshal(raw, &data); err != nil {
				continue
			}
			loc := strings.TrimSuffix(name, ".json")
			catalogs[loc] = data
		}
	})
	return catalogs
}

func NormalizeLocale(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	lowered := strings.ToLower(text)
	if lowered == "c" || lowered == "posix" {
		return ""
	}
	if mapped, ok := displayNameMap[lowered]; ok {
		if _, ok := supported[mapped]; ok {
			return mapped
		}
		return ""
	}
	tag := strings.SplitN(text, ",", 2)[0]
	tag = strings.SplitN(tag, ";", 2)[0]
	tag = strings.TrimSpace(tag)
	tag = strings.SplitN(tag, ".", 2)[0]
	tag = strings.TrimSpace(strings.ReplaceAll(tag, "_", "-"))
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, "-")
	primary := strings.ToLower(parts[0])
	rest := ""
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], "-")
	}
	candidate := primary
	if rest != "" {
		candidate = primary + "-" + rest
	}
	if mapped, ok := alias[strings.ToLower(candidate)]; ok {
		candidate = mapped
	} else if mapped, ok := alias[primary]; ok && rest == "" {
		candidate = mapped
	} else if rest == "" {
		candidate = primary
	}
	if _, ok := supported[candidate]; ok {
		return candidate
	}
	if _, ok := supported[primary]; ok {
		return primary
	}
	return ""
}

func ParseAcceptLanguage(header string) string {
	if header == "" {
		return ""
	}
	type ranked struct {
		q     float64
		index int
		tag   string
	}
	var ranges []ranked
	for idx, part := range strings.Split(header, ",") {
		piece := strings.TrimSpace(part)
		if piece == "" {
			continue
		}
		bits := strings.Split(piece, ";")
		tag := strings.TrimSpace(bits[0])
		q := 1.0
		for _, param := range bits[1:] {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") {
				var parsed float64
				if _, err := fmt.Sscanf(param[2:], "%f", &parsed); err == nil {
					q = parsed
				} else {
					q = 0
				}
			}
		}
		ranges = append(ranges, ranked{q: q, index: idx, tag: tag})
	}
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j].q > ranges[i].q || (ranges[j].q == ranges[i].q && ranges[j].index < ranges[i].index) {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}
	for _, item := range ranges {
		if loc := NormalizeLocale(item.tag); loc != "" {
			return loc
		}
	}
	return ""
}

func isExplicitAppLocale(header string) bool {
	header = strings.TrimSpace(header)
	if header == "" || strings.ContainsAny(header, ",;") {
		return false
	}
	if _, ok := supported[header]; ok {
		return true
	}
	lowered := strings.ToLower(strings.ReplaceAll(header, "_", "-"))
	if mapped, ok := alias[lowered]; ok {
		_, ok = supported[mapped]
		return ok
	}
	if mapped, ok := displayNameMap[strings.ToLower(header)]; ok {
		_, ok = supported[mapped]
		return ok
	}
	return false
}

func ResolveLocale(acceptLanguage, userLanguage, langEnv string) string {
	headerLoc := ParseAcceptLanguage(acceptLanguage)
	userLoc := NormalizeLocale(userLanguage)
	envLoc := NormalizeLocale(langEnv)
	if headerLoc != "" && isExplicitAppLocale(acceptLanguage) {
		return headerLoc
	}
	if userLoc != "" {
		return userLoc
	}
	if headerLoc != "" {
		return headerLoc
	}
	if envLoc != "" {
		return envLoc
	}
	return DefaultLocale
}

var placeholderRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

func interpolate(template string, args []Arg) string {
	for _, arg := range args {
		template = strings.ReplaceAll(template, "{{"+arg.Name+"}}", arg.Value)
	}
	return template
}

func compileTemplate(template string) *regexp.Regexp {
	var b strings.Builder
	b.WriteByte('^')
	last := 0
	for _, loc := range placeholderRe.FindAllStringSubmatchIndex(template, -1) {
		b.WriteString(regexp.QuoteMeta(template[last:loc[0]]))
		b.WriteString("(?P<" + template[loc[2]:loc[3]] + ">.*)")
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(template[last:]))
	b.WriteByte('$')
	return regexp.MustCompile(b.String())
}

func matchEnglishTemplate(en map[string]string, message string) (string, []Arg) {
	type item struct {
		key    string
		source string
		re     *regexp.Regexp
	}
	items := make([]item, 0)
	seen := map[string]struct{}{}
	for key, value := range en {
		source := ""
		if strings.Contains(key, "{{") {
			source = key
		} else if strings.Contains(value, "{{") {
			source = value
		} else {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		items = append(items, item{key: key, source: source, re: compileTemplate(source)})
	}
	sort.Slice(items, func(i, j int) bool { return len(items[i].source) > len(items[j].source) })
	for _, it := range items {
		m := it.re.FindStringSubmatch(message)
		if m == nil {
			continue
		}
		args := make([]Arg, 0, len(m)-1)
		for i, name := range it.re.SubexpNames() {
			if i == 0 || name == "" {
				continue
			}
			args = append(args, Arg{Name: name, Value: m[i]})
		}
		return it.key, args
	}
	return "", nil
}

func Translate(locale, key string, args ...Arg) string {
	if key == "" {
		return key
	}
	all := loadCatalogs()
	if locale == "" {
		locale = DefaultLocale
	}
	if text, ok := all[locale][key]; ok {
		return interpolate(text, args)
	}
	if locale != DefaultLocale {
		if text, ok := all[DefaultLocale][key]; ok {
			return interpolate(text, args)
		}
	}
	if matchedKey, extracted := matchEnglishTemplate(all[DefaultLocale], key); matchedKey != "" {
		args = append(extracted, args...)
		if text, ok := all[locale][matchedKey]; ok {
			return interpolate(text, args)
		}
		if locale != DefaultLocale {
			if text, ok := all[DefaultLocale][matchedKey]; ok {
				return interpolate(text, args)
			}
		}
	}
	return interpolate(key, args)
}

func LocaleFromContext(c *gin.Context) string {
	if c == nil {
		return ResolveLocale("", "", os.Getenv("LANG"))
	}
	header := ""
	if c.Request != nil {
		header = c.GetHeader("Accept-Language")
	}
	userLang := ""
	if raw, ok := c.Get("user"); ok {
		if user, ok := raw.(*entity.User); ok && user != nil && user.Language != nil {
			userLang = *user.Language
		}
	}
	langEnv := os.Getenv("LANG")
	if testing.Testing() && header == "" && userLang == "" {
		langEnv = ""
	}
	loc := ResolveLocale(header, userLang, langEnv)
	c.Set(localeCtxKey, loc)
	return loc
}

func T(c *gin.Context, key string, args ...Arg) string {
	return Translate(LocaleFromContext(c), key, args...)
}

func Error(key string, args ...Arg) error {
	return fmt.Errorf("%s", Translate(DefaultLocale, key, args...))
}

func SetLocale(c *gin.Context, locale string) {
	c.Set(localeCtxKey, locale)
}

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Accept-Language")
		if isExplicitAppLocale(header) {
			if loc := ParseAcceptLanguage(header); loc != "" {
				SetLocale(c, loc)
				c.Set(headerCtxKey, true)
			}
		}
		c.Next()
	}
}

func ApplyUserLocale() gin.HandlerFunc {
	return func(c *gin.Context) {
		if fromHeader, _ := c.Get(headerCtxKey); fromHeader == true {
			c.Next()
			return
		}
		header := c.GetHeader("Accept-Language")
		userLang := ""
		if raw, ok := c.Get("user"); ok {
			if user, ok := raw.(*entity.User); ok && user != nil && user.Language != nil {
				userLang = *user.Language
			}
		}
		SetLocale(c, ResolveLocale(header, userLang, os.Getenv("LANG")))
		c.Next()
	}
}
