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
	"context"
	"errors"

	"ragflow/internal/common"
)

// providerErrorDriver classifies chat failures as *common.LLMError at the
// single boundary every chat call crosses — the driver itself — instead of at
// each component's invoker. Provider and model names are known exactly here,
// and the underlying error text is preserved verbatim, so log lines and
// status extraction behave as before. Only the two chat methods are
// classified; embedding, rerank, ASR/TTS and admin calls pass through.
type providerErrorDriver struct {
	ModelDriver
}

// WrapProviderChatErrors returns driver with chat errors typed as
// *common.LLMError of kind provider. It is nil-safe and idempotent.
func WrapProviderChatErrors(driver ModelDriver) ModelDriver {
	if driver == nil {
		return nil
	}
	if _, ok := driver.(*providerErrorDriver); ok {
		return driver
	}
	return &providerErrorDriver{ModelDriver: driver}
}

// NewInstance keeps classification applied to derived copies, e.g. drivers
// re-built with a custom base URL.
func (d *providerErrorDriver) NewInstance(baseURL map[string]string) ModelDriver {
	return WrapProviderChatErrors(d.ModelDriver.NewInstance(baseURL))
}

// Underlying returns driver with any chat-error wrapper removed, for call
// sites that type-assert on the concrete driver implementation.
func Underlying(driver ModelDriver) ModelDriver {
	for {
		w, ok := driver.(*providerErrorDriver)
		if !ok {
			return driver
		}
		driver = w.ModelDriver
	}
}

func (d *providerErrorDriver) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	resp, err := d.ModelDriver.ChatWithMessages(ctx, modelName, messages, apiConfig, chatModelConfig, modelUsage)
	return resp, classifyChatError(d.Name(), modelName, err)
}

func (d *providerErrorDriver) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return classifyChatError(d.Name(), modelName, d.ModelDriver.ChatStreamlyWithSender(ctx, modelName, messages, apiConfig, modelConfig, modelUsage, sender))
}

// classifyChatError types a provider rejection while letting cancellation and
// deadline errors through untyped: they are the caller's own control flow,
// not a model-service failure to attribute to the tenant's provider.
func classifyChatError(provider, model string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return common.NewLLMProviderError(provider, model, err)
}
