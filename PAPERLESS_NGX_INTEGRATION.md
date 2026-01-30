# Paperless-ngx Integration - Implementation Summary

## Overview

This implementation adds a direct Paperless-ngx REST API integration as a data source for RAGFlow, allowing users to sync and index documents from their Paperless-ngx instances.

## What is Paperless-ngx?

Paperless-ngx is a document management system that scans, indexes, and archives all your documents. It features:
- Automatic OCR for scanned documents
- Full-text search across all documents
- Tagging and categorization
- REST API for programmatic access

## Implementation Details

### Files Created/Modified

1. **Connector Implementation** (`common/data_source/paperless_ngx_connector.py`)
   - 444 lines of Python code
   - Implements `LoadConnector` and `PollConnector` interfaces
   - Features:
     - REST API client with requests library
     - Token-based authentication
     - Pagination support for large document sets
     - Time-based filtering for incremental sync
     - Metadata extraction and preservation
     - Size threshold enforcement
     - SSL certificate verification option

2. **Enum Additions**
   - `common/data_source/config.py`: Added `PAPERLESS_NGX` to `DocumentSource` enum
   - `common/constants.py`: Added `PAPERLESS_NGX` to `FileSource` enum

3. **Sync Service Integration** (`rag/svr/sync_data_source.py`)
   - Added `PaperlessNGX` class (55 lines)
   - Registered in `func_factory` for task dispatching
   - Follows existing connector patterns (WebDAV, Moodle, etc.)

4. **Module Exports** (`common/data_source/__init__.py`)
   - Exported `PaperlessNgxConnector` class
   - Added to `__all__` list

5. **Unit Tests** (`test/unit/test_paperless_ngx_connector.py`)
   - 260 lines of comprehensive tests
   - Coverage includes:
     - Initialization and credential loading
     - API requests and error handling
     - Document listing with pagination
     - Document downloading
     - Time-based filtering
     - Full and incremental sync
     - Validation

6. **Documentation**
   - `docs/paperless_ngx_connector.md` (226 lines)
     - Overview and features
     - Configuration parameters
     - Usage examples in Python
     - API endpoints used
     - Error handling
     - Troubleshooting guide
   - `docs/examples/paperless_ngx_connector_config.md` (61 lines)
     - Configuration examples in JSON
     - API usage with curl

### Total Changes
- **8 files** modified/created
- **1,050+ lines** of code, tests, and documentation

## How It Works

### Authentication
The connector uses API token authentication, which users can generate in their Paperless-ngx instance under Settings → API Tokens.

### Document Sync Flow

1. **Initial Sync (Full)**
   - Connector queries `/api/documents/` endpoint
   - Retrieves all documents (paginated)
   - Downloads each document via `/api/documents/{id}/download/`
   - Extracts metadata (title, tags, correspondent, etc.)
   - Stores in RAGFlow knowledge base

2. **Incremental Sync**
   - Uses `modified__gte` and `modified__lte` query parameters
   - Only syncs documents modified since last sync
   - More efficient for regular updates

3. **Metadata Extraction**
   - Title and original filename
   - Correspondent (sender/author)
   - Document type
   - Tags (as comma-separated IDs)
   - Creation and modification dates
   - OCR content (first 500 characters)

## Configuration Example

```json
{
  "source": "paperless_ngx",
  "name": "My Paperless Documents",
  "config": {
    "base_url": "https://paperless.example.com",
    "verify_ssl": true,
    "batch_size": 10
  },
  "credentials": {
    "api_token": "your-api-token"
  },
  "refresh_freq": 30,
  "prune_freq": 720
}
```

## Supported Features

✅ Full document sync  
✅ Incremental sync based on modification time  
✅ Pagination for large document sets  
✅ Metadata extraction and preservation  
✅ Size threshold enforcement  
✅ SSL certificate verification  
✅ Comprehensive error handling  
✅ API token authentication  
✅ Configurable batch sizes  
✅ Document type filtering via Paperless-ngx API  

