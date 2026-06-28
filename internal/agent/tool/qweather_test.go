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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQWeather_BuildURL(t *testing.T) {
	// Not t.Parallel(): this test reads the package-level
	// `qweatherEndpoint` var, which other tests (running in parallel)
	// temporarily replace with a httptest.Server URL.

	cases := []struct {
		name     string
		params   qweatherParams
		wantLoc  string
		wantKey  string
		wantLang string
		wantHost string
		wantPath string
	}{
		{
			name:     "Beijing, default lang",
			params:   qweatherParams{APIKey: "K-abc", Location: "101010100"},
			wantLoc:  "101010100",
			wantKey:  "K-abc",
			wantLang: "zh",
			wantHost: "devapi.qweather.com",
			wantPath: "/v7/weather/now",
		},
		{
			name:     "lat,lon with explicit English",
			params:   qweatherParams{APIKey: "K-xyz", Location: "39.904,116.405", Lang: "en"},
			wantLoc:  "39.904,116.405",
			wantKey:  "K-xyz",
			wantLang: "en",
			wantHost: "devapi.qweather.com",
			wantPath: "/v7/weather/now",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildQWeatherURL(tc.params)
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", u.Path, tc.wantPath)
			}
			q := u.Query()
			if q.Get("location") != tc.wantLoc {
				t.Errorf("location = %q, want %q", q.Get("location"), tc.wantLoc)
			}
			if q.Get("key") != tc.wantKey {
				t.Errorf("key = %q, want %q", q.Get("key"), tc.wantKey)
			}
			if q.Get("lang") != tc.wantLang {
				t.Errorf("lang = %q, want %q", q.Get("lang"), tc.wantLang)
			}
		})
	}
}

func TestQWeather_ParseResponse(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": "200",
			"updateTime": "2024-05-01T12:00+08:00",
			"now": {
				"temp": "23",
				"feelsLike": "22",
				"text": "多云",
				"windDir": "东南风",
				"humidity": "65"
			}
		}`))
	}))
	defer srv.Close()

	prev := qweatherEndpoint
	qweatherEndpoint = srv.URL + "/v7/weather/now"
	t.Cleanup(func() { qweatherEndpoint = prev })

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewQWeatherToolWith(helper)

	out, err := tool.InvokableRun(context.Background(),
		`{"api_key":"K-abc","location":"101010100"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotQuery.Get("key") != "K-abc" {
		t.Errorf("server saw key = %q, want K-abc", gotQuery.Get("key"))
	}
	if gotQuery.Get("location") != "101010100" {
		t.Errorf("server saw location = %q, want 101010100", gotQuery.Get("location"))
	}
	if gotQuery.Get("lang") != "zh" {
		t.Errorf("server saw lang = %q, want zh (default)", gotQuery.Get("lang"))
	}

	var env qweatherEnvelope
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if env.Error != "" {
		t.Errorf("Error = %q, want empty", env.Error)
	}
	if env.Temp != "23" {
		t.Errorf("Temp = %q, want 23", env.Temp)
	}
	if env.FeelsLike != "22" {
		t.Errorf("FeelsLike = %q, want 22", env.FeelsLike)
	}
	if env.Text != "多云" {
		t.Errorf("Text = %q, want 多云", env.Text)
	}
	if env.WindDir != "东南风" {
		t.Errorf("WindDir = %q, want 东南风", env.WindDir)
	}
	if env.Humidity != "65" {
		t.Errorf("Humidity = %q, want 65", env.Humidity)
	}
}

func TestQWeather_UpstreamBusinessError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"404","msg":"location not found"}`))
	}))
	defer srv.Close()

	prev := qweatherEndpoint
	qweatherEndpoint = srv.URL + "/v7/weather/now"
	t.Cleanup(func() { qweatherEndpoint = prev })

	helper := NewHTTPHelper().WithClient(&http.Client{
		Transport: rewriteHostTransport(srv.URL),
	})
	tool := NewQWeatherToolWith(helper)

	_, err := tool.InvokableRun(context.Background(),
		`{"api_key":"K-abc","location":"000000000"}`)
	if err == nil {
		t.Fatal("expected error for non-200 upstream code, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want to surface upstream code", err)
	}
}

func TestQWeather_RejectsMissingAPIKey(t *testing.T) {
	t.Parallel()

	tool := NewQWeatherTool()
	_, err := tool.InvokableRun(context.Background(),
		`{"location":"101010100"}`)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("err = %v, want to mention api_key", err)
	}
}

func TestQWeather_RejectsMissingLocation(t *testing.T) {
	t.Parallel()

	tool := NewQWeatherTool()
	_, err := tool.InvokableRun(context.Background(),
		`{"api_key":"K-abc"}`)
	if err == nil {
		t.Fatal("expected error for missing location")
	}
	if !strings.Contains(err.Error(), "location") {
		t.Errorf("err = %v, want to mention location", err)
	}
}

func TestQWeather_Info(t *testing.T) {
	t.Parallel()

	tool := NewQWeatherTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "qweather" {
		t.Errorf("Name = %q, want qweather", info.Name)
	}
	if !strings.Contains(info.Desc, "QWeather") && !strings.Contains(info.Desc, "和风") {
		t.Errorf("Desc = %q, want to mention QWeather", info.Desc)
	}
}
