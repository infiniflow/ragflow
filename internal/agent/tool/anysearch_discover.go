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
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

const anySearchDiscoverToolDescription = `AnySearch groups its vertical search tags by domain, and the catalog changes over time. Use this tool before a vertical anysearch_search:
   - Call it with no arguments to list the current domains.
   - Call it with domains (1 to 5 ids from that list) to get their sub-domains: each tag, what it is for, and the params it takes, with the required ones marked.
   Pass the tag and params to anysearch_search exactly as returned. A required param that does not apply is still passed, with an empty string.`

// anySearchDiscoverMaxBytes bounds the catalog handed to the model.
const anySearchDiscoverMaxBytes = 128 << 10

// anySearchMaxDomains is the most domains one catalog request may name.
const anySearchMaxDomains = 5

var anySearchDomainID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

type anySearchDiscoverArgs struct {
	Domains []string `json:"domains"`
}

type anySearchDiscoverEnvelope struct {
	Domains   []any  `json:"domains"`
	RequestID string `json:"request_id,omitempty"`
	Error     string `json:"_ERROR,omitempty"`
}

// AnySearchDiscoverTool lists AnySearch's current domains, or the sub-domains
// and params of the named domains, so the model can choose a valid vertical
// tag. It is an Agent tool only: a catalog listing has no Canvas component.
type AnySearchDiscoverTool struct {
	client *anySearchClient
}

// NewAnySearchDiscoverTool returns an anonymous AnySearchDiscoverTool with the
// default HTTPHelper.
func NewAnySearchDiscoverTool() *AnySearchDiscoverTool {
	return NewAnySearchDiscoverToolWith(nil, "")
}

// NewAnySearchDiscoverToolWith returns an AnySearchDiscoverTool that uses the
// provided HTTPHelper and API key. Useful for tests that want to inject a
// custom transport.
func NewAnySearchDiscoverToolWith(h *HTTPHelper, apiKey string) *AnySearchDiscoverTool {
	return &AnySearchDiscoverTool{client: newAnySearchClient(h, apiKey)}
}

// Info returns the tool's metadata for the chat model.
func (d *AnySearchDiscoverTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "domains": {
      "type": "array",
      "maxItems": ` + fmt.Sprint(anySearchMaxDomains) + `,
      "items": {"type": "string"},
      "description": "Domain ids from the domain list, such as code or finance. Leave it out to list the domains."
    }
  }
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("anysearch: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        anySearchDiscoverToolName,
		Desc:        anySearchDiscoverToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun reads the catalog. Recoverable errors are returned as an _ERROR
// envelope with a nil Go error, like AnySearchTool.
func (d *AnySearchDiscoverTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args anySearchDiscoverArgs
	if strings.TrimSpace(argsJSON) == "" {
		argsJSON = "{}"
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return anySearchDiscoverErrJSON(fmt.Errorf("anysearch: parse arguments: %w", err)), nil
	}
	if len(args.Domains) > anySearchMaxDomains {
		return anySearchDiscoverErrJSON(fmt.Errorf("anysearch: domains must contain at most %d items", anySearchMaxDomains)), nil
	}
	path := "/domains"
	if len(args.Domains) > 0 {
		query := neturl.Values{}
		for _, domain := range args.Domains {
			id := strings.ToLower(strings.TrimSpace(domain))
			if !anySearchDomainID.MatchString(id) {
				return anySearchDiscoverErrJSON(fmt.Errorf("anysearch: domain %q is not a domain id, call this tool with no arguments to list them", domain)), nil
			}
			query.Add("domain", id)
		}
		path = "/sub-domains?" + query.Encode()
	}

	reply, err := d.client.call(ctx, http.MethodGet, path, nil)
	if err != nil {
		var apiErr *anySearchError
		envelope := anySearchDiscoverEnvelope{Domains: []any{}, Error: err.Error()}
		if errors.As(err, &apiErr) {
			envelope.RequestID = apiErr.requestID
		}
		return anySearchJSON(envelope), nil
	}
	domains, ok := reply.data["domains"].([]any)
	if !ok {
		return anySearchDiscoverErrJSON(anySearchFailure("invalid domain catalog", "", reply.requestID)), nil
	}
	out := anySearchJSON(anySearchDiscoverEnvelope{Domains: domains, RequestID: reply.requestID})
	if len(out) > anySearchDiscoverMaxBytes {
		return anySearchDiscoverErrJSON(fmt.Errorf("anysearch: the catalog is larger than %d bytes, ask for fewer domains", anySearchDiscoverMaxBytes)), nil
	}
	return out, nil
}

func anySearchDiscoverErrJSON(err error) string {
	return anySearchJSON(anySearchDiscoverEnvelope{Domains: []any{}, Error: err.Error()})
}
