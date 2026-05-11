# Doc Engine Implementation

RAGFlow Go document engine implementation, supporting Elasticsearch and Infinity storage engines.

## Directory Structure

```
internal/engine/
├── engine.go              # DocEngine interface definition
├── engine_factory.go      # Factory function
├── global.go              # Global engine instance management
├── elasticsearch/         # Elasticsearch implementation
│   ├── client.go          # ES client initialization
│   ├── search.go          # Search implementation
│   ├── index.go           # Index operations
│   └── document.go        # Document operations
└── infinity/              # Infinity implementation
    ├── client.go          # Infinity client initialization (placeholder)
    ├── search.go          # Search implementation (placeholder)
    ├── index.go           # Table operations (placeholder)
    └── document.go        # Document operations (placeholder)
```

## Configuration

### Using Elasticsearch

Add to `conf/service_conf.yaml`:

```yaml
doc_engine:
  type: elasticsearch
  es:
    hosts: "http://localhost:9200"
    username: "elastic"
    password: "infini_rag_flow"
```

### Using Infinity

```yaml
doc_engine:
  type: infinity
  infinity:
    uri: "localhost:23817"
    postgres_port: 5432
    db_name: "default_db"
```

**Note**: Infinity implementation is a placeholder waiting for the official Infinity Go SDK. Only Elasticsearch is fully functional at this time.

## Usage

### 1. Initialize Engine

The engine is automatically initialized on service startup (see `cmd/server_main.go`):

```go
// Initialize doc engine
if err := engine.Init(&cfg.DocEngine); err != nil {
    log.Fatalf("Failed to initialize doc engine: %v", err)
}
defer engine.Close()
```

### 2. Use in Service

In `ChunkService`:

```go
type ChunkService struct {
    docEngine engine.DocEngine
    engineType config.EngineType
}

func NewChunkService() *ChunkService {
    cfg := config.Get()
    return &ChunkService{
        docEngine:  engine.Get(),
        engineType: cfg.DocEngine.Type,
    }
}

// Search
func (s *ChunkService) RetrievalTest(req *RetrievalTestRequest) (*RetrievalTestResponse, error) {
    ctx := context.Background()

    switch s.engineType {
    case config.EngineElasticsearch:
        // Use Elasticsearch retrieval
        searchReq := &elasticsearch.SearchRequest{
            IndexNames: []string{"chunks"},
            Query:      elasticsearch.BuildMatchTextQuery([]string{"content"}, req.Question, "AUTO"),
            Size:       10,
        }
        result, _ := s.docEngine.Search(ctx, searchReq)
        esResp := result.(*elasticsearch.SearchResponse)
        // Process result...

    case config.EngineInfinity:
        // Infinity not implemented yet
        return nil, fmt.Errorf("infinity not yet implemented")
    }
}
```

### 3. Direct Use of Global Engine

```go
import "ragflow/internal/engine"

// Get engine instance
docEngine := engine.Get()

// Search
searchReq := &elasticsearch.SearchRequest{
    IndexNames: []string{"my_index"},
    Query:      elasticsearch.BuildTermQuery("status", "active"),
}
result, err := docEngine.Search(ctx, searchReq)

// Index operations
err = docEngine.CreateIndex(ctx, "my_index", mapping)
err = docEngine.DeleteIndex(ctx, "my_index")
exists, _ := docEngine.IndexExists(ctx, "my_index")

// Document operations
err = docEngine.IndexDocument(ctx, "my_index", "doc_id", docData)
bulkResp, _ := docEngine.BulkIndex(ctx, "my_index", docs)
doc, _ := docEngine.GetDocument(ctx, "my_index", "doc_id")
err = docEngine.DeleteDocument(ctx, "my_index", "doc_id")
```

## API Documentation

### DocEngine Interface

```go
type DocEngine interface {
    // Search
    Search(ctx context.Context, req interface{}) (interface{}, error)

    // Index operations
    CreateIndex(ctx context.Context, indexName string, mapping interface{}) error
    DeleteIndex(ctx context.Context, indexName string) error
    IndexExists(ctx context.Context, indexName string) (bool, error)

    // Document operations
    IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error
    BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error)
    GetDocument(ctx context.Context, indexName, docID string) (interface{}, error)
    DeleteDocument(ctx context.Context, indexName, docID string) error

    // Health check
    Ping(ctx context.Context) error
    Close() error
}
```

## Dependencies

### Elasticsearch
- `github.com/elastic/go-elasticsearch/v8`

### Infinity
- **Not available yet** - Waiting for official Infinity Go SDK

## Notes

1. **Type Conversion**: The `Search` method returns `interface{}`, requiring type assertion based on engine type
2. **Model Definitions**: Each engine has its own request/response models defined in their respective packages
3. **Error Handling**: It's recommended to handle errors uniformly in the service layer and return user-friendly error messages
4. **Performance Optimization**: For large volumes of documents, prefer using `BulkIndex` for batch operations
5. **Connection Management**: The engine is automatically closed when the program exits, no manual management needed
6. **Infinity Status**: Infinity implementation is currently a placeholder. Only Elasticsearch is fully functional.

## Extending with New Engines

To add a new document engine (e.g., Milvus, Qdrant):

1. Create a new directory under `internal/engine/`, e.g., `milvus/`
2. Implement four files: `client.go`, `search.go`, `index.go`, `document.go`
3. Add corresponding creation logic in `engine_factory.go`
4. Add configuration structure in `config.go`
5. Update service layer code to support the new engine

## Correspondence with Python Project

| Python Module | Go Module |
|--------------|-----------|
| `common/doc_store/doc_store_base.py` | `internal/engine/engine.go` |
| `rag/utils/es_conn.py` | `internal/engine/elasticsearch/` |
| `rag/utils/infinity_conn.py` | `internal/engine/infinity/` (placeholder) |
| `common/settings.py` | `internal/config/config.go` |

## Current Status

- ✅ Elasticsearch: Fully implemented and functional
- ⏳ Infinity: Placeholder implementation, waiting for official Go SDK
- 📋 OceanBase: Not implemented (removed from requirements)
