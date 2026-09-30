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
	"errors"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// streamDoneSentinel is the OpenAI-style end-of-stream marker model drivers
// emit as their final sender call. It is a transport signal, not answer text.
const streamDoneSentinel = "[DONE]"

// errStreamDone aborts the driver loop once the terminal sentinel arrives.
var errStreamDone = errors.New("chat stream done")

func chatStreamWithContext(ctx context.Context, chatModel *modelModule.ChatModel, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, <-chan error) {
	ch := make(chan string, 256)
	errCh := make(chan error, 1)
	go func() {
		defer close(ch)
		defer close(errCh)
		if err := chatModel.ChatStreamlyWithSender(ctx, messages, config, nil,
			func(delta *string, _ *string) error {
				if delta == nil {
					return nil
				}
				if *delta == streamDoneSentinel {
					return errStreamDone
				}
				select {
				case ch <- *delta:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}); err != nil {
			if errors.Is(err, errStreamDone) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			common.Warn("ChatStreamlyWithSender returned error", zap.Error(err))
			errCh <- err
		}
	}()
	return ch, errCh
}

// TenantStreamAdapter adapts a factory-created chat model to AskService's
// channel-based streaming interface.
type TenantStreamAdapter struct {
	Factory  *ModelFactory
	TenantID string
	ModelID  string
}

func (a *TenantStreamAdapter) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, <-chan error, error) {
	if a.Factory == nil {
		return nil, nil, fmt.Errorf("model factory not configured")
	}
	chatModel, err := a.Factory.NewChatModel(ctx, ModelAccess{TenantID: a.TenantID}, a.ModelID)
	if err != nil {
		return nil, nil, err
	}
	ch, errCh := chatStreamWithContext(ctx, chatModel, messages, config)
	return ch, errCh, nil
}
