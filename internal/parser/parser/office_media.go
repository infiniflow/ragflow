//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

// Office media formats recorded on cache writes. DOC and PPT share the DOCX
// and PPTX parser families, so the family name is the format.
const (
	officeMediaFormatDOCX = "docx"
	officeMediaFormatPPTX = "pptx"
	officeMediaFormatXLSX = "xlsx"
)

// officeMediaSink hands Office image bytes to the run media cache and tracks
// per-document budget omissions. A nil sink means a text-only parse: image
// bytes are skipped without error because no run media cache exists. The
// normal DOCX/PPTX/XLSX parsers always construct a sink, so a missing cache
// fails the parse instead of silently dropping images.
type officeMediaSink struct {
	sink          *media.Sink
	format        string
	err           error
	invalidImages int
}

func newOfficeMediaSink(ctx context.Context, format string) *officeMediaSink {
	return &officeMediaSink{sink: media.NewSink(ctx, format, media.NewBudget()), format: format}
}

// put keeps Office payload identification local and delegates source admission,
// deduplication and cache failures to the shared media sink.
func (s *officeMediaSink) put(data []byte, mime string) (*entity.MediaRef, bool) {
	if s == nil || s.err != nil {
		return nil, false
	}
	// The IR supplies bytes rather than source paths. Memoize every digest,
	// including rejected sources, so repeated omissions are counted once.
	source := officeMediaSource(data)
	if ref, seen := s.sink.Lookup(source); seen && ref == nil {
		return nil, false
	}
	if len(data) == 0 {
		s.sink.Skip(source)
		s.invalidImages++
		return nil, false
	}
	if mime == "" {
		mime = officeImageMIME(data)
	}
	if mime == "" {
		s.sink.Skip(source)
		s.invalidImages++
		return nil, false
	}
	ref, err := s.sink.Resolve(source, media.Size(len(data)), func() (string, []byte, bool) {
		return mime, data, true
	})
	s.err = err
	return ref, ref != nil
}

func (s *officeMediaSink) warnings() []string {
	if s == nil {
		return nil
	}
	warnings := s.sink.Warnings()
	if s.invalidImages > 0 {
		warnings = append(warnings, fmt.Sprintf("%s parser skipped %d invalid or unsupported embedded image(s)", s.format, s.invalidImages))
	}
	return warnings
}

func officeMediaSource(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var (
	tiffLittleEndian = []byte("II*\x00")
	tiffBigEndian    = []byte("MM\x00*")
	emfSignature     = []byte(" EMF")
	utf8BOM          = []byte{0xEF, 0xBB, 0xBF}
)

// officeImageMIME identifies an embedded Office image from its bytes. The IR
// carries no filename or media type, so the payload itself must supply the
// real image MIME the cache stores. Returns "" when the bytes are not a
// recognised image; the parser skips that source with a warning.
func officeImageMIME(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	switch {
	case bytes.HasPrefix(data, tiffLittleEndian), bytes.HasPrefix(data, tiffBigEndian):
		return "image/tiff"
	case isEMFImage(data):
		return "image/x-emf"
	case isWMFImage(data):
		return "image/x-wmf"
	}
	if mime := http.DetectContentType(data); strings.HasPrefix(mime, "image/") {
		return mime
	}
	if isSVGImage(data) {
		return "image/svg+xml"
	}
	return ""
}

// isEMFImage matches the EMF header: a type DWORD of 1 followed by the
// " EMF" signature at the fixed header offset.
func isEMFImage(data []byte) bool {
	return len(data) >= 44 && binary.LittleEndian.Uint32(data) == 1 && bytes.Equal(data[40:44], emfSignature)
}

// isWMFImage matches the placeable-metafile magic and the standard WMF
// header (type 1 or 2, header size 9 words).
func isWMFImage(data []byte) bool {
	if bytes.HasPrefix(data, []byte{0xD7, 0xCD, 0xC6, 0x9A}) {
		return true
	}
	return len(data) >= 4 && (data[0] == 1 || data[0] == 2) && data[1] == 0 && data[2] == 9 && data[3] == 0
}

// isSVGImage matches an XML/SVG document without allocating a copy of the
// payload: DetectContentType reports text for SVG, and the Office IR does not
// carry the original extension.
func isSVGImage(data []byte) bool {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, utf8BOM))
	if len(trimmed) > 1024 {
		trimmed = trimmed[:1024]
	}
	if len(trimmed) >= 4 && bytes.EqualFold(trimmed[:4], []byte("<svg")) {
		return true
	}
	if len(trimmed) < 5 || !bytes.EqualFold(trimmed[:5], []byte("<?xml")) {
		return false
	}
	for i := 5; i+4 <= len(trimmed); i++ {
		if trimmed[i] == '<' && bytes.EqualFold(trimmed[i:i+4], []byte("<svg")) {
			return true
		}
	}
	return false
}
