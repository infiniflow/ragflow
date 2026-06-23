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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const duckduckgoToolName = "duckduckgo"

const duckduckgoToolDescription = "Search DuckDuckGo's Instant Answer API. Returns the abstract text and a list of related topics."

// duckduckgoParams is the JSON shape the model sends into InvokableRun.
// max_results caps the number of RelatedTopics returned; the upstream
// endpoint itself is uncapped and uses heuristic ranking.
type duckduckgoParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// duckduckgoTopic is one element of the upstream `RelatedTopics` array.
// DuckDuckGo returns a recursive tree: a topic may itself have
// `Topics`, and may be a "name"/"value" pair (no URL) or a regular
// hit with `FirstURL`. We flatten one level of nesting.
type duckduckgoTopic struct {
	Text     string            `json:"text,omitempty"`
	FirstURL string            `json:"first_url,omitempty"`
	Topics   []duckduckgoTopic `json:"topics,omitempty"`
}

// duckduckgoResponse is the upstream Instant Answer envelope. We only
// model the fields we care about.
type duckduckgoResponse struct {
	AbstractText  string            `json:"abstract_text"`
	AbstractURL   string            `json:"abstract_url"`
	Abstract      string            `json:"abstract"`
	RelatedTopics []duckduckgoTopic `json:"related_topics"`
}

// duckduckgoTopicOut is the model-facing topic shape.
type duckduckgoTopicOut struct {
	Text     string `json:"text,omitempty"`
	FirstURL string `json:"first_url,omitempty"`
}

// duckduckgoEnvelope is the JSON shape the model sees.
type duckduckgoEnvelope struct {
	AbstractText  string               `json:"abstract_text,omitempty"`
	AbstractURL   string               `json:"abstract_url,omitempty"`
	RelatedTopics []duckduckgoTopicOut `json:"related_topics,omitempty"`
	Error         string               `json:"_ERROR,omitempty"`
}

// DuckDuckGoTool is the DuckDuckGo
// Instant Answer tool. It
// performs a GET against the public Instant Answer endpoint using the
// shared HTTPHelper and returns the abstract + a flat list of related
// topics.
type DuckDuckGoTool struct {
	helper *HTTPHelper
}

// NewDuckDuckGoTool returns a DuckDuckGoTool using the default HTTPHelper.
func NewDuckDuckGoTool() *DuckDuckGoTool {
	return NewDuckDuckGoToolWith(NewHTTPHelper())
}

// NewDuckDuckGoToolWith returns a DuckDuckGoTool that uses the provided
// HTTPHelper. Useful for tests.
func NewDuckDuckGoToolWith(h *HTTPHelper) *DuckDuckGoTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &DuckDuckGoTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (d *DuckDuckGoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: duckduckgoToolName,
		Desc: duckduckgoToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of related topics to return. Defaults to 5.",
				Required: false,
			},
		}),
	}, nil
}

// buildDuckDuckGoURL constructs the Instant Answer API URL.
func buildDuckDuckGoURL(query string) string {
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("no_html", "1")
	q.Set("skip_disambig", "1")
	return "https://api.duckduckgo.com/?" + q.Encode()
}

// flattenDuckDuckGoTopics flattens the upstream topic tree (which can
// have arbitrary nesting) into a list, dropping entries without a URL.
func flattenDuckDuckGoTopics(in []duckduckgoTopic) []duckduckgoTopicOut {
	var out []duckduckgoTopicOut
	for _, t := range in {
		if t.FirstURL != "" && t.Text != "" {
			out = append(out, duckduckgoTopicOut{Text: t.Text, FirstURL: t.FirstURL})
		}
		// recurse one level; upstream nests categories here
		if len(t.Topics) > 0 {
			out = append(out, flattenDuckDuckGoTopics(t.Topics)...)
		}
	}
	return out
}

// InvokableRun performs the DuckDuckGo Instant Answer query.
func (d *DuckDuckGoTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p duckduckgoParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: parse arguments: %w", err)),
			fmt.Errorf("duckduckgo: parse arguments: %w", err)
	}
	if p.Query == "" {
		return duckduckgoErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("duckduckgo: query is required")
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 5
	}

	endpoint := buildDuckDuckGoURL(p.Query)
	resp, err := d.helper.Do(ctx, http.MethodGet, endpoint, "", "", nil)
	if err != nil {
		return duckduckgoErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)
	}

	var raw duckduckgoResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: decode response: %w", err)),
			fmt.Errorf("duckduckgo: decode response: %w", err)
	}

	// Prefer AbstractText (rich) over Abstract (plain), as the Python
	// tool did historically.
	abstract := raw.AbstractText
	if abstract == "" {
		abstract = raw.Abstract
	}

	topics := flattenDuckDuckGoTopics(raw.RelatedTopics)
	if len(topics) > p.MaxResults {
		topics = topics[:p.MaxResults]
	}

	return duckduckgoJSON(duckduckgoEnvelope{
		AbstractText:  abstract,
		AbstractURL:   raw.AbstractURL,
		RelatedTopics: topics,
	}), nil
}

func duckduckgoJSON(env duckduckgoEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"duckduckgo: marshal result: %s"}`, err)
	}
	return string(b)
}

func duckduckgoErrJSON(err error) string {
	return duckduckgoJSON(duckduckgoEnvelope{Error: err.Error()})
}
