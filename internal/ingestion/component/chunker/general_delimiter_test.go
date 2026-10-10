package chunker

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/parser/parser"
)

func TestGeneralChunkerPreservesBareDelimiters(t *testing.T) {
	for _, fileType := range []string{"txt", "docx"} {
		for _, source := range []string{
			"第一句。第二句。",
			"First!Second?Third;第四句；第五句！第六句？",
			"\n第一句。\n\n第二句。\n",
			"第一句。  第二句。\t\n",
		} {
			t.Run(fileType+"/"+source, func(t *testing.T) {
				parsed := parser.NewTextParser().ParseWithResult(t.Context(), "document.txt", []byte(source))
				component, err := NewGeneralChunker(map[string]any{
					"chunk_token_size": 512,
					"delimiters":       []string{"\n", "!", "?", ";", "。", "；", "！", "？"},
				})
				if err != nil {
					t.Fatal(err)
				}
				out, err := component.Invoke(t.Context(), nil, map[string]any{
					"name": "document." + fileType, "file_type": fileType,
					"output_format": "json", "json": parsed.JSON,
				})
				if err != nil {
					t.Fatal(err)
				}
				if got := outputTexts(t, out); !reflect.DeepEqual(got, []string{source}) {
					t.Fatalf("chunks = %q, want original text %q", got, source)
				}
			})
		}
	}
}

func TestGeneralChunkerPreservesDelimitersAcrossChunks(t *testing.T) {
	const source = "Alpha one.\nBeta two; Gamma three!\nDelta four? Epsilon five。Zeta six；Eta seven！Theta eight？Iota nine."
	for _, fileType := range []string{"txt", "docx"} {
		t.Run(fileType, func(t *testing.T) {
			component, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 5,
				"delimiters":       []string{"\n", "!", "?", ";", "。", "；", "！", "？"},
			})
			if err != nil {
				t.Fatal(err)
			}
			input := generalTextInput(source)
			input["name"] = "document." + fileType
			input["file_type"] = fileType
			out, err := component.Invoke(t.Context(), nil, input)
			if err != nil {
				t.Fatal(err)
			}
			texts := outputTexts(t, out)
			if len(texts) < 2 {
				t.Fatalf("chunks = %q, want multiple chunks", texts)
			}
			if got := strings.Join(texts, ""); got != source {
				t.Fatalf("reconstructed text = %q, want %q", got, source)
			}
		})
	}
}

func TestGeneralChunkerDOCXPreservesParagraphBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  string
		second string
	}{
		{"implicit", "第一段。还有一句。", "第二段。"},
		{"trailing CRLF", "第一段。还有一句。\r\n", "第二段。"},
		{"trailing CR", "第一段。还有一句。\r", "第二段。"},
		{"leading CRLF", "第一段。还有一句。", "\r\n第二段。"},
		{"leading CR", "第一段。还有一句。", "\r第二段。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			component, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 512,
				"delimiters":       []string{"\n", "。"},
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := component.Invoke(t.Context(), nil, map[string]any{
				"name": "document.docx", "file_type": "docx", "output_format": "json",
				"json": []map[string]any{
					{"text": tc.first, "doc_type_kwd": "text"},
					{"text": tc.second, "doc_type_kwd": "text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := outputTexts(t, out), []string{"第一段。还有一句。\n第二段。"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("chunks = %q, want %q", got, want)
			}
		})
	}
}

func TestGeneralChunkerOverlapPreservesSentenceDelimiters(t *testing.T) {
	for _, fileType := range []string{"txt", "docx"} {
		t.Run(fileType, func(t *testing.T) {
			component, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 1, "overlapped_percent": 50,
				"delimiters": []string{"。"},
			})
			if err != nil {
				t.Fatal(err)
			}
			input := generalTextInput("甲乙。丙丁。")
			input["name"] = "document." + fileType
			input["file_type"] = fileType
			out, err := component.Invoke(t.Context(), nil, input)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := outputTexts(t, out), []string{"甲乙。", "乙。丙丁。"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("chunks = %q, want %q", got, want)
			}
		})
	}
}

func TestGeneralChunkerDelimiterSplitPreservesMetadata(t *testing.T) {
	for _, fileType := range []string{"txt", "docx"} {
		t.Run(fileType, func(t *testing.T) {
			component, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 0, "delimiters": []string{";"},
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := component.Invoke(t.Context(), nil, map[string]any{
				"name": "document." + fileType, "file_type": fileType, "output_format": "json",
				"json": []map[string]any{{
					"text": "alpha;beta", "doc_type_kwd": "text",
					"summary": "source summary", "keywords": "source keywords", "source_order": 3,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks := outputChunks(t, out)
			if len(chunks) != 2 {
				t.Fatalf("chunks = %#v, want two chunks", chunks)
			}
			for _, chunk := range chunks {
				if chunk["summary"] != "source summary" || chunk["keywords"] != "source keywords" || chunk["source_order"] != float64(3) {
					t.Fatalf("split lost source metadata: %#v", chunk)
				}
			}
		})
	}
}
