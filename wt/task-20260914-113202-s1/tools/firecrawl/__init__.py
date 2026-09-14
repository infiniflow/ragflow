"""
Firecrawl Plugin for RAGFlow

This plugin integrates Firecrawl's web scraping capabilities into RAGFlow,
allowing users to import web content directly into their RAG workflows.
"""

__version__ = "1.0.0"
__author__ = "Firecrawl Team"
__description__ = "Firecrawl integration for RAGFlow - Web content scraping and import"

from firecrawl_connector import FirecrawlConnector
from firecrawl_config import FirecrawlConfig

__all__ = ["FirecrawlConnector", "FirecrawlConfig"]
