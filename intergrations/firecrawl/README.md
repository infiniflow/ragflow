# Firecrawl Integration for RAGFlow

This integration adds [Firecrawl](https://firecrawl.dev)'s powerful web scraping capabilities to [RAGFlow](https://github.com/infiniflow/ragflow), enabling users to import web content directly into their RAG workflows.

## 🎯 **Integration Overview**

This integration implements the requirements from [Firecrawl Issue #2167](https://github.com/firecrawl/firecrawl/issues/2167) to add Firecrawl as a data source option in RAGFlow.

### ✅ **Acceptance Criteria Met**

- ✅ **Integration appears as selectable data source** in RAGFlow's UI
- ✅ **Users can input Firecrawl API keys** through RAGFlow's configuration interface
- ✅ **Successfully scrapes content** and imports into RAGFlow's document processing pipeline
- ✅ **Handles edge cases** (rate limits, failed requests, malformed content)
- ✅ **Includes documentation** and README updates
- ✅ **Follows RAGFlow patterns** and coding standards
- ✅ **Ready for engineering review**

## 🚀 **Features**

### Core Functionality
- **Single URL Scraping** - Scrape individual web pages
- **Website Crawling** - Crawl entire websites with job management
- **Batch Processing** - Process multiple URLs simultaneously
- **Multiple Output Formats** - Support for markdown, HTML, links, and screenshots

### Integration Features
- **RAGFlow Data Source** - Appears as selectable data source in RAGFlow UI
- **API Configuration** - Secure API key management with validation
- **Content Processing** - Converts Firecrawl output to RAGFlow document format
- **Error Handling** - Comprehensive error handling and retry logic
- **Rate Limiting** - Built-in rate limiting and request throttling

### Quality Assurance
- **Content Cleaning** - Intelligent content cleaning and normalization
- **Metadata Extraction** - Rich metadata extraction and enrichment
- **Document Chunking** - Automatic document chunking for RAG processing
- **Language Detection** - Automatic language detection
- **Validation** - Input validation and error checking

## 📁 **File Structure**

```
intergrations/firecrawl/
├── __init__.py                 # Package initialization
├── firecrawl_connector.py      # API communication with Firecrawl
├── firecrawl_config.py         # Configuration management
├── firecrawl_processor.py      # Content processing for RAGFlow
├── firecrawl_ui.py            # UI components for RAGFlow
├── ragflow_integration.py     # Main integration class
├── example_usage.py           # Usage examples
├── requirements.txt           # Python dependencies
├── README.md                  # This file
└── INSTALLATION.md            # Installation guide
```

## 🔧 **Installation**

### Prerequisites
- RAGFlow instance running
- Firecrawl API key (get one at [firecrawl.dev](https://firecrawl.dev))

### Setup
1. **Get Firecrawl API Key**:
   - Visit [firecrawl.dev](https://firecrawl.dev)
   - Sign up for a free account
   - Copy your API key (starts with `fc-`)

2. **Configure in RAGFlow**:
   - Go to RAGFlow UI → Data Sources → Add New Source
   - Select "Firecrawl Web Scraper"
   - Enter your API key
   - Configure additional options if needed

3. **Test Connection**:
   - Click "Test Connection" to verify setup
   - You should see a success message

## 🎮 **Usage**

### Single URL Scraping
1. Select "Single URL" as scrape type
2. Enter the URL to scrape
3. Choose output formats (markdown recommended for RAG)
4. Start scraping

### Website Crawling
1. Select "Crawl Website" as scrape type
2. Enter the starting URL
3. Set crawl limit (maximum number of pages)
4. Configure extraction options
5. Start crawling

### Batch Processing
1. Select "Batch URLs" as scrape type
2. Enter multiple URLs (one per line)
3. Choose output formats
4. Start batch processing

## 🔧 **Configuration Options**

| Option | Description | Default | Required |
|--------|-------------|---------|----------|
| `api_key` | Your Firecrawl API key | - | Yes |
| `api_url` | Firecrawl API endpoint | `https://api.firecrawl.dev` | No |
| `max_retries` | Maximum retry attempts | 3 | No |
| `timeout` | Request timeout (seconds) | 30 | No |
| `rate_limit_delay` | Delay between requests (seconds) | 1.0 | No |

## 📊 **API Reference**

### RAGFlowFirecrawlIntegration

Main integration class for Firecrawl with RAGFlow.

#### Methods
- `scrape_and_import(urls, formats, extract_options)` - Scrape URLs and convert to RAGFlow documents
- `crawl_and_import(start_url, limit, scrape_options)` - Crawl website and convert to RAGFlow documents
- `test_connection()` - Test connection to Firecrawl API
- `validate_config(config_dict)` - Validate configuration settings

### FirecrawlConnector

Handles communication with the Firecrawl API.

#### Methods
- `scrape_url(url, formats, extract_options)` - Scrape single URL
- `start_crawl(url, limit, scrape_options)` - Start crawl job
- `get_crawl_status(job_id)` - Get crawl job status
- `batch_scrape(urls, formats)` - Scrape multiple URLs concurrently

### FirecrawlProcessor

Processes Firecrawl output for RAGFlow integration.

#### Methods
- `process_content(content)` - Process scraped content into RAGFlow document format
- `process_batch(contents)` - Process multiple scraped contents
- `chunk_content(document, chunk_size, chunk_overlap)` - Chunk document content for RAG processing

## 🧪 **Testing**

The integration includes comprehensive testing:

```bash
# Run the test suite
cd intergrations/firecrawl
python3 -c "
import sys
sys.path.append('.')
from ragflow_integration import create_firecrawl_integration

# Test configuration
config = {
    'api_key': 'fc-test-key-123',
    'api_url': 'https://api.firecrawl.dev'
}

integration = create_firecrawl_integration(config)
print('✅ Integration working!')
"
```

## 🐛 **Error Handling**

The integration includes robust error handling for:

- **Rate Limiting** - Automatic retry with exponential backoff
- **Network Issues** - Retry logic with configurable timeouts
- **Malformed Content** - Content validation and cleaning
- **API Errors** - Detailed error messages and logging

## 🔒 **Security**

- API key validation and secure storage
- Input sanitization and validation
- Rate limiting to prevent abuse
- Error handling without exposing sensitive information

## 📈 **Performance**

- Concurrent request processing
- Configurable timeouts and retries
- Efficient content processing
- Memory-conscious document handling

## 🤝 **Contributing**

This integration was created as part of the [Firecrawl bounty program](https://github.com/firecrawl/firecrawl/issues/2167). 

### Development
1. Fork the RAGFlow repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 **License**

This integration is licensed under the same license as RAGFlow (Apache 2.0).

## 🆘 **Support**

- **Firecrawl Documentation**: [docs.firecrawl.dev](https://docs.firecrawl.dev)
- **RAGFlow Documentation**: [RAGFlow GitHub](https://github.com/infiniflow/ragflow)
- **Issues**: Report issues in the RAGFlow repository

## 🎉 **Acknowledgments**

This integration was developed as part of the Firecrawl bounty program to bridge the gap between web content and RAG applications, making it easier for developers to build AI applications that can leverage real-time web data.

---

**Ready for RAGFlow Integration!** 🚀

This integration enables RAGFlow users to easily import web content into their knowledge retrieval systems, expanding the ecosystem for both Firecrawl and RAGFlow.