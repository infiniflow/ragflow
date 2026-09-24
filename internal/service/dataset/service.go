package dataset

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// DatasetService implements the RESTful dataset APIs.
type DatasetService struct {
	kbDAO          *dao.KnowledgebaseDAO
	documentDAO    *dao.DocumentDAO
	connectorDAO   *dao.ConnectorDAO
	tenantDAO      *dao.TenantDAO
	pipelineLogDAO *dao.PipelineOperationLogDAO
	userTenantDAO  *dao.UserTenantDAO
	taskDAO        *dao.TaskDAO
	searchService  *service.SearchService
	docEngine      engine.DocEngine
	embeddingCache *utility.EmbeddingLRU
	// tagVocabularyLoader resolves the selectable-tag vocabulary for a tag
	// source file (parser_config.tags.tag_file_id). It is a field so tests can
	// inject a fake; in production it defaults to
	// component.TagVocabularyFromTagFileID (see AggregateTags). The ownerTenantID
	// it receives is the dataset's tenant: the loader must prove the file belongs
	// to it, since tag_file_id is user-writable (IDOR, CWE-639).
	tagVocabularyLoader func(ctx context.Context, tagFileID, ownerTenantID string) (map[string]int, error)
}

// NewDatasetService creates a new datasets service.
func NewDatasetService() *DatasetService {
	return &DatasetService{
		kbDAO:          dao.NewKnowledgebaseDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		connectorDAO:   dao.NewConnectorDAO(),
		tenantDAO:      dao.NewTenantDAO(),
		pipelineLogDAO: dao.NewPipelineOperationLogDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		taskDAO:        dao.NewTaskDAO(),
		searchService:  service.NewSearchService(),
		docEngine:      engine.Get(),
		embeddingCache: utility.NewEmbeddingLRU(1000),
	}
}
