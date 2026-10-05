package document

import (
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service"
)

// NewMetadataDocumentServiceForTest injects the metadata store for HTTP tests.
// Keeping this constructor in a test file avoids changing the production API.
func NewMetadataDocumentServiceForTest(docEngine engine.DocEngine) *DocumentService {
	return &DocumentService{
		documentDAO: dao.NewDocumentDAO(),
		docEngine:   docEngine,
		metadataSvc: service.NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), docEngine),
	}
}
