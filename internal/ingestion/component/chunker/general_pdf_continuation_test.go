package chunker

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestGeneralPDFJoinsCrossPageSentenceBeforeTokenMerge(t *testing.T) {
	for _, tc := range []struct {
		name       string
		delimiters []string
	}{
		{"newline", []string{"\n"}}, {"punctuation", []string{"。"}}, {"none", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := "为确保工作有序推进，领导小组下设综合工作组、物资保障"
			items := []map[string]any{
				{"text": first, "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 520, 619, 718}}},
				{"text": "组、宣传工作组。", "doc_type_kwd": "text", "positions": [][]float64{{4, 80, 197, 115, 130}}},
			}
			// Force the budget boundary to fall between the two source regions.
			out := invokePDFContinuation(t, items, tokenizeStr(first)-1, tc.delimiters, nil)
			if got, want := outputTexts(t, out), []string{first + "组、宣传工作组。"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("texts = %q, want %q", got, want)
			}
			chunks := outputChunks(t, out)
			if got, want := chunks[0]["positions"], [][]float64{{3, 80, 520, 619, 718}, {4, 80, 197, 115, 130}}; !reflect.DeepEqual(got, want) {
				t.Fatalf("positions = %#v, want %#v", got, want)
			}
		})
	}
}

func TestGeneralPDFCrossPageSentenceSplitKeepsSourcePositions(t *testing.T) {
	items := []map[string]any{
		{"text": "物资保障", "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 520, 700, 715}}},
		{"text": "组。下一段。", "doc_type_kwd": "text", "positions": [][]float64{{4, 80, 200, 100, 160}}},
	}
	out := invokePDFContinuation(t, items, 512, []string{"。"}, []string{"。"})
	if got, want := outputTexts(t, out), []string{"物资保障组。", "下一段。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %q, want %q", got, want)
	}
	chunks := outputChunks(t, out)
	for i, want := range [][][]float64{
		{{3, 80, 520, 700, 715}, {4, 80, 200, 100, 120}},
		{{4, 80, 200, 120, 160}},
	} {
		if got := chunks[i]["positions"]; !reflect.DeepEqual(got, want) {
			t.Errorf("chunk %d positions = %#v, want %#v", i, got, want)
		}
	}
}

