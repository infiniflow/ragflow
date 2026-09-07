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

package handler

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// testWAV builds a minimal 24kHz/16-bit/mono PCM WAV.
func testWAV(pcm []byte) []byte {
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:], 1)     // PCM
	binary.LittleEndian.PutUint16(fmtChunk[2:], 1)     // mono
	binary.LittleEndian.PutUint32(fmtChunk[4:], 24000) // sample rate
	binary.LittleEndian.PutUint32(fmtChunk[8:], 48000) // byte rate
	binary.LittleEndian.PutUint16(fmtChunk[12:], 2)    // block align
	binary.LittleEndian.PutUint16(fmtChunk[14:], 16)   // bits per sample
	return buildWAV(&wavFormat{fmtChunk: fmtChunk}, pcm)
}

func TestSplitWAVRoundTrip(t *testing.T) {
	pcm := []byte{1, 2, 3, 4, 5}
	format, gotPCM, err := splitWAV(testWAV(pcm))
	if err != nil {
		t.Fatalf("splitWAV: %v", err)
	}
	if !bytes.Equal(gotPCM, pcm) {
		t.Errorf("pcm = %v, want %v", gotPCM, pcm)
	}
	rebuilt := buildWAV(format, gotPCM)
	if len(rebuilt)%2 != 0 {
		t.Errorf("rebuilt WAV length = %d, want even (RIFF chunks are 2-byte aligned)", len(rebuilt))
	}
	if got := binary.LittleEndian.Uint32(rebuilt[4:8]); got != uint32(len(rebuilt)-8) {
		t.Errorf("RIFF size = %d, want %d", got, len(rebuilt)-8)
	}
	format2, pcm2, err := splitWAV(rebuilt)
	if err != nil {
		t.Fatalf("splitWAV(rebuilt): %v", err)
	}
	if !bytes.Equal(format.fmtChunk, format2.fmtChunk) {
		t.Errorf("fmt chunk changed after round trip")
	}
	if !bytes.Equal(pcm2, pcm) {
		t.Errorf("pcm changed after round trip: %v", pcm2)
	}
}

func TestBuildWAVConcatenatedPCM(t *testing.T) {
	seg1 := []byte{1, 2}
	seg2 := []byte{3, 4, 5}
	fmt1, pcm1, err := splitWAV(testWAV(seg1))
	if err != nil {
		t.Fatalf("splitWAV seg1: %v", err)
	}
	_, pcm2, err := splitWAV(testWAV(seg2))
	if err != nil {
		t.Fatalf("splitWAV seg2: %v", err)
	}

	combined := buildWAV(fmt1, append(pcm1, pcm2...))
	_, pcm, err := splitWAV(combined)
	if err != nil {
		t.Fatalf("splitWAV combined: %v", err)
	}
	if !bytes.Equal(pcm, []byte{1, 2, 3, 4, 5}) {
		t.Errorf("combined pcm = %v, want [1 2 3 4 5]", pcm)
	}
	if got := binary.LittleEndian.Uint32(combined[4:8]); got != uint32(len(combined)-8) {
		t.Errorf("RIFF size = %d, want %d", got, len(combined)-8)
	}
}

func TestSplitWAVRejectsNonWAV(t *testing.T) {
	if _, _, err := splitWAV([]byte("not a wav")); err == nil {
		t.Fatal("error = nil, want parse error")
	}
	if _, _, err := splitWAV(nil); err == nil {
		t.Fatal("error = nil, want parse error for empty input")
	}
}
