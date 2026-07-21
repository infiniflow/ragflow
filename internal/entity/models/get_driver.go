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

package models

import (
	"fmt"
	"strings"
)

// GetPreconfiguredDriver returns the provider's pre-built ModelDriver from
// ProviderManager, with an optional base URL override applied.
//
// The caller MUST NOT mutate the returned driver — use NewInstance to
// create a derived copy with custom base URLs.
//
// Returns:
//   - driver, nil — found and (optionally) overridden
//   - nil, error — provider manager uninitialized, provider unknown,
//     or provider does not support custom base URLs
func GetPreconfiguredDriver(driver string, baseURLOverride string) (ModelDriver, error) {
	if strings.EqualFold(driver, "dummy") {
		baseURL := map[string]string(nil)
		if strings.TrimSpace(baseURLOverride) != "" {
			baseURL = map[string]string{"default": strings.TrimRight(baseURLOverride, "/")}
		}
		return NewDummyModel(baseURL, URLSuffix{Chat: "chat/completions"}), nil
	}

	pm := GetProviderManager()
	if pm == nil {
		return nil, fmt.Errorf("provider manager not initialized")
	}
	provider := pm.FindProvider(driver)
	if provider == nil || provider.ModelDriver == nil {
		return nil, fmt.Errorf("provider %q is not configured", driver)
	}
	md := provider.ModelDriver
	if strings.TrimSpace(baseURLOverride) != "" {
		md = md.NewInstance(map[string]string{
			"default": strings.TrimRight(baseURLOverride, "/"),
		})
		if md == nil {
			return nil, fmt.Errorf("provider %q does not support a custom base_url", driver)
		}
	}
	return md, nil
}
