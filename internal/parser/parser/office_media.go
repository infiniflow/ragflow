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
	"errors"
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
	ctx    context.Context
	cache  media.Cache
	budget *media.Budget
	format string
	// refs memoizes each image digest for the parse. A nil value means the
	// source was already rejected, so repeats neither charge the budget nor
	// inflate the warning counters.
	refs map[string]*entity.MediaRef

	err           error
	invalidImages int
	limitSkips    int
}

func newOfficeMediaSink(ctx context.Context, format string) *officeMediaSink {
	return &officeMediaSink{
		ctx:    ctx,
		cache:  media.CacheFromContext(ctx),
		budget: media.NewBudget(),
		format: format,
	}
}

// put validates one embedded image, charges it against the document budget
// and stores it in the run media cache. It returns false when the image must
// be skipped: the caller stays in sync with a warning already recorded here or
// on the budget. Cache failures other than the run limit set err, which the
// parser turns into a parse failure; a run-limit skip is a per-image warning.
func (s *officeMediaSink) put(data []byte, mime string) (*entity.MediaRef, bool) {
	if s == nil || s.err != nil {
		return nil, false
	}
	if len(data) == 0 {
		s.invalidImages++
		return nil, false
	}
	if mime == "" {
		mime = officeImageMIME(data)
		if mime == "" {
			s.invalidImages++
			return nil, false
		}
	}
	if s.cache == nil {
		s.err = fmt.Errorf("%s media cache is not configured", s.format)
		return nil, false
	}
	// The IR carries only bytes, so the digest is the stable source identity:
	// one repeated image shares a single budget admission, one cache write and
	// one MediaRef.
	source := officeMediaSource(data)
	if ref, seen := s.refs[source]; seen {
		if ref == nil {
			return nil, false
		}
		return ref, true
	}
	reservation, admission := s.budget.Reserve(media.Size(len(data)))
	if admission != media.Admitted || !reservation.Commit(media.Size(len(data))) {
		s.memoizeRef(source, nil)
		return nil, false
	}
	ref, err := s.cache.Put(s.ctx, s.format, mime, data)
	if err != nil {
		if errors.Is(err, media.ErrMediaLimitExceeded) {
			s.limitSkips++
			s.memoizeRef(source, nil)
			return nil, false
		}
		s.err = fmt.Errorf("%s media cache write: %w", s.format, err)
		return nil, false
	}
	s.memoizeRef(source, &ref)
	return &ref, true
}

func (s *officeMediaSink) warnings() []string {
	if s == nil {
		return nil
	}
	warnings := s.budget.Warnings()
	if s.invalidImages > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%s parser skipped %d invalid or unsupported embedded image(s)",
			s.format,
			s.invalidImages,
		))
	}
	if s.limitSkips > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%s parser skipped %d image(s) after the run media cache limit",
			s.format,
			s.limitSkips,
		))
	}
	return warnings
}

// officeMediaSource returns the digest used to dedupe budget admissions and
// cache writes for one parse.
func officeMediaSource(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *officeMediaSink) memoizeRef(source string, ref *entity.MediaRef) {
	if s.refs == nil {
		s.refs = make(map[string]*entity.MediaRef)
	}
	s.refs[source] = ref
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
	if mime := http.DetectContentType(data); strings.HasPrefix(mime, "image/") {
		return mime
	}
	switch {
	case bytes.HasPrefix(data, tiffLittleEndian), bytes.HasPrefix(data, tiffBigEndian):
		return "image/tiff"
	case isEMFImage(data):
		return "image/x-emf"
	case isWMFImage(data):
		return "image/x-wmf"
	case isSVGImage(data):
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
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<svg")) {
		return true
	}
	if !bytes.HasPrefix(lower, []byte("<?xml")) {
		return false
	}
	if len(lower) > 1024 {
		lower = lower[:1024]
	}
	return bytes.Contains(lower, []byte("<svg"))
}
