//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/ingestion/testutil"

	"github.com/gin-gonic/gin"
)

// TestPullMessageFromQueueInitializesSharedConsumer preserves the admin
// endpoint's ability to pull messages before the ingestor starts. The endpoint
// must initialize the existing durable consumer rather than creating a
// dispatcher-specific consumer.
func TestPullMessageFromQueueInitializesSharedConsumer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	queue := testutil.SetupNatsEngine(t)
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	payload, err := json.Marshal(common.TaskMessage{
		TaskID:   "admin-pull-before-ingestor",
		TaskType: common.TaskTypeIngestionTask,
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	if err := queue.PublishTask(common.TaskSubject, payload); err != nil {
		t.Fatalf("publish task: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/",
		bytes.NewBufferString(`{"message_count":1,"ack_policy":"ACK"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	(&Handler{}).PullMessageFromQueue(ctx)

	var response struct {
		Code int `json:"code"`
		Data []struct {
			ID  string `json:"id"`
			Ack string `json:"ack"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != int(common.CodeSuccess) {
		t.Fatalf("response code = %d, want %d; body = %s", response.Code, common.CodeSuccess, recorder.Body.String())
	}
	if len(response.Data) != 1 || response.Data[0].ID != "admin-pull-before-ingestor" || response.Data[0].Ack != "true" {
		t.Fatalf("pull response = %+v, want acked admin-pull-before-ingestor", response.Data)
	}
}

func TestPullMessageFromQueueRejectsOutOfRangeMessageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name         string
		messageCount int
	}{
		{name: "negative", messageCount: -1},
		{name: "zero", messageCount: 0},
		{name: "above limit", messageCount: 101},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(
				http.MethodPost,
				"/",
				bytes.NewBufferString(fmt.Sprintf(`{"message_count":%d,"ack_policy":"ACK"}`, testCase.messageCount)),
			)
			ctx.Request.Header.Set("Content-Type", "application/json")

			(&Handler{}).PullMessageFromQueue(ctx)

			var response struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != int(common.CodeBadRequest) {
				t.Fatalf("response code = %d, want %d; body = %s", response.Code, common.CodeBadRequest, recorder.Body.String())
			}
		})
	}
}
