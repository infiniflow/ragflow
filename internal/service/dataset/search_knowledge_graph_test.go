package dataset

import (
	"context"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type graphSearchRecorder struct {
	datasets  []service.DatasetStructureGraphInput
	documents []service.DocumentStructureGraphInput
	templates []service.DocumentStructureGraphTemplate
}

func (r *graphSearchRecorder) GetDatasetStructure(_ context.Context, in service.DatasetStructureGraphInput) (*service.DatasetStructureGraphResponse, error) {
	r.datasets = append(r.datasets, in)
	return &service.DatasetStructureGraphResponse{Templates: r.templates}, nil
}

func (r *graphSearchRecorder) GetDocumentGraph(_ context.Context, in service.DocumentStructureGraphInput) (*service.DocumentStructureGraphResponse, error) {
	r.documents = append(r.documents, in)
	return &service.DocumentStructureGraphResponse{Templates: r.templates}, nil
}

func TestSearchKnowledgeGraphsRespectsDocumentScope(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	for _, doc := range []entity.Document{{ID: "allowed", KbID: "ds", ParserConfig: entity.JSONMap{}}, {ID: "outside", KbID: "other", ParserConfig: entity.JSONMap{}}} {
		if err := db.Create(&doc).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := &DatasetService{documentDAO: dao.NewDocumentDAO()}
	datasets := []*entity.Knowledgebase{{ID: "ds", TenantID: "owner"}}
	graphs := &graphSearchRecorder{templates: []service.DocumentStructureGraphTemplate{
		{TemplateID: "empty"},
		{TemplateID: "graph", Entities: []service.StructureGraphNode{{"name": "Daisy", "source_chunk_ids": []string{"chunk"}}},
			Relations: []service.StructureGraphRelation{{"from": "Daisy", "to": "Wisdom"}}},
	}}
	got, err := svc.searchKnowledgeGraphs(t.Context(), graphs, datasets, "Daisy", []string{"allowed", "outside"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs.datasets) != 0 || len(graphs.documents) != 1 {
		t.Fatalf("unexpected graph requests: datasets=%+v documents=%+v", graphs.datasets, graphs.documents)
	}
	if in := graphs.documents[0]; in.DocumentID != "allowed" || in.DatasetID != "ds" || in.TenantID != "owner" || in.Keywords != "Daisy" || in.GraphType != "knowledge_graph" {
		t.Fatalf("unexpected document graph input: %+v", in)
	}
	if len(got) != 1 || got[0].DocumentID != "allowed" || len(got[0].Templates) != 1 || got[0].Templates[0].TemplateID != "graph" {
		t.Fatalf("unexpected graph result: %+v", got)
	}
	got, err = svc.searchKnowledgeGraphs(t.Context(), graphs, datasets, "Daisy", nil, true)
	if err != nil || len(got) != 0 || len(graphs.datasets) != 0 || len(graphs.documents) != 1 {
		t.Fatalf("empty document scope widened the search: %+v, %v", got, err)
	}
	got, err = svc.searchKnowledgeGraphs(t.Context(), graphs, datasets, "Daisy", nil, false)
	if err != nil || len(got) != 1 || len(graphs.datasets) != 1 || graphs.datasets[0].TenantID != "owner" || got[0].DocumentID != "" {
		t.Fatalf("unexpected dataset graph search: %+v, %v, requests: %+v", got, err, graphs.datasets)
	}
}
