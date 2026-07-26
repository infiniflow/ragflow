package dataset

import (
	"context"

	"ragflow/internal/engine/types"
)

// fakeChatDocEngine is a no-op DocEngine implementation for tests.
type fakeChatDocEngine struct{}

func (fakeChatDocEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (fakeChatDocEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) DropChunkStore(context.Context, string, string) error { return nil }
func (fakeChatDocEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (fakeChatDocEngine) CreateMetadataStore(context.Context, string) error { return nil }
func (fakeChatDocEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (fakeChatDocEngine) DropMetadataStore(context.Context, string) error           { return nil }
func (fakeChatDocEngine) MetadataStoreExists(context.Context, string) (bool, error) { return true, nil }
func (fakeChatDocEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (fakeChatDocEngine) DeleteDocument(context.Context, string, string) error { return nil }
func (fakeChatDocEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (fakeChatDocEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (fakeChatDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (fakeChatDocEngine) Ping(context.Context) error                          { return nil }
func (fakeChatDocEngine) Close() error                                        { return nil }
func (fakeChatDocEngine) GetType() string                                     { return "fake" }
func (fakeChatDocEngine) SupportsPageRank() bool                              { return false }
func (fakeChatDocEngine) FilterDocIdsByMetaPushdown(context.Context, []string, []map[string]interface{}, string) []string {
	return nil
}
