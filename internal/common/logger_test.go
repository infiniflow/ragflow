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

package common

import (
	"bytes"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestColoredLineWriteSyncerColorsRequestAndResponse(t *testing.T) {
	var output bytes.Buffer
	syncer := coloredLineWriteSyncer{WriteSyncer: zapcore.AddSync(&output), color: true}
	input := []byte("timestamp\tinfo\t" + cyanLogMarker + `url=https://provider.example payload={"query":"hello"}` + greenLogMarker + ` response_code=200 response_body={"answer":"ok"}` + resetLogMarker + "\n")

	n, err := syncer.Write(input)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write() bytes = %d, want %d", n, len(input))
	}
	want := "timestamp\tinfo\t" + ansiBrightCyan + `url=https://provider.example payload={"query":"hello"}` + ansiGreen + ` response_code=200 response_body={"answer":"ok"}` + ansiReset + "\n"
	if got := output.String(); got != want {
		t.Errorf("colored line = %q, want %q", got, want)
	}
}

func TestColoredLineWriteSyncerKeepsFileLinePlain(t *testing.T) {
	var output bytes.Buffer
	syncer := coloredLineWriteSyncer{WriteSyncer: zapcore.AddSync(&output)}
	input := []byte("timestamp\tinfo\t" + cyanLogMarker + `url=https://provider.example payload={"query":"hello"}` + greenLogMarker + ` response_code=200 response_body={"answer":"ok"}` + resetLogMarker + "\n")

	if _, err := syncer.Write(input); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	want := "timestamp\tinfo\t" + `url=https://provider.example payload={"query":"hello"} response_code=200 response_body={"answer":"ok"}` + "\n"
	if got := output.String(); got != want {
		t.Errorf("file line = %q, want %q", got, want)
	}
}

func TestColoredLineWriteSyncerKeepsJSONUnescapedAfterEncoding(t *testing.T) {
	var output bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			MessageKey: "msg",
			LineEnding: zapcore.DefaultLineEnding,
		}),
		coloredLineWriteSyncer{WriteSyncer: zapcore.AddSync(&output), color: true},
		zapcore.InfoLevel,
	)
	zap.New(core).Info(cyanLogMarker + `url=https://provider.example payload={"query":"hello"}` + greenLogMarker + ` response_code=200 response_body={"answer":"ok"}` + resetLogMarker)

	want := ansiBrightCyan + `url=https://provider.example payload={"query":"hello"}` + ansiGreen + ` response_code=200 response_body={"answer":"ok"}` + ansiReset + "\n"
	if got := output.String(); got != want {
		t.Errorf("encoded line = %q, want %q", got, want)
	}
}
