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

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Bound whole SSE events, including multiline data, using the model readers' limit.
const maxChatCompletionEventBytes = 1024 * 1024

type chatCompletionStreamData struct {
	chatCompletionData
	Final bool `json:"final"`
}

type chatCompletionStream struct {
	output          io.Writer
	legacy          bool
	data            chatCompletionData
	content         strings.Builder
	displayed       string
	answerPrinted   bool
	lineOpen        bool
	pendingRevision bool
}

// consumeChatCompletionStream displays native answer events as they arrive.
// Completion markers end reading even when the server keeps the body open.
func consumeChatCompletionStream(reader io.Reader, output io.Writer, legacy bool) (*chatCompletionData, bool, error) {
	stream := &chatCompletionStream{output: output, legacy: legacy}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxChatCompletionEventBytes+8)
	var firstData string
	var multiline strings.Builder
	var eventBytes int
	var hasData bool

	dispatch := func() (bool, error) {
		if !hasData {
			return false, nil
		}
		payload := firstData
		if multiline.Len() > 0 {
			payload = multiline.String()
		}
		firstData, eventBytes, hasData = "", 0, false
		multiline.Reset()
		return stream.consume(payload)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			done, err := dispatch()
			if err != nil || done {
				return stream.finish(err)
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		if field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		extra := len(value)
		if hasData {
			extra++ // SSE joins multiple data fields with a newline.
		}
		if extra > maxChatCompletionEventBytes-eventBytes {
			return stream.finish(fmt.Errorf("SSE event exceeds %d bytes", maxChatCompletionEventBytes))
		}
		eventBytes += extra
		if !hasData {
			firstData, hasData = value, true
		} else {
			if multiline.Len() == 0 {
				multiline.WriteString(firstData)
			}
			multiline.WriteByte('\n')
			multiline.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return stream.finish(fmt.Errorf("read SSE: %w", err))
	}
	// Accept a final marker without a trailing blank line, as the old reader did.
	if done, err := dispatch(); err != nil || done {
		return stream.finish(err)
	}
	return stream.finish(fmt.Errorf("stream ended before its completion marker: %w", io.ErrUnexpectedEOF))
}

// consume distinguishes answer objects from the native boolean completion frame.
func (s *chatCompletionStream) consume(payload string) (bool, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return false, nil
	}
	if payload == "[DONE]" {
		return true, nil
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return false, fmt.Errorf("invalid SSE event: %w", err)
	}
	if envelope.Code != 0 {
		return false, fmt.Errorf("server returned code %d: %s", envelope.Code, envelope.Message)
	}
	raw := bytes.TrimSpace(envelope.Data)
	if bytes.Equal(raw, []byte("true")) {
		return true, nil
	}
	if len(raw) == 0 || raw[0] != '{' {
		return false, fmt.Errorf("invalid SSE data: expected an answer object or true")
	}
	var chunk chatCompletionStreamData
	if err := json.Unmarshal(envelope.Data, &chunk); err != nil {
		return false, fmt.Errorf("invalid SSE answer: %w", err)
	}
	s.mergeMetadata(chunk.chatCompletionData)
	if chunk.Final {
		if chunk.Answer != "" {
			s.setAnswer(chunk.Answer)
		}
		return false, s.renderSnapshot(s.data.Answer, true)
	}
	if chunk.Answer == "" {
		return false, nil
	}
	if s.legacy {
		s.setAnswer(chunk.Answer)
		return false, s.renderSnapshot(chunk.Answer, false)
	}
	s.content.WriteString(chunk.Answer)
	s.data.Answer = s.content.String()
	return false, s.printText(chunk.Answer)
}

// mergeMetadata retains fields absent from intermediate events and final-only references.
func (s *chatCompletionStream) mergeMetadata(data chatCompletionData) {
	if data.Reference != nil {
		s.data.Reference = data.Reference
	}
	if data.ID != "" {
		s.data.ID = data.ID
	}
	if data.SessionID != "" {
		s.data.SessionID = data.SessionID
	}
	if data.ChatID != "" {
		s.data.ChatID = data.ChatID
	}
}

// setAnswer replaces a complete result instead of appending it to streamed deltas.
func (s *chatCompletionStream) setAnswer(answer string) {
	if s.legacy {
		s.data.Answer = answer
		return
	}
	if answer != s.data.Answer {
		s.content.Reset()
		s.content.WriteString(answer)
	}
	s.data.Answer = s.content.String()
}

// renderSnapshot prints only growth, or labels a genuinely revised final result.
func (s *chatCompletionStream) renderSnapshot(answer string, final bool) error {
	if answer == "" {
		return nil
	}
	if !s.answerPrinted || strings.HasPrefix(answer, s.displayed) {
		text := answer
		if s.answerPrinted {
			text = answer[len(s.displayed):]
		}
		if err := s.printText(text); err != nil {
			return err
		}
		s.pendingRevision = false
		return nil
	}
	if !final {
		s.pendingRevision = true
		return nil
	}
	if err := s.endLine(); err != nil {
		return err
	}
	if err := s.write("Final answer: "); err != nil {
		return err
	}
	s.lineOpen = true
	if err := s.write(answer); err != nil {
		return err
	}
	s.displayed, s.pendingRevision = s.data.Answer, false
	return nil
}

// printText writes immediately without changing or deduplicating answer characters.
func (s *chatCompletionStream) printText(text string) error {
	if text == "" {
		return nil
	}
	if !s.answerPrinted {
		if err := s.write("Answer: "); err != nil {
			return err
		}
		s.lineOpen = true
	}
	if err := s.write(text); err != nil {
		return err
	}
	s.answerPrinted, s.displayed = true, s.data.Answer
	return nil
}

// write treats incomplete output as an error instead of hiding a truncated answer.
func (s *chatCompletionStream) write(text string) error {
	n, err := io.WriteString(s.output, text)
	if err == nil && n != len(text) {
		err = io.ErrShortWrite
	}
	return err
}

// endLine separates answer output from references, timing, and command errors.
func (s *chatCompletionStream) endLine() error {
	if !s.lineOpen {
		return nil
	}
	s.lineOpen = false
	return s.write("\n")
}

// finish emits a deferred snapshot correction only after successful completion.
func (s *chatCompletionStream) finish(err error) (*chatCompletionData, bool, error) {
	if err == nil && s.pendingRevision {
		err = s.renderSnapshot(s.data.Answer, true)
	}
	err = errors.Join(err, s.endLine())
	data := s.data
	return &data, s.answerPrinted, err
}
