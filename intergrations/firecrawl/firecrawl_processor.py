"""
Content processor for converting Firecrawl output to RAGFlow document format.
"""

import re
import hashlib
from typing import List, Dict, Any
from dataclasses import dataclass
import logging
from datetime import datetime

from firecrawl_connector import ScrapedContent


@dataclass
class RAGFlowDocument:
    """Represents a document in RAGFlow format."""
    
    id: str
    title: str
    content: str
    source_url: str
    metadata: Dict[str, Any]
    created_at: datetime
    updated_at: datetime
    content_type: str = "text"
    language: str = "en"
    chunk_size: int = 1000
    chunk_overlap: int = 200


class FirecrawlProcessor:
    """Processes Firecrawl content for RAGFlow integration."""
    
    def __init__(self):
        """Initialize the processor."""
        self.logger = logging.getLogger(__name__)
    
    def generate_document_id(self, url: str, content: str) -> str:
        """Generate a unique document ID."""
        # Create a hash based on URL and content
        content_hash = hashlib.md5(f"{url}:{content[:100]}".encode()).hexdigest()
        return f"firecrawl_{content_hash}"
    
    def clean_content(self, content: str) -> str:
        """Clean and normalize content."""
        if not content:
            return ""
        
        # Remove excessive whitespace
        content = re.sub(r'\s+', ' ', content)
        
        # Remove HTML tags if present
        content = re.sub(r'<[^>]+>', '', content)
        
        # Remove special characters that might cause issues
        content = re.sub(r'[^\w\s\.\,\!\?\;\:\-\(\)\[\]\"\']', '', content)
        
        return content.strip()
    
    def extract_title(self, content: ScrapedContent) -> str:
        """Extract title from scraped content."""
        if content.title:
            return content.title
        
        if content.metadata and content.metadata.get("title"):
            return content.metadata["title"]
        
        # Extract title from markdown if available
        if content.markdown:
            title_match = re.search(r'^#\s+(.+)$', content.markdown, re.MULTILINE)
            if title_match:
                return title_match.group(1).strip()
        
        # Fallback to URL
        return content.url.split('/')[-1] or content.url
    
    def extract_description(self, content: ScrapedContent) -> str:
        """Extract description from scraped content."""
        if content.description:
            return content.description
        
        if content.metadata and content.metadata.get("description"):
            return content.metadata["description"]
        
        # Extract first paragraph from markdown
        if content.markdown:
            # Remove headers and get first paragraph
            text = re.sub(r'^#+\s+.*$', '', content.markdown, flags=re.MULTILINE)
            paragraphs = [p.strip() for p in text.split('\n\n') if p.strip()]
            if paragraphs:
                return paragraphs[0][:200] + "..." if len(paragraphs[0]) > 200 else paragraphs[0]
        
        return ""
    
    def extract_language(self, content: ScrapedContent) -> str:
        """Extract language from content metadata."""
        if content.metadata and content.metadata.get("language"):
            return content.metadata["language"]
        
        # Simple language detection based on common words
        if content.markdown:
            text = content.markdown.lower()
            if any(word in text for word in ["the", "and", "or", "but", "in", "on", "at"]):
                return "en"
            elif any(word in text for word in ["le", "la", "les", "de", "du", "des"]):
                return "fr"
            elif any(word in text for word in ["der", "die", "das", "und", "oder"]):
                return "de"
            elif any(word in text for word in ["el", "la", "los", "las", "de", "del"]):
                return "es"
        
        return "en"  # Default to English
    
    def create_metadata(self, content: ScrapedContent) -> Dict[str, Any]:
        """Create comprehensive metadata for RAGFlow document."""
        metadata = {
            "source": "firecrawl",
            "url": content.url,
            "domain": self.extract_domain(content.url),
            "scraped_at": datetime.utcnow().isoformat(),
            "status_code": content.status_code,
            "content_length": len(content.markdown or ""),
            "has_html": bool(content.html),
            "has_markdown": bool(content.markdown)
        }
        
        # Add original metadata if available
        if content.metadata:
            metadata.update({
                "original_title": content.metadata.get("title"),
                "original_description": content.metadata.get("description"),
                "original_language": content.metadata.get("language"),
                "original_keywords": content.metadata.get("keywords"),
                "original_robots": content.metadata.get("robots"),
                "og_title": content.metadata.get("ogTitle"),
                "og_description": content.metadata.get("ogDescription"),
                "og_image": content.metadata.get("ogImage"),
                "og_url": content.metadata.get("ogUrl")
            })
        
        return metadata
    
    def extract_domain(self, url: str) -> str:
        """Extract domain from URL."""
        try:
            from urllib.parse import urlparse
            return urlparse(url).netloc
        except Exception:
            return ""
    
    def process_content(self, content: ScrapedContent) -> RAGFlowDocument:
        """Process scraped content into RAGFlow document format."""
        if content.error:
            raise ValueError(f"Content has error: {content.error}")
        
        # Determine primary content
        primary_content = content.markdown or content.html or ""
        if not primary_content:
            raise ValueError("No content available to process")
        
        # Clean content
        cleaned_content = self.clean_content(primary_content)
        
        # Extract metadata
        title = self.extract_title(content)
        language = self.extract_language(content)
        metadata = self.create_metadata(content)
        
        # Generate document ID
        doc_id = self.generate_document_id(content.url, cleaned_content)
        
        # Create RAGFlow document
        document = RAGFlowDocument(
            id=doc_id,
            title=title,
            content=cleaned_content,
            source_url=content.url,
            metadata=metadata,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
            content_type="text",
            language=language
        )
        
        return document
    
    def process_batch(self, contents: List[ScrapedContent]) -> List[RAGFlowDocument]:
        """Process multiple scraped contents into RAGFlow documents."""
        documents = []
        
        for content in contents:
            try:
                document = self.process_content(content)
                documents.append(document)
            except Exception as e:
                self.logger.error(f"Failed to process content from {content.url}: {e}")
                continue
        
        return documents
    
    def chunk_content(self, document: RAGFlowDocument, 
                     chunk_size: int = 1000, 
                     chunk_overlap: int = 200) -> List[Dict[str, Any]]:
        """Chunk document content for RAG processing."""
        content = document.content
        chunks = []
        
        if len(content) <= chunk_size:
            return [{
                "id": f"{document.id}_chunk_0",
                "content": content,
                "metadata": {
                    **document.metadata,
                    "chunk_index": 0,
                    "total_chunks": 1
                }
            }]
        
        # Split content into chunks
        start = 0
        chunk_index = 0
        
        while start < len(content):
            end = start + chunk_size
            
            # Try to break at sentence boundary
            if end < len(content):
                # Look for sentence endings
                sentence_end = content.rfind('.', start, end)
                if sentence_end > start + chunk_size // 2:
                    end = sentence_end + 1
            
            chunk_content = content[start:end].strip()
            
            if chunk_content:
                chunks.append({
                    "id": f"{document.id}_chunk_{chunk_index}",
                    "content": chunk_content,
                    "metadata": {
                        **document.metadata,
                        "chunk_index": chunk_index,
                        "total_chunks": len(chunks) + 1,  # Will be updated
                        "chunk_start": start,
                        "chunk_end": end
                    }
                })
                chunk_index += 1
            
            # Move start position with overlap
            start = end - chunk_overlap
            if start >= len(content):
                break
        
        # Update total chunks count
        for chunk in chunks:
            chunk["metadata"]["total_chunks"] = len(chunks)
        
        return chunks
    
    def validate_document(self, document: RAGFlowDocument) -> bool:
        """Validate RAGFlow document."""
        if not document.id:
            return False
        
        if not document.title:
            return False
        
        if not document.content:
            return False
        
        if not document.source_url:
            return False
        
        return True
