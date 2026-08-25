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
	"context"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"time"
)

// OAuthLoginInit prepares a redirect to the channel's authorization URL.
// The returned state must also be set as a short-lived cookie on the caller
// so the callback can perform a CSRF check that ties the state token to the
// browser that initiated the flow.
type OAuthLoginInit struct {
	State        string
	AuthURL      string
	Channel      string
	CookieMaxAge time.Duration
}

// OAuthCallbackResult is returned to the handler after a successful callback
// so it can mint the user-facing auth response (cookie + redirect).
type OAuthCallbackResult struct {
	User      *entity.User
	IsNewUser bool
}

func (s *UserService) OAuthLoginInitiate(channel string, redis *redis.Client) (*OAuthLoginInit, common.ErrorCode, error) {
	return nil, common.CodeServerError, fmt.Errorf("oauth login initiate not implemented")
}

func (s *UserService) OAuthCallback(ctx context.Context, channel, code, callbackState, expectedState string, redis *redis.Client) (*OAuthCallbackResult, common.ErrorCode, error) {
	return nil, common.CodeDataError, fmt.Errorf("oauth callback not implemented")
}
