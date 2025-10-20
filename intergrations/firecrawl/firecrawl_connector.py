"""
Main connector class for integrating Firecrawl with RAGFlow.
"""

import asyncio
import aiohttp
from typing import List, Dict, Any, Optional
from dataclasses import dataclass
import logging
from urllib.parse import urlparse

from firecrawl_config import FirecrawlConfig


@dataclass
class ScrapedContent:
    """Represents scraped content from Firecrawl."""
    
    url: str
    markdown: Optional[str] = None
    html: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None
    title: Optional[str] = None
    description: Optional[str] = None
    status_code: Optional[int] = None
    error: Optional[str] = None


@dataclass
class CrawlJob:
    """Represents a crawl job from Firecrawl."""
    
    job_id: str
    status: str
    total: Optional[int] = None
    completed: Optional[int] = None
    data: Optional[List[ScrapedContent]] = None
    error: Optional[str] = None


class FirecrawlConnector:
    """Main connector class for Firecrawl integration with RAGFlow."""
    
    def __init__(self, config: FirecrawlConfig):
        """Initialize the Firecrawl connector."""
        self.config = config
        self.logger = logging.getLogger(__name__)
        self.session: Optional[aiohttp.ClientSession] = None
        self._rate_limit_semaphore = asyncio.Semaphore(config.max_concurrent_requests)
    
    async def __aenter__(self):
        """Async context manager entry."""
        await self._create_session()
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Async context manager exit."""
        await self._close_session()
    
    async def _create_session(self):
        """Create aiohttp session with proper headers."""
        headers = {
            "Authorization": f"Bearer {self.config.api_key}",
            "Content-Type": "application/json",
            "User-Agent": "RAGFlow-Firecrawl-Plugin/1.0.0"
        }
        
        timeout = aiohttp.ClientTimeout(total=self.config.timeout)
        self.session = aiohttp.ClientSession(
            headers=headers,
            timeout=timeout
        )
    
    async def _close_session(self):
        """Close aiohttp session."""
        if self.session:
            await self.session.close()
    
    async def _make_request(self, method: str, endpoint: str, **kwargs) -> Dict[str, Any]:
        """Make HTTP request with rate limiting and retry logic."""
        async with self._rate_limit_semaphore:
            # Rate limiting
            await asyncio.sleep(self.config.rate_limit_delay)
            
            url = f"{self.config.api_url}{endpoint}"
            
            for attempt in range(self.config.max_retries):
                try:
                    async with self.session.request(method, url, **kwargs) as response:
                        if response.status == 429:  # Rate limited
                            wait_time = 2 ** attempt
                            self.logger.warning(f"Rate limited, waiting {wait_time}s")
                            await asyncio.sleep(wait_time)
                            continue
                        
                        response.raise_for_status()
                        return await response.json()
                        
                except aiohttp.ClientError as e:
                    self.logger.error(f"Request failed (attempt {attempt + 1}): {e}")
                    if attempt == self.config.max_retries - 1:
                        raise
                    await asyncio.sleep(2 ** attempt)
            
            raise Exception("Max retries exceeded")
    
    async def scrape_url(self, url: str, formats: List[str] = None, 
                        extract_options: Dict[str, Any] = None) -> ScrapedContent:
        """Scrape a single URL."""
        if formats is None:
            formats = ["markdown", "html"]
        
        payload = {
            "url": url,
            "formats": formats
        }
        
        if extract_options:
            payload["extractOptions"] = extract_options
        
        try:
            response = await self._make_request("POST", "/v2/scrape", json=payload)
            
            if not response.get("success"):
                return ScrapedContent(url=url, error=response.get("error", "Unknown error"))
            
            data = response.get("data", {})
            metadata = data.get("metadata", {})
            
            return ScrapedContent(
                url=url,
                markdown=data.get("markdown"),
                html=data.get("html"),
                metadata=metadata,
                title=metadata.get("title"),
                description=metadata.get("description"),
                status_code=metadata.get("statusCode")
            )
            
        except Exception as e:
            self.logger.error(f"Failed to scrape {url}: {e}")
            return ScrapedContent(url=url, error=str(e))
    
    async def start_crawl(self, url: str, limit: int = 100, 
                         scrape_options: Dict[str, Any] = None) -> CrawlJob:
        """Start a crawl job."""
        if scrape_options is None:
            scrape_options = {"formats": ["markdown", "html"]}
        
        payload = {
            "url": url,
            "limit": limit,
            "scrapeOptions": scrape_options
        }
        
        try:
            response = await self._make_request("POST", "/v2/crawl", json=payload)
            
            if not response.get("success"):
                return CrawlJob(
                    job_id="",
                    status="failed",
                    error=response.get("error", "Unknown error")
                )
            
            job_id = response.get("id")
            return CrawlJob(job_id=job_id, status="started")
            
        except Exception as e:
            self.logger.error(f"Failed to start crawl for {url}: {e}")
            return CrawlJob(job_id="", status="failed", error=str(e))
    
    async def get_crawl_status(self, job_id: str) -> CrawlJob:
        """Get the status of a crawl job."""
        try:
            response = await self._make_request("GET", f"/v2/crawl/{job_id}")
            
            if not response.get("success"):
                return CrawlJob(
                    job_id=job_id,
                    status="failed",
                    error=response.get("error", "Unknown error")
                )
            
            status = response.get("status", "unknown")
            total = response.get("total")
            data = response.get("data", [])
            
            # Convert data to ScrapedContent objects
            scraped_content = []
            for item in data:
                metadata = item.get("metadata", {})
                scraped_content.append(ScrapedContent(
                    url=metadata.get("sourceURL", ""),
                    markdown=item.get("markdown"),
                    html=item.get("html"),
                    metadata=metadata,
                    title=metadata.get("title"),
                    description=metadata.get("description"),
                    status_code=metadata.get("statusCode")
                ))
            
            return CrawlJob(
                job_id=job_id,
                status=status,
                total=total,
                completed=len(scraped_content),
                data=scraped_content
            )
            
        except Exception as e:
            self.logger.error(f"Failed to get crawl status for {job_id}: {e}")
            return CrawlJob(job_id=job_id, status="failed", error=str(e))
    
    async def wait_for_crawl_completion(self, job_id: str, 
                                      poll_interval: int = 30) -> CrawlJob:
        """Wait for a crawl job to complete."""
        while True:
            job = await self.get_crawl_status(job_id)
            
            if job.status in ["completed", "failed", "cancelled"]:
                return job
            
            self.logger.info(f"Crawl {job_id} status: {job.status}")
            await asyncio.sleep(poll_interval)
    
    async def batch_scrape(self, urls: List[str], 
                          formats: List[str] = None) -> List[ScrapedContent]:
        """Scrape multiple URLs concurrently."""
        if formats is None:
            formats = ["markdown", "html"]
        
        tasks = [self.scrape_url(url, formats) for url in urls]
        results = await asyncio.gather(*tasks, return_exceptions=True)
        
        # Handle exceptions
        processed_results = []
        for i, result in enumerate(results):
            if isinstance(result, Exception):
                processed_results.append(ScrapedContent(
                    url=urls[i],
                    error=str(result)
                ))
            else:
                processed_results.append(result)
        
        return processed_results
    
    def validate_url(self, url: str) -> bool:
        """Validate if URL is properly formatted."""
        try:
            result = urlparse(url)
            return all([result.scheme, result.netloc])
        except Exception:
            return False
    
    def extract_domain(self, url: str) -> str:
        """Extract domain from URL."""
        try:
            return urlparse(url).netloc
        except Exception:
            return ""
