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
	"encoding/binary"
	"fmt"
)

// wavFormat carries the fmt chunk payload of a WAV file so concatenated PCM
// can be re-wrapped with the original encoding parameters.
type wavFormat struct {
	fmtChunk []byte
}

// splitWAV parses a RIFF/WAVE file and returns its fmt chunk payload and raw
// PCM data chunk. Chunks are 2-byte aligned per the RIFF spec.
func splitWAV(b []byte) (*wavFormat, []byte, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, nil, fmt.Errorf("not a RIFF/WAVE file (%d bytes)", len(b))
	}

	var fmtChunk, data []byte
	for off := 12; off+8 <= len(b); {
		chunkID := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		off += 8
		if size < 0 || off+size > len(b) {
			return nil, nil, fmt.Errorf("malformed WAV chunk %q: size %d exceeds buffer", chunkID, size)
		}
		switch chunkID {
		case "fmt ":
			fmtChunk = b[off : off+size]
		case "data":
			data = b[off : off+size]
		}
		off += size + (size & 1) // chunks are padded to even sizes
	}

	if fmtChunk == nil {
		return nil, nil, fmt.Errorf("WAV file has no fmt chunk")
	}
	if data == nil {
		return nil, nil, fmt.Errorf("WAV file has no data chunk")
	}
	return &wavFormat{fmtChunk: fmtChunk}, data, nil
}

// buildWAV wraps raw PCM data into a RIFF/WAVE file reusing the given fmt
// chunk (encoding, sample rate, channel count).
func buildWAV(f *wavFormat, pcm []byte) []byte {
	fmtSize := len(f.fmtChunk)
	dataPad := len(pcm) & 1
	total := 4 + (8 + fmtSize + fmtSize&1) + (8 + len(pcm) + dataPad)

	out := make([]byte, 0, 8+total)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(total))
	out = append(out, "WAVE"...)
	out = append(out, "fmt "...)
	out = binary.LittleEndian.AppendUint32(out, uint32(fmtSize))
	out = append(out, f.fmtChunk...)
	if fmtSize&1 == 1 {
		out = append(out, 0)
	}
	out = append(out, "data"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(pcm)))
	out = append(out, pcm...)
	if dataPad == 1 {
		out = append(out, 0)
	}
	return out
}
