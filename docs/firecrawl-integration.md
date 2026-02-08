# Firecrawl Integration for RAGFlow

This document describes how to use the Firecrawl integration to import web content into RAGFlow.

## Overview

[Firecrawl](https://firecrawl.dev) is a powerful web scraping API that converts websites into clean, LLM-ready markdown. This integration allows RAGFlow users to:

- **Scrape single URLs**: Import content from specific web pages
- **Batch scraping**: Scrape multiple URLs at once
- **Crawl entire sites**: Automatically discover and import content from websites

## Prerequisites

1. A Firecrawl API key (get one at https://firecrawl.dev)
2. RAGFlow instance running (v0.24.0 or later)

## Configuration

### Getting a Firecrawl API Key

1. Visit https://firecrawl.dev
2. Sign up for an account
3. Navigate to Settings → API Keys
4. Create a new API key
5. Copy the key for use in RAGFlow

### Adding Firecrawl as a Data Source

1. In RAGFlow, go to **Settings** → **Data Sources**
2. Click **Add Data Source**
3. Select **Firecrawl** from the available sources
4. Fill in the configuration:

| Field | Description | Required |
|-------|-------------|----------|
| Name | A descriptive name for this data source | Yes |
| Firecrawl API Key | Your Firecrawl API key | Yes |
| URLs to Scrape | Comma-separated list of specific URLs to scrape | No* |
| Crawl Base URL | Base URL to start crawling from | No* |
| Max Crawl Depth | How many levels deep to crawl (default: 2) | No |
| Include Paths | URL patterns to include (e.g., `/docs/*`) | No |
| Exclude Paths | URL patterns to exclude (e.g., `/admin/*`) | No |

*At least one of "URLs to Scrape" or "Crawl Base URL" must be provided.

## Usage Examples

### Example 1: Scraping Specific Pages

To import content from specific pages:

- **URLs to Scrape**: `https://docs.example.com/intro, https://docs.example.com/quickstart`

This will scrape only the specified URLs.

### Example 2: Crawling Documentation Site

To crawl an entire documentation site:

- **Crawl Base URL**: `https://docs.example.com`
- **Max Crawl Depth**: `3`
- **Include Paths**: `/docs/*, /guides/*`
- **Exclude Paths**: `/api/internal/*`

This will start from the base URL and discover all linked pages up to 3 levels deep, only including pages matching the include patterns.

### Example 3: Hybrid Approach

You can combine both methods:

- **URLs to Scrape**: `https://blog.example.com/important-post`
- **Crawl Base URL**: `https://docs.example.com`

This will scrape the specific blog post AND crawl the documentation site.

## Features

### Rate Limit Handling

The connector automatically handles Firecrawl's rate limits:
- Requests are spaced to avoid hitting limits
- If rate limited, the connector waits and retries
- Exponential backoff is used for retries

### Error Handling

The connector handles common errors gracefully:
- **401 Unauthorized**: Invalid or expired API key
- **403 Forbidden**: Access denied to specific URLs
- **404 Not Found**: URL doesn't exist
- **429 Too Many Requests**: Rate limited (auto-retry)
- **Connection errors**: Automatic retries with backoff

### Content Extraction

For each scraped page, the connector:
1. Extracts the main content as Markdown
2. Preserves page metadata (title, description, etc.)
3. Creates a RAGFlow document ready for chunking and embedding

## Sync Behavior

- **Initial Sync**: Scrapes all configured URLs and performs crawl
- **Incremental Sync**: Currently performs a full rescrape (Firecrawl doesn't have built-in change detection)

**Note**: For large sites, consider using Include/Exclude paths to limit the scope and reduce API usage.

## Troubleshooting

### Common Issues

1. **"Firecrawl API key is invalid"**
   - Verify your API key is correct
   - Check if the key has been revoked or expired

2. **"At least one URL or crawl URL must be specified"**
   - Provide either URLs to Scrape, a Crawl Base URL, or both

3. **"Unable to connect to Firecrawl API"**
   - Check your internet connection
   - Verify https://api.firecrawl.dev is accessible

4. **Crawl taking too long**
   - Reduce Max Crawl Depth
   - Use Include Paths to limit scope
   - Use Exclude Paths to skip unnecessary sections

### API Usage & Limits

Firecrawl has usage limits based on your plan:
- Free tier: Limited requests per month
- Paid tiers: Higher limits available

Monitor your usage in the Firecrawl dashboard.

## API Reference

The connector uses the following Firecrawl API endpoints:
- `POST /v1/scrape` - Scrape a single URL
- `POST /v1/crawl` - Start a crawl job
- `GET /v1/crawl/{id}` - Check crawl job status

For more details, see the [Firecrawl API documentation](https://docs.firecrawl.dev).

## Contributing

This integration is part of RAGFlow. To contribute improvements:

1. Fork the RAGFlow repository
2. Make your changes
3. Submit a pull request

## Support

- RAGFlow issues: https://github.com/infiniflow/ragflow/issues
- Firecrawl documentation: https://docs.firecrawl.dev
- Firecrawl support: https://firecrawl.dev/support
