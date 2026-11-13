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

package common

import (
	"fmt"
	"strings"
	"time"
)

type ModelUsage struct {
	UserID         string    `json:"user_id"`
	UserEmail      string    `json:"user_email"`
	TenantID       string    `json:"tenant_id"`
	TenantEmail    string    `json:"tenant_email"`
	GroupID        string    `json:"group_id"`
	GroupName      string    `json:"group_name"`
	AppID          string    `json:"app_id"`
	AppName        string    `json:"app_name"`
	SessionID      string    `json:"session_id"`
	ProviderName   string    `json:"provider_name"`
	InstanceID     string    `json:"instance_id"`
	APIKey         string    `json:"api_key"`
	ModelName      string    `json:"model_name"`
	Type           string    `json:"type"`
	RequestID      string    `json:"request_id"`
	StartAt        time.Time `json:"start_at"`
	ResponseTimeMS int64     `json:"response_time_ms"`
	ErrorMessage   string    `json:"error_message"`

	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (m *ModelUsage) String() string {
	return fmt.Sprintf("%#v", m)
}

// GroupModelRefPrefix marks a model reference that points at a model group
// instead of a single model: gp-<group_id>. Stored in the tenant_*id model
// configuration columns (widened to 35 chars to fit the prefix), it makes a
// model slot resolve through the group's failover picker at call time.
const GroupModelRefPrefix = "gp-"

// GroupModelRefWidth is the storage width of a tenant_*id model-reference
// column: the gp- prefix plus a 32-char group id. len() of a string constant
// is not a compile-time constant in Go, so the value is spelled out.
const GroupModelRefWidth int64 = 35

// ParseGroupModelRef recognizes a group reference in model-ref position and
// returns the embedded group id. A gp- ref whose remainder is not a group id
// is not a group reference; it fails resolution downstream as an invalid
// model ref.
func ParseGroupModelRef(modelRef string) (string, bool) {
	if !strings.HasPrefix(modelRef, GroupModelRefPrefix) {
		return "", false
	}
	groupID := strings.TrimPrefix(modelRef, GroupModelRefPrefix)
	if !IsUUID(groupID) {
		return "", false
	}
	return groupID, true
}

func ParseCompositeModelName(compositeName string) (modelName, instanceName, providerName string, err error) {
	parts := strings.Split(compositeName, "@")
	switch len(parts) {
	case 3:
		// Format: model@instance@provider
		return parts[0], parts[1], parts[2], nil
	case 2:
		// Format: model@provider -> instance defaults to "default"
		return parts[0], "default", parts[1], nil
	case 1:
		return parts[0], "", "", fmt.Errorf("provider name missing in model name: %s", compositeName)
	}
	// len(parts) > 3: any '@' characters embedded in the leftmost modelName
	// component must be preserved in that component instead of being dropped
	// or assigned to the instance/provider fields.
	n := len(parts)
	return strings.Join(parts[:n-2], "@"), parts[n-2], parts[n-1], nil
}
