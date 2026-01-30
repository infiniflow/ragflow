# Paperless-ngx Connector for RAGFlow

## Overview

This connector integrates Paperless-ngx document management system with RAGFlow, allowing you to index and search documents stored in Paperless-ngx.

## Features

- **REST API Integration**: Connects directly to Paperless-ngx REST API
- **Full & Incremental Sync**: Supports both full document sync and incremental updates
- **Metadata Extraction**: Captures document metadata including:
  - Title
  - Original filename
  - Correspondent
  - Document type
  - Tags
  - OCR content
  - Creation and modification dates
- **Pagination Support**: Handles large document collections efficiently
- **Time-based Filtering**: Syncs only documents modified within a specific time range
- **SSL Configuration**: Optional SSL certificate verification
- **Size Threshold**: Configurable document size limits

## Configuration

### Prerequisites

1. A running Paperless-ngx instance
2. API token from Paperless-ngx (Settings → API Tokens)

### Connector Settings

```json
{
  "source": "paperless_ngx",
  "name": "My Paperless-ngx",
  "config": {
    "base_url": "https://paperless.example.com",
    "verify_ssl": true,
    "batch_size": 10
  },
  "credentials": {
    "api_token": "your-api-token-here"
  },
  "refresh_freq": 30,
  "prune_freq": 720
}
```

### Configuration Parameters

- **base_url** (required): The base URL of your Paperless-ngx instance
  - Example: `https://paperless.example.com` or `http://localhost:8000`
  - The connector automatically normalizes URLs:
    - Missing `//` after scheme is fixed automatically: `http:192.168.1.6:8000` → `http://192.168.1.6:8000`
    - Missing scheme defaults to `https://`: `paperless.example.com` → `https://paperless.example.com`
    - Trailing slashes are removed automatically
  - **Note**: Always include the scheme (`http://` or `https://`) for clarity
- **verify_ssl** (optional): Whether to verify SSL certificates
  - Default: `true`
  - Set to `false` for self-signed certificates (not recommended for production)
- **batch_size** (optional): Number of documents to process in each batch
  - Default: `2` (from INDEX_BATCH_SIZE)
  - Adjust based on your system resources
- **api_token** (required): Paperless-ngx API authentication token
  - Generate in Paperless-ngx: Settings → API Tokens

### Creating an API Token in Paperless-ngx

1. Log in to your Paperless-ngx instance
2. Navigate to Settings → API Tokens
3. Click "Create Token"
4. Copy the generated token (it will only be shown once)
5. Use this token in the connector credentials

## Usage

### Via RAGFlow API

```python
import requests

# Create connector
response = requests.post(
    "http://localhost:9380/v1/connector/set",
    json={
        "name": "My Paperless Documents",
        "source": "paperless_ngx",
        "config": {
            "base_url": "https://paperless.example.com",
            "verify_ssl": true,
            "batch_size": 10
        },
        "credentials": {
            "api_token": "your-token-here"
        },
        "refresh_freq": 30,
        "prune_freq": 720
    },
    headers={"Authorization": "Bearer your-ragflow-token"}
)
```

### Sync Behavior

#### Full Sync (Initial or Reindex)
- Downloads all documents from Paperless-ngx
- Extracts full metadata and content
- Occurs on first sync or when manually triggered

#### Incremental Sync
- Downloads only documents modified since last sync
- Uses Paperless-ngx's `modified__gte` and `modified__lte` filters
- Runs automatically based on `refresh_freq` setting

## API Endpoints Used

The connector uses the following Paperless-ngx API endpoints:

- `GET /api/documents/` - List documents with filtering
- `GET /api/documents/{id}/download/` - Download document content

## Document Metadata

Each document indexed includes:

```json
{
  "id": "paperless_ngx:https://paperless.example.com:123",
  "source": "paperless_ngx",
  "semantic_identifier": "Document Title",
  "extension": ".pdf",
  "size_bytes": 12345,
  "doc_updated_at": "2024-01-15T10:30:00Z",
  "metadata": {
    "title": "Document Title",
    "original_filename": "document.pdf",
    "correspondent": "John Doe",
    "document_type": "Invoice",
    "tags": "1,2,3",
    "created": "2024-01-01T00:00:00Z",
    "ocr_content": "First 500 characters of OCR content..."
  }
}
```

## Error Handling

The connector handles the following error scenarios:

- **401 Unauthorized**: Invalid or expired API token
- **403 Forbidden**: Insufficient permissions
- **404 Not Found**: Invalid base URL or missing documents
- **Connection Errors**: Network issues, server down
- **Timeout Errors**: Slow responses or large downloads

## Limitations

- Documents larger than `BLOB_STORAGE_SIZE_THRESHOLD` (default: 20MB) are skipped
- OCR content in metadata is truncated to first 500 characters
- Tag metadata includes only tag IDs, not tag names (to avoid additional API calls)

## Troubleshooting

### Connection Issues

If you encounter connection errors:

1. Verify the `base_url` is correct and accessible
2. Check if Paperless-ngx is running
3. For HTTPS with self-signed certificates, set `verify_ssl: false` (development only)

### Authentication Issues

If you get 401 errors:

1. Verify the API token is correct
2. Check if the token has been revoked in Paperless-ngx
3. Generate a new token if needed

### No Documents Syncing

If no documents are syncing:

1. Check if there are documents in Paperless-ngx
2. Verify the time range filters (for incremental sync)
3. Check document size - very large documents may be skipped
4. Review connector logs for specific errors

## Development

### Running Tests

```bash
# Install test dependencies
uv sync --all-extras

# Run tests
pytest test/unit/test_paperless_ngx_connector.py -v
```

### Manual Testing

```python
from common.data_source.paperless_ngx_connector import PaperlessNgxConnector

# Create connector
connector = PaperlessNgxConnector(
    base_url="https://paperless.example.com",
    verify_ssl=True,
)

# Load credentials
connector.load_credentials({
    "api_token": "your-token-here"
})

# Validate settings
connector.validate_connector_settings()

# Load documents
for batch in connector.load_from_state():
    for doc in batch:
        print(f"Document: {doc.semantic_identifier}")
```

## Related Documentation

- [Paperless-ngx API Documentation](https://docs.paperless-ngx.com/api/)
- [RAGFlow Connector Development Guide](../../../docs/connector_development.md)
- [RAGFlow API Reference](../../../docs/api_reference.md)
