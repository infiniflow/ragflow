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

import "errors"

// ErrChatModelUnavailable marks failures caused by an unavailable or
// unconfigured chat model, so handlers can return a clear user-facing message.
var ErrChatModelUnavailable = errors.New("chat model unavailable or not configured")

// ChatModelUnavailableMessage is the user-facing message returned to the
// client when the chat model cannot be used (not configured / invalid key /
// model not found).
const ChatModelUnavailableMessage = "The chat model is unavailable or not configured. Please check the chat model settings."
