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
	"sort"

	"ragflow/internal/common"
	"ragflow/internal/server"
)

type LoginChannel struct {
	Channel     string `json:"channel"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
}

// GetLoginChannels gets all supported authentication channels configured under
// the top-level `oauth:` block. The result is sorted by channel name so the
// login page renders a stable order across requests and restarts.
func (s *UserService) GetLoginChannels() ([]*LoginChannel, common.ErrorCode, error) {
	providers := server.GetConfig().GetOAuthConfig().Providers
	channels := make([]*LoginChannel, 0, len(providers))
	for _, p := range providers {
		channels = append(channels, &LoginChannel{
			Channel:     p.Channel,
			DisplayName: p.DisplayName,
			Icon:        p.Icon,
		})
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Channel < channels[j].Channel
	})

	return channels, common.CodeSuccess, nil
}
