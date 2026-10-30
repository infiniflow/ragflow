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
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
)

const qweatherToolName = "qweather"

const qweatherToolDescription = "Fetch current weather conditions from 和风天气 (QWeather, devapi.qweather.com). " +
	"Returns {temp, feelsLike, text, windDir, humidity}."

// qweatherEndpoint is the QWeather v7 "now" endpoint. Exposed as a
// package var so tests can substitute a httptest.Server URL.
var qweatherEndpoint = "https://devapi.qweather.com/v7/weather/now"

// qweatherParams is the JSON shape the model sends into InvokableRun.
//
//   - Location (required): the location code, e.g. "101010100" (Beijing)
//     or a lat,lon string "39.904,116.405".
//   - Lang (optional): language code for `text` and `windDir`. Defaults
//     to "zh" per the inbox spec.
type qweatherParams struct {
	Location string `json:"location"`
	Lang     string `json:"lang,omitempty"`
}

// qweatherNow is the upstream `now` object.
type qweatherNow struct {
	Temp      string `json:"temp"`      // "23"
	FeelsLike string `json:"feelsLike"` // "22"
	Text      string `json:"text"`      // "多云"
	WindDir   string `json:"windDir"`   // "东南风"
	Humidity  string `json:"humidity"`  // "65"
}

// qweatherResponse is the upstream QWeather envelope. We model only
// the fields the inbox spec calls out; additional fields from QWeather
// (obsTime, precip, pressure, ...) are ignored.
type qweatherResponse struct {
	Code string      `json:"code"` // "200" = OK
	Now  qweatherNow `json:"now"`
}

// qweatherEnvelope is the model-facing JSON shape.
type qweatherEnvelope struct {
	Temp      string `json:"temp,omitempty"`
	FeelsLike string `json:"feels_like,omitempty"`
	Text      string `json:"text,omitempty"`
	WindDir   string `json:"wind_dir,omitempty"`
	Humidity  string `json:"humidity,omitempty"`
	Error     string `json:"_ERROR,omitempty"`
}

// (QWeather) current-conditions tool (
// batch 4). It performs a GET against devapi.qweather.com/v7/weather/now
// and returns the parsed now.{temp, feelsLike, text, windDir, humidity}.
//
// QWeatherTool uses the shared HTTPHelper for retry/timeout/OTel
// propagation.
type QWeatherTool struct {
	helper *HTTPHelper
	apiKey string
}

// NewQWeatherTool returns a QWeatherTool using the default HTTPHelper
// and the QWEATHER_API_KEY env var for credential resolution.
func NewQWeatherTool() *QWeatherTool {
	return NewQWeatherToolWith(nil, "")
}

func NewQWeatherToolWith(h *HTTPHelper, apiKey string) *QWeatherTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &QWeatherTool{helper: h, apiKey: strings.TrimSpace(apiKey)}
}

// resolveAPIKey returns the node-level credential, or QWEATHER_API_KEY
// when the node does not carry one.
func (q *QWeatherTool) resolveAPIKey() string {
	if q.apiKey != "" {
		return q.apiKey
	}
	return strings.TrimSpace(common.GetEnv(common.EnvQWeatherAPIKey))
}

// Info returns the tool's metadata for the chat model.
func (q *QWeatherTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: qweatherToolName,
		Desc: qweatherToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"location": {
				Type:     schema.String,
				Desc:     "Location code (e.g. 101010100 for Beijing) or \"lat,lon\" (e.g. \"39.904,116.405\").",
				Required: true,
			},
			"lang": {
				Type:     schema.String,
				Desc:     "Language for `text` and `windDir`. Defaults to \"zh\".",
				Required: false,
			},
		}),
	}, nil
}

// buildQWeatherURL composes the devapi.qweather.com URL. Centralized
// for testability.
func buildQWeatherURL(apiKey string, p qweatherParams) string {
	q := url.Values{}
	q.Set("location", p.Location)
	q.Set("key", apiKey)
	if p.Lang == "" {
		p.Lang = "zh"
	}
	q.Set("lang", p.Lang)
	return qweatherEndpoint + "?" + q.Encode()
}

// InvokableRun performs the QWeather current-conditions GET.
func (q *QWeatherTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p qweatherParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return qweatherErrJSON(fmt.Errorf("qweather: parse arguments: %w", err)),
			fmt.Errorf("qweather: parse arguments: %w", err)
	}
	apiKey := q.resolveAPIKey()
	if apiKey == "" {
		return qweatherErrJSON(fmt.Errorf("qweather: api_key is not configured")),
			fmt.Errorf("qweather: api_key is not configured (set the node-level api_key or QWEATHER_API_KEY)")
	}
	if p.Location == "" {
		return qweatherErrJSON(fmt.Errorf("qweather: location is required")),
			fmt.Errorf("qweather: location is required")
	}

	endpoint := buildQWeatherURL(apiKey, p)
	headers := map[string]string{
		"Accept": "application/json",
	}

	resp, err := q.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return qweatherErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return qweatherErrJSON(fmt.Errorf("qweather: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("qweather: upstream returned %d", resp.StatusCode)
	}

	var raw qweatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return qweatherErrJSON(fmt.Errorf("qweather: decode response: %w", err)),
			fmt.Errorf("qweather: decode response: %w", err)
	}
	// QWeather uses "200" string code for OK; anything else is a
	// business-level error. Code "404" = city not found, "401" = authentication failed,
	// "429" = exceeded access count, "402" = exceeded access rate.
	if raw.Code != "200" {
		return qweatherErrJSON(fmt.Errorf("qweather: upstream returned code %q", raw.Code)),
			fmt.Errorf("qweather: upstream returned code %q", raw.Code)
	}
	return qweatherJSON(qweatherEnvelope{
		Temp:      raw.Now.Temp,
		FeelsLike: raw.Now.FeelsLike,
		Text:      raw.Now.Text,
		WindDir:   raw.Now.WindDir,
		Humidity:  raw.Now.Humidity,
	}), nil
}

func qweatherJSON(env qweatherEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"qweather: marshal result: %s"}`, err)
	}
	return string(b)
}

func qweatherErrJSON(err error) string {
	return qweatherJSON(qweatherEnvelope{Error: err.Error()})
}
