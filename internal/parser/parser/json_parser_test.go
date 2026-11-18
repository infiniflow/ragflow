package parser

import (
	"context"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

func jsonTexts(res ParseResult) []string {
	texts := make([]string, 0, len(res.JSON))
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		texts = append(texts, text)
	}
	return texts
}

func assertJSONTexts(t *testing.T, res ParseResult, want ...string) {
	t.Helper()
	got := jsonTexts(res)
	if len(got) != len(want) {
		t.Fatalf("got %d items %q, want %q", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestJSONParser_SplitsArrayAndLines(t *testing.T) {
	p := NewJSONParser()
	res := p.ParseWithResult(context.Background(), "a.json", []byte(`[{"q":"a"},{"q":"b"}]`))
	assertJSONTexts(t, res, `{"q":"a"}`, `{"q":"b"}`)

	res = p.ParseWithResult(context.Background(), "a.jsonl", []byte("{\"q\":\"a\"}\n{\"q\":\"b\"}\n"))
	assertJSONTexts(t, res, `{"q":"a"}`, `{"q":"b"}`)
}

// Each number keeps the digits it was written with, and <, > and & stay
// readable. Decoded into a float64 and re-encoded with json.Marshal, an id
// above 2^53 changed digits and "a < b" became "a \u003c b".
func TestJSONParser_KeepsValuesAsWritten(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, file, data string
		want             []string
	}{
		{
			name: "object",
			file: "a.json",
			data: `{"id": 12345678901234567890, "price": 1.50, "note": "a < b && c > d"}`,
			want: []string{`{"id":12345678901234567890,"note":"a < b && c > d","price":1.50}`},
		},
		{
			name: "array",
			file: "a.json",
			data: `[{"id": 9007199254740993}, 1e5]`,
			want: []string{`{"id":9007199254740993}`, `1e5`},
		},
		{
			name: "lines",
			file: "a.jsonl",
			data: "{\"id\": 12345678901234567890}\n{\"html\": \"<b>x</b>\"}\n",
			want: []string{`{"id":12345678901234567890}`, `{"html":"<b>x</b>"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONTexts(t, p.ParseWithResult(context.Background(), tc.file, []byte(tc.data)), tc.want...)
		})
	}
}

// Files saved by Windows editors and Excel start with a UTF-8 BOM. The shape
// detection looks at the first byte, so an undecoded BOM turned the whole
// file into a single raw text item.
func TestJSONParser_UTF8BOM(t *testing.T) {
	p := NewJSONParser()
	bom := "\xef\xbb\xbf"

	res := p.ParseWithResult(context.Background(), "a.json", []byte(bom+`[{"q":"a"},{"q":"b"}]`))
	assertJSONTexts(t, res, `{"q":"a"}`, `{"q":"b"}`)

	res = p.ParseWithResult(context.Background(), "a.jsonl", []byte(bom+"{\"q\":\"a\"}\n{\"q\":\"b\"}\n"))
	assertJSONTexts(t, res, `{"q":"a"}`, `{"q":"b"}`)
}

// Non-UTF-8 files are decoded before json.Unmarshal, which would otherwise
// replace every invalid byte with U+FFFD.
func TestJSONParser_DecodesNonUTF8(t *testing.T) {
	src := `[{"q":"中文问题"}]`
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}
	utf16le, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("UTF-16LE encode: %v", err)
	}
	cases := []struct {
		name     string
		data     []byte
		encoding string
	}{
		{"gbk", gbk, "gb18030"},
		{"utf-16le", utf16le, "utf-16le"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := NewJSONParser().ParseWithResult(context.Background(), "a.json", tc.data)
			assertJSONTexts(t, res, `{"q":"中文问题"}`)
			if res.File["encoding"] != tc.encoding {
				t.Errorf("encoding = %v, want %q", res.File["encoding"], tc.encoding)
			}
		})
	}
}
