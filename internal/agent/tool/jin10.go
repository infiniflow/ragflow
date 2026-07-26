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
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const jin10ToolName = "jin10"

const jin10ToolDescription = "Subscribe to Jin10 (jin10.com) real-time financial flash news. " +
	"STUB: Jin10 has no public REST API; data is delivered over a private " +
	"WebSocket subscription. Not yet implemented in the Go Canvas. " +
	"Use the Python Canvas for Jin10 data."

const jin10UnsupportedMessage = "Jin10 requires WebSocket subscription — not yet implemented in Go Canvas. " +
	"Use Python Canvas."

// jin10Params is the JSON shape the model sends into InvokableRun. The
// Python implementation accepts a category (e.g. "all", "global", "cny")
// and an optional speed filter. The Go stub preserves the shape for
// future WebSocket client compatibility but rejects every invocation.
type jin10Params struct {
	Category string `json:"category"`
	Speed    string `json:"speed,omitempty"`
}

// jin10Envelope is the model-facing JSON shape. The stub always
// returns a populated Error.
type jin10Envelope struct {
	Items []any  `json:"items,omitempty"`
	Error string `json:"_ERROR,omitempty"`
}

// Jin10Tool is the Jin10
// flash-news tool.
//
// Jin10 (https://www.jin10.com) has no public API; subscribers receive
// data via a private WebSocket connection with token authentication.
// A native Go WebSocket client is deferred. For P3-B4 the tool is
// registered so DSLs that reference "jin10" keep parsing, but every
// invocation fails fast with a clear "use Python Canvas" message.
//
// Jin10Tool does not own an HTTPHelper — it never makes network calls.
type Jin10Tool struct{}

// NewJin10Tool returns a Jin10Tool. No HTTPHelper is allocated; the
// stub never issues network requests.
func NewJin10Tool() *Jin10Tool { return &Jin10Tool{} }

// Info returns the tool's metadata for the chat model.
func (j *Jin10Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: jin10ToolName,
		Desc: jin10ToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"category": {
				Type:     schema.String,
				Desc:     "News category filter (e.g. \"all\", \"global\", \"cny\"). Defaults to \"all\".",
				Required: false,
			},
			"speed": {
				Type:     schema.String,
				Desc:     "Optional speed/dedup filter. Defaults to empty (no filter).",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun validates the input shape (category is optional with
// default "all") and returns a clear "use Python Canvas" error. The
// model receives a JSON envelope with the message in the `_ERROR` field.
func (j *Jin10Tool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p jin10Params
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
			return jin10ErrJSON(fmt.Errorf("jin10: parse arguments: %w", err)),
				errors.New(jin10UnsupportedMessage)
		}
	}
	if p.Category == "" {
		p.Category = "all"
	}
	return jin10ErrJSON(errors.New(jin10UnsupportedMessage)),
		errors.New(jin10UnsupportedMessage)
}

func jin10JSON(env jin10Envelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"jin10: marshal result: %s"}`, err)
	}
	return string(b)
}

func jin10ErrJSON(err error) string {
	return jin10JSON(jin10Envelope{Error: err.Error()})
}
