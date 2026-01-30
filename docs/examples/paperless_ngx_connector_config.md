# Paperless-ngx Connector Example Configuration

This example shows how to configure the Paperless-ngx connector in RAGFlow.

## Example 1: Basic Configuration with HTTPS

```json
{
  "source": "paperless_ngx",
  "name": "Paperless Documents",
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

## Example 2: Local Development (HTTP)

```json
{
  "source": "paperless_ngx",
  "name": "Local Paperless",
  "config": {
    "base_url": "http://localhost:8000",
    "verify_ssl": false,
    "batch_size": 5
  },
  "credentials": {
    "api_token": "dev-token"
  },
  "refresh_freq": 15
}
```

## Creating via API (curl)

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
    "refresh_freq": 30
  }'
```
