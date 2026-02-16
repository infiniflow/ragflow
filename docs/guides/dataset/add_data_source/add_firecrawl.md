# Add Firecrawl

Use the Firecrawl native connector to crawl web content and sync markdown documents into your RAGFlow dataset.

## Prerequisites

- A Firecrawl account and API key
- A website URL you are authorized to crawl

## Configure in RAGFlow

1. Go to **User Settings → Data sources → Add source**.
2. Select **Firecrawl**.
3. Fill in:
   - **Firecrawl API Key**
   - **Start URL** (root URL to crawl)
   - **API URL** (optional, default: `https://api.firecrawl.dev`)
   - **Max Pages** (optional)
   - **Include Paths / Exclude Paths** (optional)
4. Save and run sync.

## Notes

- The connector retries Firecrawl rate-limit responses (`429`) with backoff.
- If the crawl does not finish before timeout, the sync fails with a timeout error.
- Malformed API responses are surfaced clearly in the sync logs.
