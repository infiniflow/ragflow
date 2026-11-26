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

package media

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestMetafileMIMEIgnoresNonMetafileGzip(t *testing.T) {
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write([]byte("a gzip-compressed text document")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got := MetafileMIME(compressed.Bytes()); got != "" {
		t.Fatalf("ordinary gzip MIME = %q, want no metafile MIME", got)
	}
}

func TestMetafileFontFallback(t *testing.T) {
	wmf := make([]byte, 18+56)
	copy(wmf, []byte{1, 0, 9, 0, 0, 3})
	binary.LittleEndian.PutUint32(wmf[18:], 28)
	binary.LittleEndian.PutUint16(wmf[22:], 0x02fb)
	wmf[37] = 134                      // GBK charset in the WMF LOGFONT.
	copy(wmf[42:], "\xcb\xce\xcc\xe5") // 宋体 in GBK.

	emf := make([]byte, 88+104)
	binary.LittleEndian.PutUint32(emf, 1)
	binary.LittleEndian.PutUint32(emf[4:], 88)
	copy(emf[40:], " EMF")
	binary.LittleEndian.PutUint32(emf[88:], 82)
	binary.LittleEndian.PutUint32(emf[92:], 104)
	copy(emf[128:], "S\x00i\x00m\x00S\x00u\x00n\x00")

	fonts := map[string]bool{"noto serif cjk sc": true}
	for _, tc := range []struct {
		name       string
		data       []byte
		faceOffset int
		want       []byte
	}{
		{"WMF", wmf, 42, []byte("Noto Serif CJK SC\x00")},
		{"EMF", emf, 128, []byte("N\x00o\x00t\x00o\x00 \x00S\x00e\x00r\x00i\x00f\x00 \x00C\x00J\x00K\x00 \x00S\x00C\x00\x00\x00")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.data)
			got := substituteMetafileFonts(tc.data, fonts)
			if !bytes.HasPrefix(got[tc.faceOffset:], tc.want) {
				t.Fatalf("font field = %x, want prefix %x", got[tc.faceOffset:], tc.want)
			}
			if !bytes.Equal(tc.data, original) {
				t.Fatal("the original metafile was modified")
			}
			if got := substituteMetafileFonts(tc.data[:len(tc.data)-1], fonts); !bytes.Equal(got, tc.data[:len(tc.data)-1]) {
				t.Fatal("a truncated record was modified")
			}
		})
	}
	if got := substituteMetafileFonts(wmf, map[string]bool{"宋体": true, "noto serif cjk sc": true}); !bytes.Equal(got, wmf) {
		t.Fatal("the available original font was replaced")
	}
	for _, tc := range []struct {
		name       string
		kind, size uint32
		options    int
	}{
		{"ExtTextOutW", 84, 80, 52},
		{"SmallTextOut", 108, 40, 20},
		{"PolyTextOutW", 97, 104, 80},
	} {
		t.Run(tc.name+" glyph indexes", func(t *testing.T) {
			record := make([]byte, tc.size)
			binary.LittleEndian.PutUint32(record, tc.kind)
			binary.LittleEndian.PutUint32(record[4:], tc.size)
			if tc.kind == 97 {
				binary.LittleEndian.PutUint32(record[36:], 2)
				// The first descriptor omits its rectangle and uses characters.
				binary.LittleEndian.PutUint32(record[56:], 0x0100)
			}
			binary.LittleEndian.PutUint32(record[tc.options:], 0x0010)
			// Put the text after the font to test discarding earlier rewrites.
			data := append(bytes.Clone(emf), record...)
			if got := substituteMetafileFonts(data, fonts); !bytes.Equal(got, data) {
				t.Fatal("font substitution changed a metafile with glyph-index text")
			}
		})
	}
}

func TestSinkNormalizesMetafilesAcrossDocuments(t *testing.T) {
	for _, tc := range []struct{ document, ext, mime string }{
		{"pptx", "wmf", "image/x-wmf"},
		{"docx", "emf", "image/x-emf"},
		{"xlsx", "wmf", "image/x-wmf"},
		{"html", "emf", "application/octet-stream"},
	} {
		t.Run(tc.document, func(t *testing.T) {
			data, err := os.ReadFile("testdata/metafile_test." + tc.ext)
			if err != nil {
				t.Fatal(err)
			}
			cache := new(sinkTestCache)
			sink := NewSink(WithCache(t.Context(), cache), tc.document, NewBudget())
			ref, err := sink.Resolve("image", Size(len(data)), func() (string, []byte, bool) {
				return tc.mime, data, true
			})
			if err != nil || ref == nil {
				t.Fatalf("reference = %v, error = %v, warnings = %v", ref, err, sink.Warnings())
			}
			if ref.MIME != "image/png" {
				t.Fatalf("cached MIME = %q, want image/png", ref.MIME)
			}
			img, err := png.Decode(bytes.NewReader(cache.data))
			if err != nil {
				t.Fatal(err)
			}
			center := img.Bounds().Min.Add(img.Bounds().Size().Div(2))
			r, g, b, _ := img.At(center.X, center.Y).RGBA()
			if r < 0x8000 || g > 0x2000 || b > 0x2000 {
				t.Fatalf("expected a red rectangle at %v, got %04x %04x %04x", center, r, g, b)
			}
		})
	}
}

func TestSinkKeepsInvalidMetafileAndContinues(t *testing.T) {
	cache := new(sinkTestCache)
	sink := NewSink(WithCache(t.Context(), cache), "docx", NewBudget())
	for range 2 {
		ref, err := sink.Resolve("bad", 4, func() (string, []byte, bool) {
			return "image/x-wmf", []byte{0xd7, 0xcd, 0xc6, 0x9a}, true
		})
		if ref == nil || ref.MIME != "image/x-wmf" || err != nil {
			t.Fatalf("invalid WMF: reference = %v, error = %v", ref, err)
		}
	}
	ref, err := sink.Resolve("next", 5, func() (string, []byte, bool) {
		return "image/png", []byte("image"), true
	})
	if ref == nil || err != nil || cache.writes != 2 {
		t.Fatalf("next image: reference = %v, error = %v, writes = %d", ref, err, cache.writes)
	}
	if warnings := sink.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "kept 1 image(s) in original format") {
		t.Fatalf("warnings = %v", warnings)
	}
}
