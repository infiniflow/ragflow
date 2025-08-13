# MonkeyOCR Complete Integration with RAGFlow

## 🎉 Phase 2 Complete: Full Production Integration

This document describes the complete integration of MonkeyOCR with RAGFlow, including database integration, API endpoints, and Docker deployment.

## 📋 Implementation Summary

### ✅ Phase 1 Completed
- Core MonkeyOCR processor (`deepdoc/vision/monkey_ocr.py`)
- RAGFlow parser integration (`rag/app/monkey_ocr_parser.py`)
- Configuration system (`conf/monkey_ocr_config.json`)

### ✅ Phase 2 Completed
- Database integration (`api/db/services/monkeyocr_service.py`)
- API endpoints (`api/apps/monkeyocr_app.py`)
- Docker deployment setup
- Complete production-ready integration

## 🏗️ Architecture Overview

```
RAGFlow + MonkeyOCR Integration
├── Core Components
│   ├── deepdoc/vision/monkey_ocr.py          # Core processor
│   ├── rag/app/monkey_ocr_parser.py          # RAG parser
│   └── api/db/services/monkeyocr_service.py  # Database service
├── API Layer
│   └── api/apps/monkeyocr_app.py             # REST API endpoints
├── Configuration
│   ├── conf/monkey_ocr_config.json           # Integration config
│   └── api/db/__init__.py                    # Parser type registration
└── Deployment
    ├── Dockerfile.monkeyocr                   # Docker image
    ├── docker-compose-monkeyocr.yml          # Docker Compose
    ├── docker/init_monkeyocr.sh              # Initialization script
    └── requirements_monkeyocr.txt             # Dependencies
```

## 🚀 Quick Start with Docker

### 1. Clone and Setup
```bash
git clone <ragflow-repo>
cd ragflow
```

### 2. Build and Run with Docker Compose
```bash
# Build and start all services
docker-compose -f docker-compose-monkeyocr.yml up -d

# Check status
docker-compose -f docker-compose-monkeyocr.yml ps

# View logs
docker-compose -f docker-compose-monkeyocr.yml logs -f ragflow
```

### 3. Access Services
- **RAGFlow Web UI**: http://localhost:80
- **API Endpoints**: http://localhost:9380/api/v1/monkeyocr/
- **Health Check**: http://localhost:9380/api/v1/monkeyocr/health

## 📊 API Endpoints

### Core Endpoints

#### 1. Parser Information
```bash
GET /api/v1/monkeyocr/info
```
Returns parser capabilities and version information.

#### 2. Parser Registration
```bash
POST /api/v1/monkeyocr/register
Content-Type: application/json

{
  "tenant_id": "your-tenant-id"
}
```

#### 3. Document Parsing
```bash
POST /api/v1/monkeyocr/parse
Content-Type: multipart/form-data

file: <uploaded-file>
split_pages: false
pred_abandon: false
```

#### 4. Text Extraction from Images
```bash
POST /api/v1/monkeyocr/extract-text
Content-Type: application/json

{
  "image_paths": ["/path/to/image1.jpg", "/path/to/image2.png"],
  "task": "text"
}
```

#### 5. Health Check
```bash
GET /api/v1/monkeyocr/health
```

### Additional Endpoints
- `GET /api/v1/monkeyocr/supported-formats` - Get supported file formats
- `POST /api/v1/monkeyocr/validate-file` - Validate file compatibility
- `GET /api/v1/monkeyocr/parsing-options` - Get parsing options
- `GET /api/v1/monkeyocr/available/<tenant_id>` - Check parser availability

## 🔧 Configuration

### Environment Variables
```bash
# Enable MonkeyOCR
MONKEYOCR_ENABLED=true

# Configuration paths
MONKEYOCR_CONFIG_PATH=/app/conf/monkey_ocr_config.json
MONKEYOCR_MODEL_PATH=/app/monkeyocr/model_weight
MONKEYOCR_CACHE_DIR=/app/monkeyocr/cache

# Database configuration
DATABASE_TYPE=mysql
DATABASE_HOST=mysql
DATABASE_PORT=3306
DATABASE_NAME=ragflow
DATABASE_USER=ragflow
DATABASE_PASSWORD=ragflow123

# Redis configuration
REDIS_HOST=redis
REDIS_PORT=6379

# Elasticsearch configuration
ELASTICSEARCH_HOST=es01
ELASTICSEARCH_PORT=9200
ELASTICSEARCH_USERNAME=elastic
ELASTICSEARCH_PASSWORD=ragflow123
```

### Configuration File
The integration uses `conf/monkey_ocr_config.json` for detailed configuration:

