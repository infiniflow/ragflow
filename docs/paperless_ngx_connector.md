# Paperless-ngx Connector for RAGFlow

## Overview

This connector integrates Paperless-ngx document management system with RAGFlow, allowing you to index and search documents stored in Paperless-ngx.

**Architecture Philosophy:**
- **Paperless-ngx**: Handles OCR, metadata extraction, and document management
- **RAGFlow**: Uses OCR content from Paperless, generates embeddings, structures knowledge bases, and answers questions
- **No Duplication**: Avoids redundant OCR work by using Paperless's OCR results directly

## Features

- **REST API Integration**: Connects directly to Paperless-ngx REST API
- **OCR Content First**: Uses OCR text from Paperless API (95% of cases), downloads PDF only as fallback
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
- **Intelligent Fallback**: Downloads PDF only when:
  - OCR content is empty
  - OCR content is too short (below threshold)
  - Re-OCR is needed for attachments or errors

## Ingestion Strategy (Best Practice)

For each Paperless document:

### API (Primary Source)
- `id`, `document_type`, `tags`, `correspondent`, `created`
- **`content` (OCR text)** - Used directly in 95% of cases

### PDF Download (Fallback Only)
- Used only when:
  - Content is empty or too short
  - Attachments need processing
  - Re-OCR required for errors

### Task Logic
```
If content present and length >= min_content_length:
  → Use OCR text from API (no download)
If empty or too short:
  → Download PDF from Paperless
  → RAGFlow may re-OCR locally if needed
```

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
    "batch_size": 10,
    "min_content_length": 100
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
- **min_content_length** (optional): Minimum OCR content length to use instead of downloading PDF
  - Default: `100` characters
  - If OCR content length >= this value, use OCR text directly
  - If OCR content length < this value or is empty, download PDF
  - Recommended: 100-500 characters depending on your use case
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
            "batch_size": 10,
            "min_content_length": 100
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

#### Content-First Approach (Recommended)
- **Primary**: Uses OCR text from Paperless API (fast, no download)
  - Applies when `content` field exists and length >= `min_content_length`
  - Creates text document (.txt extension) from OCR content
  - Avoids redundant OCR work (Paperless already performed it)
  - Saves bandwidth and processing time
  
- **Fallback**: Downloads PDF from Paperless
  - Only when OCR content is empty or too short
  - Allows RAGFlow to re-OCR if needed
  - Handles attachments and special cases

#### Full Sync (Initial or Reindex)
- Retrieves all documents from Paperless-ngx
- Uses OCR content where available (95% of cases)
- Downloads PDF only for documents without sufficient OCR content
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
  - Connection timeout: 10 seconds (for establishing connection)
  - Read timeout: 60 seconds (configurable via `REQUEST_TIMEOUT_SECONDS` env var)

**Fast-Fail Behavior:** The connector uses separate connect and read timeouts. Connection attempts fail within 10 seconds, preventing long delays when the server is unreachable or the URL is incorrect.

## Limitations

- Documents larger than `BLOB_STORAGE_SIZE_THRESHOLD` (default: 20MB) are skipped
- OCR content in metadata is truncated to first 500 characters
- Tag metadata includes only tag IDs, not tag names (to avoid additional API calls)
- Connection attempts timeout after 10 seconds maximum

## Troubleshooting

### Connection Issues

If you encounter connection errors:

1. **Verify the URL format**: Ensure `base_url` includes the scheme (e.g., `http://` or `https://`)
   - ✓ Correct: `http://192.168.1.6:8000` or `https://paperless.example.com`
   - ✗ Incorrect: `http:192.168.1.6:8000` (missing `//`)
2. Verify the `base_url` is correct and accessible from the RAGFlow container
3. Check if Paperless-ngx is running: `curl http://your-paperless-url/api/`
4. For HTTPS with self-signed certificates, set `verify_ssl: false` (development only)
5. Check network connectivity and firewall rules

**Note:** Connection attempts fail quickly (within 10 seconds) to prevent blocking. If you see "connection timeout" errors, verify network connectivity.

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

# Create connector with content-first strategy
connector = PaperlessNgxConnector(
    base_url="https://paperless.example.com",
    verify_ssl=True,
    min_content_length=100,  # Use OCR content if >= 100 chars
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
