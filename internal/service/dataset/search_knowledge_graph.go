package dataset

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type knowledgeGraphReader interface {
	GetDocumentGraph(context.Context, service.DocumentStructureGraphInput) (*service.DocumentStructureGraphResponse, error)
	GetDatasetStructure(context.Context, service.DatasetStructureGraphInput) (*service.DatasetStructureGraphResponse, error)
}

func (d *DatasetService) searchKnowledgeGraphs(ctx context.Context, graphs knowledgeGraphReader, datasets []*entity.Knowledgebase, question string, docIDs []string, documentScoped bool) ([]service.KnowledgeGraphSearchResult, error) {
	var results []service.KnowledgeGraphSearchResult
	if documentScoped {
		docs, err := d.documentDAO.GetByIDs(ctx, dao.DB, docIDs)
		if err != nil {
			return nil, err
		}
		owners := make(map[string]string, len(datasets))
		for _, dataset := range datasets {
			owners[dataset.ID] = dataset.TenantID
		}
		for _, doc := range docs {
			tenant, authorized := owners[doc.KbID]
			if !authorized {
				continue
			}
			graph, err := graphs.GetDocumentGraph(ctx, service.DocumentStructureGraphInput{
				TenantID: tenant, DatasetID: doc.KbID, DocumentID: doc.ID,
				GraphType: "knowledge_graph", Keywords: question,
			})
			if err != nil {
				return nil, err
			}
			results = appendGraphMatches(results, doc.KbID, doc.ID, graph.Templates)
		}
		return results, nil
	}

	for _, dataset := range datasets {
		graph, err := graphs.GetDatasetStructure(ctx, service.DatasetStructureGraphInput{
			TenantID: dataset.TenantID, DatasetID: dataset.ID, Kind: "knowledge_graph", Keywords: question,
		})
		if err != nil {
			return nil, err
		}
		results = appendGraphMatches(results, dataset.ID, "", graph.Templates)
	}
	return results, nil
}

func appendGraphMatches(results []service.KnowledgeGraphSearchResult, datasetID, docID string, templates []service.DocumentStructureGraphTemplate) []service.KnowledgeGraphSearchResult {
	var matches []service.DocumentStructureGraphTemplate
	for _, template := range templates {
		if len(template.Entities) > 0 || len(template.Relations) > 0 {
			matches = append(matches, template)
		}
	}
	if len(matches) == 0 {
		return results
	}
	return append(results, service.KnowledgeGraphSearchResult{
		DatasetID: datasetID, DocumentID: docID, Templates: matches,
	})
}
