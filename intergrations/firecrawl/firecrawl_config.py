"""
Configuration management for Firecrawl integration with RAGFlow.
"""

import os
from typing import Dict, Any
from dataclasses import dataclass
import json


@dataclass
class FirecrawlConfig:
    """Configuration class for Firecrawl integration."""
    
    api_key: str
    api_url: str = "https://api.firecrawl.dev"
    max_retries: int = 3
    timeout: int = 30
    rate_limit_delay: float = 1.0
    max_concurrent_requests: int = 5
    
    def __post_init__(self):
        """Validate configuration after initialization."""
        if not self.api_key:
            raise ValueError("Firecrawl API key is required")
        
        if not self.api_key.startswith("fc-"):
            raise ValueError("Invalid Firecrawl API key format. Must start with 'fc-'")
        
        if self.max_retries < 1 or self.max_retries > 10:
            raise ValueError("Max retries must be between 1 and 10")
        
        if self.timeout < 5 or self.timeout > 300:
            raise ValueError("Timeout must be between 5 and 300 seconds")
        
        if self.rate_limit_delay < 0.1 or self.rate_limit_delay > 10.0:
            raise ValueError("Rate limit delay must be between 0.1 and 10.0 seconds")
    
    @classmethod
    def from_env(cls) -> "FirecrawlConfig":
        """Create configuration from environment variables."""
        api_key = os.getenv("FIRECRAWL_API_KEY")
        if not api_key:
            raise ValueError("FIRECRAWL_API_KEY environment variable not set")
        
        return cls(
            api_key=api_key,
            api_url=os.getenv("FIRECRAWL_API_URL", "https://api.firecrawl.dev"),
            max_retries=int(os.getenv("FIRECRAWL_MAX_RETRIES", "3")),
            timeout=int(os.getenv("FIRECRAWL_TIMEOUT", "30")),
            rate_limit_delay=float(os.getenv("FIRECRAWL_RATE_LIMIT_DELAY", "1.0")),
            max_concurrent_requests=int(os.getenv("FIRECRAWL_MAX_CONCURRENT", "5"))
        )
    
    @classmethod
    def from_dict(cls, config_dict: Dict[str, Any]) -> "FirecrawlConfig":
        """Create configuration from dictionary."""
        return cls(**config_dict)
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert configuration to dictionary."""
        return {
            "api_key": self.api_key,
            "api_url": self.api_url,
            "max_retries": self.max_retries,
            "timeout": self.timeout,
            "rate_limit_delay": self.rate_limit_delay,
            "max_concurrent_requests": self.max_concurrent_requests
        }
    
    def to_json(self) -> str:
        """Convert configuration to JSON string."""
        return json.dumps(self.to_dict(), indent=2)
    
    @classmethod
    def from_json(cls, json_str: str) -> "FirecrawlConfig":
        """Create configuration from JSON string."""
        config_dict = json.loads(json_str)
        return cls.from_dict(config_dict)
