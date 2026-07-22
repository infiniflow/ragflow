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

package service

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateRefreshFreq(t *testing.T) {
	negative := int64(-1)
	zero := int64(0)
	positive := int64(5)

	tests := []struct {
		name    string
		freq    *int64
		present bool
		err     error
	}{
		{name: "unset", freq: nil},
		{name: "zero", freq: &zero},
		{name: "positive", freq: &positive},
		{name: "negative", freq: &negative, err: ErrInvalidRefreshFreq},
		{name: "null", freq: nil, present: true, err: ErrInvalidRefreshFreq},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRefreshFreq(tt.freq, tt.present)
			if !errors.Is(err, tt.err) {
				t.Fatalf("validateRefreshFreq() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestCreateConnectorRejectsNullRefreshFreq(t *testing.T) {
	var req CreateConnectorRequest
	if err := json.Unmarshal([]byte(`{"refresh_freq":null}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	connector, err := NewConnectorService().CreateConnector("tenant-1", &req)

	if connector != nil {
		t.Fatalf("CreateConnector() connector = %#v, want nil", connector)
	}
	if !errors.Is(err, ErrInvalidRefreshFreq) {
		t.Fatalf("CreateConnector() error = %v, want %v", err, ErrInvalidRefreshFreq)
	}
}

func TestUpdateConnectorRequestPreservesNullRefreshFreq(t *testing.T) {
	var req UpdateConnectorRequest
	if err := json.Unmarshal([]byte(`{"refresh_freq":null}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if !errors.Is(validateRefreshFreq(req.RefreshFreq, req.refreshFreqSet), ErrInvalidRefreshFreq) {
		t.Fatal("explicit null refresh_freq was treated as an omitted field")
	}
}

func TestCreateConnectorRejectsNegativeRefreshFreq(t *testing.T) {
	negative := int64(-1)

	connector, err := NewConnectorService().CreateConnector("tenant-1", &CreateConnectorRequest{RefreshFreq: &negative})

	if connector != nil {
		t.Fatalf("CreateConnector() connector = %#v, want nil", connector)
	}
	if !errors.Is(err, ErrInvalidRefreshFreq) {
		t.Fatalf("CreateConnector() error = %v, want %v", err, ErrInvalidRefreshFreq)
	}
}
