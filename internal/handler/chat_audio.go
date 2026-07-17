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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// ChatAudioSpeechRequest is the request body for POST /api/v1/chat/audio/speech.
type ChatAudioSpeechRequest struct {
	Text string `json:"text" binding:"required"`
}

// ttsSegmentSplitRegex mirrors Python's re.split(r"[，。/《》？；：！\n\r:;]+", text)
// used by chat_api.py's /chat/audio/speech endpoint.
var ttsSegmentSplitRegex = regexp.MustCompile("[，。/《》？；：！\\n\\r:;]+")

// ChatAudioSpeech converts text to speech using the tenant's default TTS model.
// It returns a streaming audio/mpeg response aligned with the Python endpoint.
func (h *ChatHandler) ChatAudioSpeech(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	var req ChatAudioSpeechRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	if h.llm == nil {
		common.ErrorWithCode(c, common.CodeServerError, "TTS service not available")
		return
	}

	driver, modelName, apiConfig, _, err := h.llm.GetTenantDefaultModelByType(user.ID, entity.ModelTypeTTS)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}

	// Match Python's streaming audio response headers.
	c.Header("Content-Type", "audio/mpeg")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	segments := ttsSegmentSplitRegex.Split(req.Text, -1)
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		resp, err := driver.AudioSpeech(&modelName, &seg, apiConfig, &modelModule.TTSConfig{Format: "mp3"})
		if err != nil {
			common.Warn("chat TTS synthesis failed",
				zap.String("segment", truncateString(seg, 64)),
				zap.Error(err))
			continue
		}
		if resp == nil || len(resp.Audio) == 0 {
			continue
		}
		if _, werr := c.Writer.Write(resp.Audio); werr != nil {
			return
		}
		c.Writer.Flush()
	}
}

// chatAudioAllowedExts is the set of audio extensions supported by the
// transcription endpoint, matching Python's ALLOWED_EXTS.
var chatAudioAllowedExts = map[string]struct{}{
	".wav":  {},
	".mp3":  {},
	".m4a":  {},
	".aac":  {},
	".flac": {},
	".ogg":  {},
	".webm": {},
	".opus": {},
	".wma":  {},
}

// ChatAudioTranscription transcribes an uploaded audio file using the tenant's
// default ASR model. It supports both a single JSON response and SSE streaming.
func (h *ChatHandler) ChatAudioTranscription(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	if h.llm == nil {
		common.ErrorWithCode(c, common.CodeServerError, "ASR service not available")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Missing 'file' in multipart form-data")
		return
	}

	suffix := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if _, ok := chatAudioAllowedExts[suffix]; suffix == "" || !ok {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
			fmt.Sprintf("Unsupported audio format: %s. Allowed: %s", suffix, allowedAudioExtsList()))
		return
	}

	// Save the uploaded file to a temporary location so the model driver can read it.
	tmpFile, err := os.CreateTemp("", "*"+suffix)
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, "Failed to create temp audio file: "+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		common.ErrorWithCode(c, common.CodeServerError, "Failed to save audio file: "+err.Error())
		return
	}

	driver, modelName, apiConfig, _, err := h.llm.GetTenantDefaultModelByType(user.ID, entity.ModelTypeSpeech2Text)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}

	streamMode := strings.ToLower(c.PostForm("stream")) == "true"
	if streamMode {
		disableWriteDeadlineForSSE(c)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Writer.WriteHeader(http.StatusOK)

		sender := func(content, _ *string) error {
			if content == nil {
				return nil
			}
			event := map[string]interface{}{"text": *content}
			if *content == "[DONE]" {
				event["event"] = "done"
			} else {
				event["event"] = "partial"
			}
			data, _ := json.Marshal(event)
			if _, err := c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data)); err != nil {
				return err
			}
			c.Writer.Flush()
			return nil
		}

		if err := driver.TranscribeAudioWithSender(&modelName, &tmpPath, apiConfig, &modelModule.ASRConfig{}, sender); err != nil {
			errEvent := map[string]interface{}{"event": "error", "text": err.Error()}
			data, _ := json.Marshal(errEvent)
			_, _ = c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
			c.Writer.Flush()
		}
		return
	}

	resp, err := driver.TranscribeAudio(&modelName, &tmpPath, apiConfig, &modelModule.ASRConfig{})
	if err != nil {
		common.ErrorWithCode(c, common.CodeServerError, err.Error())
		return
	}
	if resp == nil {
		common.ErrorWithCode(c, common.CodeServerError, "empty transcription response")
		return
	}

	common.SuccessWithData(c, map[string]string{"text": resp.Text}, "success")
}

func allowedAudioExtsList() string {
	exps := make([]string, 0, len(chatAudioAllowedExts))
	for ext := range chatAudioAllowedExts {
		exps = append(exps, ext)
	}
	sort.Strings(exps)
	return strings.Join(exps, ", ")
}

func truncateString(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