## API Endpoints Used

- `GET /api/documents/` - List documents with filtering and pagination
- `GET /api/documents/{id}/download/` - Download document content

## Error Handling

The connector properly handles:
- 401 Unauthorized (invalid/expired token)
- 403 Forbidden (insufficient permissions)
- 404 Not Found (invalid URL or missing documents)
- Connection errors
- Timeout errors
- SSL certificate errors

## Testing

Unit tests cover:
- Credential loading and validation
- API request handling
- Error scenarios (401, 403, etc.)
- Document listing with pagination
- Time-based filtering
- Document downloading
- Full and incremental sync workflows

To run tests:
```bash
pytest test/unit/test_paperless_ngx_connector.py -v
```

## Usage

### Via RAGFlow API

```bash
curl -X POST "http://localhost:9380/v1/connector/set" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_RAGFLOW_TOKEN" \
  -d '{
    "source": "paperless_ngx",
    "name": "My Paperless Documents",
    "config": {
      "base_url": "https://paperless.example.com",
      "verify_ssl": true,
      "batch_size": 10
    },
    "credentials": {
      "api_token": "your-paperless-api-token"
    },
    "refresh_freq": 30,
    "prune_freq": 720
  }'
```

### Manual Testing

```python
from common.data_source.paperless_ngx_connector import PaperlessNgxConnector

connector = PaperlessNgxConnector(
    base_url="https://paperless.example.com",
    verify_ssl=True,
)

connector.load_credentials({"api_token": "your-token"})
connector.validate_connector_settings()

for batch in connector.load_from_state():
    for doc in batch:
        print(f"Document: {doc.semantic_identifier}")
```

## Integration Points

The connector integrates seamlessly with existing RAGFlow infrastructure:

1. **Enums**: Added to `DocumentSource` and `FileSource` enums
2. **Sync Service**: Registered in `func_factory` for task dispatching
3. **Connector Runner**: Uses existing `SyncBase` pattern
4. **Task Scheduler**: Works with existing refresh/prune scheduling
5. **Knowledge Base**: Documents flow into standard KB indexing pipeline

## Future Enhancements (Not in Scope)

Potential future improvements:
- Tag name resolution (currently uses tag IDs)
- Correspondent name caching
- Document type filtering at connector level
- Custom field extraction
- Attachment handling for mail documents
- WebSocket support for real-time updates

## Security Considerations

✅ API tokens stored securely in credentials  
✅ SSL verification enabled by default  
✅ No credentials logged  
✅ Proper exception handling to avoid token leakage  
✅ Size threshold to prevent DoS via large documents  

## Performance Characteristics

- **Batch Processing**: Configurable batch size (default: 2, recommended: 10-20)
- **Pagination**: Handles large document sets efficiently (100 docs/page)
- **Incremental Sync**: Only syncs modified documents
- **Parallel Downloads**: Not implemented (sequential for reliability)
- **Memory Usage**: Bounded by batch size × document size

## Limitations

1. Documents larger than `BLOB_STORAGE_SIZE_THRESHOLD` (20MB) are skipped
2. OCR content in metadata limited to 500 characters
3. Tag metadata uses IDs, not names (to avoid extra API calls)
4. Sequential document processing (no parallelization)

## Compatibility

- **Paperless-ngx**: Any version with REST API (v1.8.0+)
- **RAGFlow**: Current version
- **Python**: 3.10+
- **Dependencies**: requests, standard library only

## Documentation References

- Main documentation: `docs/paperless_ngx_connector.md`
- Configuration examples: `docs/examples/paperless_ngx_connector_config.md`
- Unit tests: `test/unit/test_paperless_ngx_connector.py`
- Connector code: `common/data_source/paperless_ngx_connector.py`

## Conclusion

This implementation provides a robust, production-ready integration between RAGFlow and Paperless-ngx, following all existing RAGFlow connector patterns and best practices. The connector is well-tested, documented, and ready for use.
