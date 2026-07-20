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
	"strings"
	"testing"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// doneEmittingDriver streams the given deltas followed by the terminal
// "[DONE]" sentinel, mirroring real OpenAI-compatible drivers.
type doneEmittingDriver struct {
	*modelModule.DummyModel
	deltas []string
}

func (d *doneEmittingDriver) ChatStreamlyWithSender(modelName string, messages []modelModule.Message, apiConfig *modelModule.APIConfig, modelConfig *modelModule.ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	for _, delta := range d.deltas {
		if err := sender(&delta, nil); err != nil {
			return err
		}
	}
	done := streamDoneSentinel
	return sender(&done, nil)
}

func TestChatStreamWithContextStripsDoneSentinel(t *testing.T) {
	modelName := "fake-model"
	chatModel := &modelModule.ChatModel{
		ModelDriver: &doneEmittingDriver{
			DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{}),
			deltas:     []string{"Hello", " world"},
		},
		ModelName: &modelName,
		APIConfig: &modelModule.APIConfig{},
	}

	ch := chatStreamWithContext(context.Background(), chatModel, nil, &modelModule.ChatConfig{})
	var sb strings.Builder
	for delta := range ch {
		sb.WriteString(delta)
	}

	if got := sb.String(); got != "Hello world" {
		t.Fatalf("expected %q, got %q", "Hello world", got)
	}
}