```json
{
  "monkeyocr": {
    "enabled": true,
    "config_path": "/app/monkeyocr/model_configs.yaml",
    "model_weights_path": "/app/monkeyocr/model_weight",
    "supported_formats": [".pdf", ".jpg", ".jpeg", ".png"],
    "max_image_dimension": 1280,
    "default_dpi": 150,
    "batch_size": 2,
    "device": "cpu",
    "capabilities": {
      "document_layout_analysis": true,
      "text_extraction": true,
      "formula_recognition": true,
      "table_extraction": true,
      "image_ocr": true,
      "omr_processing": true
    },
    "parsing_options": {
      "split_pages": false,
      "pred_abandon": false,
      "extract_images": true,
      "generate_layout_pdf": true,
      "generate_spans_pdf": true
    },
    "performance": {
      "enable_caching": true,
      "cache_dir": "/app/monkeyocr/cache",
      "max_cache_size": "1GB",
      "enable_parallel_processing": true,
      "max_workers": 4
    }
  },
  "integration": {
    "ragflow_parser_registry": true,
    "auto_register": true,
    "priority": 10,
    "fallback_parser": "default"
  }
}
```

## 🐳 Docker Deployment

### Services Included
1. **RAGFlow App** - Main application with MonkeyOCR integration
2. **MySQL** - Database for RAGFlow
3. **Redis** - Caching and queues
4. **Elasticsearch** - Vector storage
5. **Nginx** - Reverse proxy

### Volumes
- `monkeyocr_models` - Persistent model storage
- `monkeyocr_cache` - Persistent cache storage
- `mysql_data` - Database data
- `redis_data` - Redis data
- `esdata01` - Elasticsearch data

### Health Checks
All services include health checks to ensure proper startup order and monitoring.

## 🔍 Usage Examples

### Python API Usage
```python
import requests

# Register parser
response = requests.post('http://localhost:9380/api/v1/monkeyocr/register',
                        json={'tenant_id': 'your-tenant-id'})

# Parse document
with open('document.pdf', 'rb') as f:
    files = {'file': f}
    data = {'split_pages': 'false', 'pred_abandon': 'false'}
    response = requests.post('http://localhost:9380/api/v1/monkeyocr/parse',
                           files=files, data=data)

# Extract text from images
response = requests.post('http://localhost:9380/api/v1/monkeyocr/extract-text',
                        json={
                            'image_paths': ['/path/to/image.jpg'],
                            'task': 'text'
                        })
```

### Direct Service Usage
```python
from api.db.services.monkeyocr_service import MonkeyOCRService

# Create service instance
service = MonkeyOCRService()

# Register parser
service.register_parser('tenant-id')

# Parse document
result = service.parse_document(
    doc_id='doc-123',
    file_path='/path/to/document.pdf',
    split_pages=False
)

# Extract text from images
text_results = service.extract_text_from_images(
    image_paths=['/path/to/image.jpg'],
    task='text'
)
```

## 🧪 Testing

### Health Check
```bash
curl http://localhost:9380/api/v1/monkeyocr/health
```

### Parser Info
```bash
curl http://localhost:9380/api/v1/monkeyocr/info
```

### Document Parsing Test
```bash
curl -X POST http://localhost:9380/api/v1/monkeyocr/parse \
  -F "file=@test_document.pdf" \
  -F "split_pages=false"
```

## 🔧 Troubleshooting

### Common Issues

1. **Model Download Failures**
   ```bash
   # Check model directory
   docker exec ragflow-app ls -la /app/monkeyocr/model_weight

   # Manual model download
   docker exec ragflow-app bash /app/docker/init_monkeyocr.sh
   ```

2. **Memory Issues**
   ```bash
   # Increase memory limits in docker-compose
   services:
     ragflow:
       mem_limit: 4g
   ```

3. **Import Errors**
   ```bash
   # Check dependencies
   docker exec ragflow-app pip list | grep monkeyocr

   # Rebuild container
   docker-compose -f docker-compose-monkeyocr.yml build --no-cache
   ```

### Logs
```bash
# View application logs
docker-compose -f docker-compose-monkeyocr.yml logs ragflow

# View MonkeyOCR specific logs
docker exec ragflow-app tail -f /app/logs/monkeyocr.log
```

## 📈 Performance Optimization

### GPU Support
For GPU acceleration, modify the Docker setup:

```yaml
# docker-compose-monkeyocr.yml
services:
  ragflow:
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

### Caching
Enable caching in configuration:
```json
{
  "monkeyocr": {
    "performance": {
      "enable_caching": true,
      "cache_dir": "/app/monkeyocr/cache",
      "max_cache_size": "2GB"
    }
  }
}
```

### Parallel Processing
```json
{
  "monkeyocr": {
    "performance": {
      "enable_parallel_processing": true,
      "max_workers": 8
    }
  }
}
```

## 🔒 Security Considerations

1. **API Authentication** - All endpoints require login
2. **File Validation** - Input files are validated before processing
3. **Resource Limits** - Docker containers have memory and CPU limits
4. **Network Isolation** - Services communicate through internal network

## 📚 Additional Resources

- [MonkeyOCR Documentation](https://github.com/Yuliang-Liu/MonkeyOCR)
- [RAGFlow Documentation](https://docs.ragflow.io)
- [Docker Documentation](https://docs.docker.com)

## 🤝 Support

For issues and questions:
1. Check the troubleshooting section
2. Review logs: `docker-compose logs ragflow`
3. Verify configuration settings
4. Check health endpoint: `/api/v1/monkeyocr/health`

## 📄 License

This integration follows the same license as the main RAGFlow project and MonkeyOCR.
