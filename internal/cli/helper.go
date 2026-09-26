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

package cli

import (
	"fmt"
)

func (c *CLI) apiModeClient() (*HTTPClient, error) {
	if c.Config.CLIMode != APIMode {
		return nil, fmt.Errorf("this command is only allowed in USER mode")
	}
	httpClient := c.APIServerClientMap[c.Config.APIClientConfig.CurrentAPIServer]
	if httpClient == nil || (httpClient.LoginToken == nil && !httpClient.useAPIKey) {
		return nil, fmt.Errorf("no authorization")
	}
	return httpClient, nil
}

func (c *HTTPClient) AuthKind() string {
	if c.LoginToken != nil {
		return "web"
	}
	if c.useAPIKey {
		return "api"
	}
	return "web"
}