func TestGeneralPDFCrossPageParentPositionsFollowSourceText(t *testing.T) {
	items := []map[string]any{
		{"text": "物资保障", "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 520, 700, 715}}},
		{"text": "组。下一段。", "doc_type_kwd": "text", "positions": [][]float64{{4, 80, 200, 100, 160}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"。"}, nil)
	if got, want := outputTexts(t, out), []string{"物资保障组。", "下一段。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts = %q, want %q", got, want)
	}
	chunks := outputChunks(t, out)
	if got, want := chunks[1]["positions"], [][]float64{{4, 80, 200, 120, 160}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %#v, want %#v", got, want)
	}
}

func TestGeneralPDFCrossPageEnglishKeepsWordSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "work with the\r\n", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "team to finish.", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"work with the team to finish."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageKanaJoinsWithoutSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "これは", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "テストです。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"これはテストです。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageMixedScriptKeepsSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "The word is", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "ひらがな", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"The word is ひらがな"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageHanLatinKeepsSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "使用", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "python 进行开发。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"使用 python 进行开发。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageHanKanaJoinsWithoutSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "日本語", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "の文章。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"日本語の文章。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageProlongedSoundMarkJoinsWithoutSpace(t *testing.T) {
	items := []map[string]any{
		{"text": "コンピュー", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "ター。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"コンピューター。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageAbbreviationIsNotListMarker(t *testing.T) {
	items := []map[string]any{
		{"text": "references,", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "e.g. this article.", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"references, e.g. this article."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageRejectsRegionBelowPreviousBottom(t *testing.T) {
	items := []map[string]any{
		{"text": "负责物资保障", "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 520, 100, 115}}},
		{"text": "下一段内容", "doc_type_kwd": "text", "positions": [][]float64{{4, 80, 520, 500, 515}}},
	}
	out := invokePDFContinuation(t, items, 512, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"负责物资保障\n下一段内容"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFContinuationRebuildsMismatchedSources(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "物资保障", DocType: "text", Positions: json.RawMessage(`[[3,80,520,700,715]]`)},
		{Text: "组、宣传工作组。", DocType: "text", Positions: json.RawMessage(`[[4,80,200,100,115]]`)},
	}
	joined, sources := joinPDFContinuations(units, [][]pdfSourceSpan{})
	if len(joined) != 1 {
		t.Fatalf("joined units = %d, want 1", len(joined))
	}
	if len(sources) != 1 || len(sources[0]) == 0 {
		t.Fatalf("sources = %#v, want one populated source list", sources)
	}
}

func TestGeneralPDFContinuationAcrossThreePagesKeepsEachSource(t *testing.T) {
	items := []map[string]any{
		{"text": "为保障工作", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "有序推进，设立物资保障", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 520, 100, 700}}},
		{"text": "组。", "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"为保障工作有序推进，设立物资保障组。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
	chunks := outputChunks(t, out)
	if got, want := chunks[0]["positions"], [][]float64{{1, 80, 520, 700, 715}, {2, 80, 520, 100, 700}, {3, 80, 200, 100, 115}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("positions=%#v, want %#v", got, want)
	}
}

func TestGeneralPDFCrossPageContinuationHonorsCustomDelimiter(t *testing.T) {
	items := []map[string]any{
		{"text": "物资保障SEP", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "组、宣传工作组。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
	}
	out := invokePDFContinuation(t, items, 1, []string{"`SEP`"}, nil)
	if got, want := outputTexts(t, out), []string{"物资保障", "组、宣传工作组。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageContinuationDoesNotCrossMedia(t *testing.T) {
	for _, kind := range []string{"image", "table"} {
		t.Run(kind, func(t *testing.T) {
			items := []map[string]any{
				{"text": "物资保障", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
				{"text": "插图或表格", "doc_type_kwd": kind, "positions": [][]float64{{1, 80, 520, 720, 760}}},
				{"text": "组、宣传工作组。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
			}
			out := invokePDFContinuation(t, items, 512, []string{"\n"}, nil)
			if got, want := outputTexts(t, out), []string{"物资保障", "插图或表格", "组、宣传工作组。"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("texts=%q, want %q", got, want)
			}
		})
	}
}

func TestGeneralPDFCrossPageSourcesResetAfterMedia(t *testing.T) {
	items := []map[string]any{
		{"text": "物资保障", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 700, 715}}},
		{"text": "组。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 200, 100, 115}}},
		{"text": "插图", "doc_type_kwd": "image", "positions": [][]float64{{2, 80, 520, 200, 300}}},
		{"text": "后文。", "doc_type_kwd": "text", "positions": [][]float64{{2, 80, 520, 400, 415}}},
	}
	out := invokePDFContinuation(t, items, 512, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"物资保障组。", "插图", "后文。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageTallParagraphKeepsIndentation(t *testing.T) {
	items := []map[string]any{
		{"text": "上一段没有结束标点", "doc_type_kwd": "text", "positions": [][]float64{{1, 80, 520, 600, 720}}},
		{"text": "新的缩进段落。", "doc_type_kwd": "text", "positions": [][]float64{{2, 112, 520, 100, 250}}},
	}
	out := invokePDFContinuation(t, items, 512, []string{"\n"}, nil)
	if got, want := outputTexts(t, out), []string{"上一段没有结束标点\n新的缩进段落。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("texts=%q, want %q", got, want)
	}
}

func TestGeneralPDFCrossPageContinuationKeepsParagraphBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, layout string
		page, left                  float64
	}{
		{"sentence ended", "第一段。", "下一段。", "", 4, 80},
		{"numbered item", "负责物资保障", "（二）宣传工作组", "", 4, 80},
		{"title", "负责物资保障", "管理办法", "title", 4, 80},
		{"new indentation", "负责物资保障", "下一段内容", "", 4, 112},
		{"same page", "负责物资保障", "下一段内容", "", 3, 80},
		{"missing page", "负责物资保障", "下一段内容", "", 5, 80},
		{"blank line", "负责物资保障\n\n", "下一段内容", "", 4, 80},
		{"spaced blank line", "负责物资保障\n \t\n", "下一段内容", "", 4, 80},
		{"lettered item", "负责物资保障", "b) 宣传工作组", "", 4, 80},
		{"lettered item no space", "负责物资保障", "b)宣传工作组", "", 4, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			top := 100.0
			if tc.page == 3 {
				top = 740
			}
			items := []map[string]any{
				{"text": tc.first, "doc_type_kwd": "text", "positions": [][]float64{{3, 80, 520, 700, 715}}},
				{"text": tc.second, "doc_type_kwd": "text", "layout_type": tc.layout, "positions": [][]float64{{tc.page, tc.left, 520, top, top + 15}}},
			}
			out := invokePDFContinuation(t, items, 512, []string{"\n"}, nil)
			blankLine := tc.name == "blank line" || tc.name == "spaced blank line"
			if got, want := outputTexts(t, out), []string{tc.first + "\n" + tc.second}; !blankLine && !reflect.DeepEqual(got, want) {
				t.Fatalf("texts=%q, want %q", got, want)
			}
			if blankLine {
				if got := outputTexts(t, out); len(got) != 1 || got[0] != tc.first+tc.second {
					t.Fatalf("lost paragraph boundary: %q", got)
				}
			}
		})
	}
}

func invokePDFContinuation(t *testing.T, items []map[string]any, budget int, delimiters, children []string) map[string]any {
	t.Helper()
	c, err := NewGeneralChunker(map[string]any{"chunk_token_size": budget, "delimiters": delimiters, "children_delimiters": children})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "continuation.pdf", "file_type": "pdf", "output_format": "json", "json": items})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
