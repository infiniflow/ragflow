// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
)

var anySearchDiagnosticID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// anySearchWebSearchDiagnostic holds only one request's diagnostic state.
type anySearchWebSearchDiagnostic struct {
	key, query, requestID string
}

func (d *anySearchWebSearchDiagnostic) observe(status int, body []byte) {
	var envelope struct {
		Code      json.RawMessage `json:"code"`
		RequestID string          `json:"request_id"`
	}
	fields := []zap.Field{zap.Int("http_status", status)}
	if json.Unmarshal(body, &envelope) == nil {
		var code *int64
		if json.Unmarshal(envelope.Code, &code) == nil && code != nil {
			fields = append(fields, zap.Int64("business_code", *code))
		}
		id := envelope.RequestID
		containsSecret := (d.key != "" && strings.Contains(id, d.key)) || (d.query != "" && strings.Contains(id, d.query))
		if containsSecret {
			fields = append(fields, zap.Bool("request_id_redacted", true))
		} else if anySearchDiagnosticID.MatchString(id) {
			d.requestID = id
			fields = append(fields, zap.String("request_id", id))
		}
	}
	if d.requestID == "" {
		fields = append(fields, zap.Bool("request_id_unavailable", true))
	}
	common.Info("AnySearch response", fields...)
}

func (d *anySearchWebSearchDiagnostic) withRequestID(err error) error {
	if d.requestID == "" {
		return err
	}
	return fmt.Errorf("%w (request_id %s)", err, d.requestID)
}
