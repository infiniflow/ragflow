# Installation Guide for Firecrawl RAGFlow Integration

This guide will help you install and configure the Firecrawl integration plugin for RAGFlow.

## Prerequisites

- RAGFlow instance running (version 0.20.5 or later)
- Python 3.8 or higher
- Firecrawl API key (get one at [firecrawl.dev](https://firecrawl.dev))

## Installation Methods

### Method 1: Manual Installation

1. **Download the plugin**:
   ```bash
   git clone https://github.com/firecrawl/firecrawl.git
   cd firecrawl/ragflow-firecrawl-integration
   ```

2. **Install dependencies**:
   ```bash
   pip install -r plugin/firecrawl/requirements.txt
   ```

3. **Copy plugin to RAGFlow**:
   ```bash
   # Assuming RAGFlow is installed in /opt/ragflow
   cp -r plugin/firecrawl /opt/ragflow/plugin/
   ```

4. **Restart RAGFlow**:
   ```bash
   # Restart RAGFlow services
   docker compose -f /opt/ragflow/docker/docker-compose.yml restart
   ```

### Method 2: Using pip (if available)

```bash
pip install ragflow-firecrawl-integration
```

### Method 3: Development Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/firecrawl/firecrawl.git
   cd firecrawl/ragflow-firecrawl-integration
   ```

2. **Install in development mode**:
   ```bash
   pip install -e .
   ```

## Configuration

### 1. Get Firecrawl API Key

1. Visit [firecrawl.dev](https://firecrawl.dev)
2. Sign up for a free account
3. Navigate to your dashboard
4. Copy your API key (starts with `fc-`)

### 2. Configure in RAGFlow

1. **Access RAGFlow UI**:
   - Open your browser and go to your RAGFlow instance
   - Log in with your credentials

2. **Add Firecrawl Data Source**:
   - Go to "Data Sources" → "Add New Source"
   - Select "Firecrawl Web Scraper"
   - Enter your API key
   - Configure additional options if needed

3. **Test Connection**:
   - Click "Test Connection" to verify your setup
   - You should see a success message

## Configuration Options

| Option             | Description                      | Default                     | Required |
|--------------------|----------------------------------|-----------------------------|----------|
| `api_key`          | Your Firecrawl API key           | -                           | Yes      |
| `api_url`          | Firecrawl API endpoint           | `https://api.firecrawl.dev` | No       |
| `max_retries`      | Maximum retry attempts           | 3                           | No       |
| `timeout`          | Request timeout (seconds)        | 30                          | No       |
| `rate_limit_delay` | Delay between requests (seconds) | 1.0                         | No       |

## Environment Variables

You can also configure the plugin using environment variables:

```bash
export FIRECRAWL_API_KEY="fc-your-api-key-here"
export FIRECRAWL_API_URL="https://api.firecrawl.dev"
export FIRECRAWL_MAX_RETRIES="3"
export FIRECRAWL_TIMEOUT="30"
export FIRECRAWL_RATE_LIMIT_DELAY="1.0"
```

## Verification

### 1. Check Plugin Installation

```bash
# Check if the plugin directory exists
ls -la /opt/ragflow/plugin/firecrawl/

# Should show:
# __init__.py
# firecrawl_connector.py
# firecrawl_config.py
# firecrawl_processor.py
# firecrawl_ui.py
# ragflow_integration.py
# requirements.txt
```

### 2. Test the Integration

```bash
# Run the example script
cd /opt/ragflow/plugin/firecrawl/
python example_usage.py
```

### 3. Check RAGFlow Logs

```bash
# Check RAGFlow server logs
docker logs docker-ragflow-cpu-1

# Look for messages like:
# "Firecrawl plugin loaded successfully"
# "Firecrawl data source registered"
```

## Troubleshooting

### Common Issues

1. **Plugin not appearing in RAGFlow**:
   - Check if the plugin directory is in the correct location
   - Restart RAGFlow services
   - Check RAGFlow logs for errors

2. **API Key Invalid**:
   - Ensure your API key starts with `fc-`
   - Verify the key is active in your Firecrawl dashboard
   - Check for typos in the configuration

3. **Connection Timeout**:
   - Increase the timeout value in configuration
   - Check your network connection
   - Verify the API URL is correct

4. **Rate Limiting**:
   - Increase the `rate_limit_delay` value
   - Reduce the number of concurrent requests
   - Check your Firecrawl usage limits

### Debug Mode

Enable debug logging to see detailed information:

```python
import logging
logging.basicConfig(level=logging.DEBUG)
```

### Check Dependencies

```bash
# Verify all dependencies are installed
pip list | grep -E "(aiohttp|pydantic|requests)"

# Should show:
# aiohttp>=3.8.0
# pydantic>=2.0.0
# requests>=2.28.0
```

## Uninstallation

To remove the plugin:

1. **Remove plugin directory**:
   ```bash
   rm -rf /opt/ragflow/plugin/firecrawl/
   ```

2. **Restart RAGFlow**:
   ```bash
   docker compose -f /opt/ragflow/docker/docker-compose.yml restart
   ```

3. **Remove dependencies** (optional):
   ```bash
   pip uninstall ragflow-firecrawl-integration
   ```

## Support

If you encounter issues:

1. Check the [troubleshooting section](#troubleshooting)
2. Review RAGFlow logs for error messages
3. Verify your Firecrawl API key and configuration
4. Check the [Firecrawl documentation](https://docs.firecrawl.dev)
5. Open an issue in the [Firecrawl repository](https://github.com/firecrawl/firecrawl/issues)

## Next Steps

After successful installation:

1. Read the [README.md](README.md) for usage examples
2. Try scraping a simple URL to test the integration
3. Explore the different scraping options (single URL, crawl, batch)
4. Configure your RAGFlow workflows to use the scraped content
