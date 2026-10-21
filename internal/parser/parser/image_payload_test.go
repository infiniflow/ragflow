package parser

import (
	"context"
	"strings"
	"testing"
)

func TestParsersNormalizeInlineImagePayload(t *testing.T) {
	markdown, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, payload, want string
	}{
		{"padded", "YQ==", "YQ=="},
		{"missing_two_padding", "YQ", "YQ=="},
		{"missing_one_padding", "YWI", "YWI="},
		{"no_padding_needed", "YWJj", "YWJj"},
		{"line_wrapped", "YW\r\nJj", "YWJj"},
		{"empty", "", ""},
		{"invalid_character", "Y?==", ""},
		{"space_inside", "Y WJj", ""},
		{"tab_inside", "YW\tJj", ""},
		{"partial_padding", "YQ=", ""},
		{"excess_padding", "YQ===", ""},
		{"padding_inside", "Y=Q=", ""},
		{"invalid_length", "Y", ""},
	} {
		for _, format := range []string{"html", "html_table", "markdown"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				uri := "data:image/png;base64," + tc.payload
				var result ParseResult
				switch format {
				case "html":
					result = NewHTMLParser().ParseWithResult(context.Background(), "image.html", []byte(`<p><img alt="caption" src="`+uri+`"></p>`))
				case "html_table":
					result = NewHTMLParser().ParseWithResult(context.Background(), "image.html", []byte(`<table><tr><td><img alt="caption" src="`+uri+`"></td></tr></table>`))
				case "markdown":
					result = markdown.ParseWithResult(context.Background(), "image.md", []byte("![caption](<"+uri+">)"))
				}
				if result.Err != nil {
					t.Fatal(result.Err)
				}
				images := 0
				for _, item := range result.JSON {
					image, _ := item.ToMap()["image"].(string)
					if image == "" {
						continue
					}
					images++
					if format != "markdown" {
						image = strings.TrimPrefix(image, "data:image/png;base64,")
					}
					if image != tc.want {
						t.Errorf("image payload = %q, want %q", image, tc.want)
					}
				}
				wantImages := 1
				if tc.want == "" {
					wantImages = 0
					if len(result.Warnings) == 0 {
						t.Error("rejected image should produce a parser warning")
					}
				}
				if images != wantImages {
					t.Errorf("images = %d, want %d; warnings = %v", images, wantImages, result.Warnings)
				}
			})
		}
	}
}

func TestNormalizeBase64ImagePayloadByteLimit(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		limit         int
		want          string
		wantSize      int64
	}{
		{"negative_limit", "YQ", -1, "", 0},
		{"zero_limit", "YQ", 0, "", 0},
		{"exact_limit_padded", "YWI=", 2, "YWI=", 2},
		{"exact_limit_unpadded", "YWI", 2, "YWI=", 2},
		{"exact_limit_wrapped", "YW\r\nI", 2, "YWI=", 2},
		{"over_limit_padded", "YWI=", 1, "", 0},
		{"over_limit_unpadded", "YWI", 1, "", 0},
		{"over_encoded_limit", "YWJjZA==", 2, "", 0},
		{"data_after_padding", "G0==JbjQ", 16, "", 0},
		{"data_after_padding_long", "aMRFPqfZALtY6rw50P==Pn5d", 16, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, size, ok := normalizeBase64ImagePayload(tc.payload, tc.limit)
			if got != tc.want || ok != (tc.want != "") || size != tc.wantSize {
				t.Errorf("normalizeBase64ImagePayload(%q, %d) = (%q, %d, %v), want (%q, %d, %v)",
					tc.payload, tc.limit, got, size, ok, tc.want, tc.wantSize, tc.want != "")
			}
		})
	}
}
