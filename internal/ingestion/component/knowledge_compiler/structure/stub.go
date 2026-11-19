//go:build !ee

package structure

import "ragflow/internal/ingestion/component/knowledge_compiler/common"

func postprocessAdditionalKind(_ Type, rows []common.Product) []common.Product {
	return rows
}
