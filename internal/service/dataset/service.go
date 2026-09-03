package dataset

import (
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
	tenantLLMDAO   *dao.TenantLLMDAO
	pipelineLogDAO *dao.PipelineOperationLogDAO
	userTenantDAO  *dao.UserTenantDAO
	taskDAO        *dao.TaskDAO
	searchService  *service.SearchService
	docEngine      engine.DocEngine
	embeddingCache *utility.EmbeddingLRU
}

// NewDatasetService creates a new datasets service.
func NewDatasetService() *DatasetService {
	return &DatasetService{
		kbDAO:          dao.NewKnowledgebaseDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		connectorDAO:   dao.NewConnectorDAO(),
		tenantDAO:      dao.NewTenantDAO(),
		tenantLLMDAO:   dao.NewTenantLLMDAO(),
		pipelineLogDAO: dao.NewPipelineOperationLogDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		taskDAO:        dao.NewTaskDAO(),
		searchService:  service.NewSearchService(),
		docEngine:      engine.Get(),
		embeddingCache: utility.NewEmbeddingLRU(1000),
	}
}
