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

package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestClearWriteDeadlineAllowsLongLivedStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		clearResponseWriteDeadline(c)
		c.Header("Content-Type", "text/event-stream")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()

		if _, err := c.Writer.Write([]byte("data: first\n\n")); err != nil {
			t.Errorf("write first chunk: %v", err)
			return
		}
		c.Writer.Flush()

		time.Sleep(120 * time.Millisecond)

		if _, err := c.Writer.Write([]byte("data: second\n\n")); err != nil {
			t.Errorf("write second chunk: %v", err)
			return
		}
		c.Writer.Flush()
	})

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 30 * time.Millisecond
	server.Start()
	defer server.Close()

	client := server.Client()
	client.Timeout = time.Second
	resp, err := client.Get(server.URL + "/stream")
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}

	body := string(bodyBytes)
	for _, want := range []string{"data: first", "data: second"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body missing %q: %q", want, body)
		}
	}
}

// TestClearWriteDeadlineAllowsLateSingleWrite pins the non-stream counterpart:
// a handler that computes past http.Server.WriteTimeout before writing its one
// JSON response must still deliver the body once the deadline is cleared — the
// exact failure mode of long agentic chat runs ("write tcp ... i/o timeout").
func TestClearWriteDeadlineAllowsLateSingleWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/complete", func(c *gin.Context) {
		clearResponseWriteDeadline(c)

		time.Sleep(120 * time.Millisecond)

		c.JSON(http.StatusOK, gin.H{"answer": "done"})
	})

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 30 * time.Millisecond
	server.Start()
	defer server.Close()

	client := server.Client()
	client.Timeout = time.Second
	resp, err := client.Post(server.URL+"/complete", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("post complete: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(bodyBytes); !strings.Contains(got, `"answer":"done"`) {
		t.Fatalf("response body missing answer after exceeded WriteTimeout: %q", got)
	}
}
