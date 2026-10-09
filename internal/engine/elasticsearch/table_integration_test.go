//go:build integration

package elasticsearch_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/ingestion/task/indexdoc"
	"ragflow/internal/parser/parser"
	"ragflow/internal/server/config"
	"ragflow/internal/tokenizer"
	es "ragflow/internal/engine/elasticsearch"
)

func TestTableColumnModeRoundTrip(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("set ES_TEST=1 for real Elasticsearch")
	}
	if err := common.InitLogger("error", common.FileOutput{}, "column_mode_test"); err != nil {
		t.Fatal(err)
	}
	hosts := common.GetEnv(common.EnvESHost)
	if hosts == "" {
		hosts = "http://localhost:1200"
	}
	user := common.GetEnv(common.EnvESUsername)
	if user == "" {
		user = "elastic"
	}
	password := common.GetEnv(common.EnvESPassword)
	if password == "" {
		password = "infini_rag_flow"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	e, err := es.NewEngine(ctx, config.ElasticsearchConfig{Hosts: hosts, Username: user, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, format := range []string{"csv", "xlsx"} {
		for _, mode := range []string{"auto", "manual", "metadata-only"} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				tenant := fmt.Sprintf("column_test_%d", time.Now().UnixNano())
				base, kb, doc := "ragflow_"+tenant, "columns", "document"
				defer func() {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cleanupCancel()
					for _, index := range []string{base, "ragflow_doc_meta_" + tenant} {
						if err := e.DropChunkStore(cleanupCtx, index, ""); err != nil {
							t.Errorf("cleanup %s: %v", index, err)
						}
					}
				}()
				if err := e.CreateChunkStore(ctx, base, kb, 2, "table"); err != nil {
					t.Fatal(err)
				}
				if err := e.CreateMetadataStore(ctx, tenant); err != nil {
					t.Fatal(err)
				}
				headers := []any{"名称", "金额", "备注", "未指定"}
				values := []any{"visibleword", "secretword", "bothword", "defaultword"}
				var parsed parser.ParseResult
				if format == "csv" {
					parsed = parser.NewCSVParser().ParseWithResult(ctx, "table.csv", []byte("名称,金额,备注,未指定\nvisibleword,secretword,bothword,defaultword\n"))
				} else {
					workbook := excelize.NewFile()
					defer workbook.Close()
					if err := workbook.SetSheetRow("Sheet1", "A1", &headers); err != nil {
						t.Fatal(err)
					}
					if err := workbook.SetSheetRow("Sheet1", "A2", &values); err != nil {
						t.Fatal(err)
					}
					buf, err := workbook.WriteToBuffer()
					if err != nil {
						t.Fatal(err)
					}
					p, err := parser.NewXLSXParser("")
					if err != nil {
						t.Fatal(err)
					}
					parsed = p.ParseWithResult(ctx, "table.xlsx", buf.Bytes())
				}
				if parsed.Err != nil {
					t.Fatal(parsed.Err)
				}
				roles := map[string]any{"名称": "indexing", "金额": "metadata", "备注": "both"}
				effectiveMode := mode
				if mode == "metadata-only" {
					effectiveMode = "manual"
					roles = map[string]any{"名称": "metadata", "金额": "metadata", "备注": "metadata", "未指定": "metadata"}
				}
				component, err := chunker.NewTableChunker(map[string]any{"column_mode": effectiveMode, "column_roles": roles})
				if err != nil {
					t.Fatal(err)
				}
				output, err := component.Invoke(ctx, nil, map[string]any{"name": "table." + format, "file_type": format, "output_format": "json", "json": parsed.JSON})
				if err != nil {
					t.Fatal(err)
				}
				if output["_ERROR"] != nil {
					t.Fatalf("chunker: %v", output["_ERROR"])
				}
				chunks, ok := output["chunks"].([]map[string]any)
				if !ok || len(chunks) != 1 {
					t.Fatalf("chunks = %#v", output)
				}
				profile, metadata := indexdoc.ProjectTableChunks(chunks, "elasticsearch")
				if profile == nil {
					t.Fatal("no indexed field profile")
				}
				if mode == "auto" && len(metadata) != 0 {
					t.Fatalf("auto aggregated metadata: %#v", metadata)
				}
				if mode == "manual" && (metadata["金额"] == nil || metadata["备注"] == nil || metadata["未指定"] != nil) {
					t.Fatalf("manual aggregates = %#v", metadata)
				}
				for _, chunk := range chunks {
					tokens, err := tokenizer.Tokenize(fmt.Sprint(chunk["text"]))
					if err != nil {
						t.Fatal(err)
					}
					chunk["content_ltks"] = tokens
					fine, err := tokenizer.FineGrainedTokenize(tokens)
					if err != nil {
						t.Fatal(err)
					}
					chunk["content_sm_ltks"] = fine
				}
				if _, err := indexdoc.ProcessChunksForPipeline(chunks, doc, "table."+format, time.Now()); err != nil {
					t.Fatal(err)
				}
				if _, err := e.InsertChunks(ctx, chunks, base, kb); err != nil {
					t.Fatal(err)
				}
				id := fmt.Sprint(chunks[0]["id"])
				raw, err := e.GetChunk(ctx, base, id, []string{kb})
				if err != nil {
					t.Fatal(err)
				}
				stored, ok := raw.(map[string]interface{})
				if !ok {
					t.Fatalf("stored = %#v", raw)
				}
				body := fmt.Sprint(stored["content_with_weight"])
				data, ok := stored["chunk_data"].(map[string]interface{})
				if !ok {
					t.Fatalf("chunk_data = %#v", stored)
				}
				if data[common.TableDataKey("金额")] != "secretword" || fmt.Sprint(stored["table_row_int"]) != "1" {
					t.Fatalf("structured row = %#v", stored)
				}
				if mode == "auto" && (!strings.Contains(body, "secretword") || len(data) != 4) {
					t.Fatalf("auto body=%q data=%#v", body, data)
				}
				if mode == "manual" {
					if strings.Contains(body, "secretword") || !strings.Contains(body, "visibleword") || !strings.Contains(body, "bothword") || !strings.Contains(body, "defaultword") {
						t.Fatalf("manual body = %q", body)
					}
					if _, exists := data[common.TableDataKey("名称")]; exists {
						t.Fatal("indexing-only value entered JSON")
					}
				}
				if mode == "metadata-only" && body != "Sheet 1, row 2" {
					t.Fatalf("locator = %q", body)
				}
				// Metadata-only values cannot enter a normal body search.
				result, err := e.Search(ctx, &types.SearchRequest{IndexNames: []string{base}, KbIDs: []string{kb}, Filter: map[string]interface{}{"available_int": 1}, Limit: 10, MatchExprs: []interface{}{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "secretword"}}})
				if err != nil {
					t.Fatal(err)
				}
				want := int64(0)
				if mode == "auto" {
					want = 1
				}
				if result.Total != want {
					t.Fatalf("body search total=%d want=%d", result.Total, want)
				}
				for key := range metadata {
					profile.OwnedMetadata = append(profile.OwnedMetadata, key)
				}
				if metadata == nil {
					metadata = map[string]any{}
				}
				encoded, err := profile.Encode()
				if err != nil {
					t.Fatal(err)
				}
				metadata[entity.TableProfileMetadataField] = encoded
				metadata["user"] = "kept"
				if _, err := e.InsertMetadata(ctx, []map[string]interface{}{{"id": doc, "kb_id": kb, "meta_fields": metadata}}, tenant); err != nil {
					t.Fatal(err)
				}
				keys := append([]string{entity.TableProfileMetadataField}, profile.OwnedKeys()...)
				if err := e.DeleteMetadataKeys(ctx, doc, kb, keys, tenant); err != nil {
					t.Fatal(err)
				}
				meta, err := e.SearchMetadata(ctx, &types.SearchMetadataRequest{TenantID: tenant, Limit: 10, Filter: map[string]interface{}{"id": doc}})
				if err != nil {
					t.Fatal(err)
				}
				if len(meta.MetadataRecords) != 1 {
					t.Fatalf("metadata rows = %#v", meta)
				}
				fields, ok := meta.MetadataRecords[0]["meta_fields"].(map[string]interface{})
				if !ok || len(fields) != 1 || fields["user"] != "kept" {
					t.Fatalf("revoked metadata = %#v", meta.MetadataRecords)
				}
				if err := e.UpdateChunks(ctx, map[string]interface{}{"id": id, "doc_id": doc}, map[string]interface{}{"available_int": 0}, base, kb); err != nil {
					t.Fatal(err)
				}
				result, err = e.Search(ctx, &types.SearchRequest{IndexNames: []string{base}, KbIDs: []string{kb}, Filter: map[string]interface{}{"available_int": 1}, Limit: 10})
				if err != nil {
					t.Fatal(err)
				}
				if result.Total != 0 {
					t.Fatalf("disabled row is searchable: %#v", result)
				}
				if _, err := e.DeleteChunks(ctx, map[string]interface{}{"id": id, "doc_id": doc}, base, kb); err != nil {
					t.Fatal(err)
				}
				raw, err = e.GetChunk(ctx, base, id, []string{kb})
				if err != nil {
					t.Fatal(err)
				}
				if raw != nil {
					t.Fatalf("deleted row = %#v", raw)
				}
			})
		}
	}
}
