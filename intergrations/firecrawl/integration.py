"""
RAGFlow Integration Entry Point for Firecrawl

This file provides the main entry point for the Firecrawl integration with RAGFlow.
It follows RAGFlow's integration patterns and provides the necessary interfaces.
"""

from typing import Dict, Any
import logging

from ragflow_integration import RAGFlowFirecrawlIntegration, create_firecrawl_integration
from firecrawl_ui import FirecrawlUIBuilder

# Set up logging
logger = logging.getLogger(__name__)


class FirecrawlRAGFlowPlugin:
    """
    Main plugin class for Firecrawl integration with RAGFlow.
    This class provides the interface that RAGFlow expects from integrations.
    """
    
    def __init__(self):
        """Initialize the Firecrawl plugin."""
        self.name = "firecrawl"
        self.display_name = "Firecrawl Web Scraper"
        self.description = "Import web content using Firecrawl's powerful scraping capabilities"
        self.version = "1.0.0"
        self.author = "Firecrawl Team"
        self.category = "web"
        self.icon = "🌐"
        
        logger.info(f"Initialized {self.display_name} plugin v{self.version}")
    
    def get_plugin_info(self) -> Dict[str, Any]:
        """Get plugin information for RAGFlow."""
        return {
            "name": self.name,
            "display_name": self.display_name,
            "description": self.description,
            "version": self.version,
            "author": self.author,
            "category": self.category,
            "icon": self.icon,
            "supported_formats": ["markdown", "html", "links", "screenshot"],
            "supported_scrape_types": ["single", "crawl", "batch"]
        }
    
    def get_config_schema(self) -> Dict[str, Any]:
        """Get configuration schema for RAGFlow."""
        return FirecrawlUIBuilder.create_data_source_config()["config_schema"]
    
    def get_ui_schema(self) -> Dict[str, Any]:
        """Get UI schema for RAGFlow."""
        return FirecrawlUIBuilder.create_ui_schema()
    
    def validate_config(self, config: Dict[str, Any]) -> Dict[str, Any]:
        """Validate configuration and return any errors."""
        try:
            integration = create_firecrawl_integration(config)
            return integration.validate_config(config)
        except Exception as e:
            logger.error(f"Configuration validation error: {e}")
            return {"general": str(e)}
    
    def test_connection(self, config: Dict[str, Any]) -> Dict[str, Any]:
        """Test connection to Firecrawl API."""
        try:
            integration = create_firecrawl_integration(config)
            # Run the async test_connection method
            import asyncio
            return asyncio.run(integration.test_connection())
        except Exception as e:
            logger.error(f"Connection test error: {e}")
            return {
                "success": False,
                "error": str(e),
                "message": "Connection test failed"
            }
    
    def create_integration(self, config: Dict[str, Any]) -> RAGFlowFirecrawlIntegration:
        """Create and return a Firecrawl integration instance."""
        return create_firecrawl_integration(config)
    
    def get_help_text(self) -> Dict[str, str]:
        """Get help text for users."""
        return FirecrawlUIBuilder.create_help_text()
    
    def get_validation_rules(self) -> Dict[str, Any]:
        """Get validation rules for configuration."""
        return FirecrawlUIBuilder.create_validation_rules()


# RAGFlow integration entry points
def get_plugin() -> FirecrawlRAGFlowPlugin:
    """Get the plugin instance for RAGFlow."""
    return FirecrawlRAGFlowPlugin()


def get_integration(config: Dict[str, Any]) -> RAGFlowFirecrawlIntegration:
    """Get an integration instance with the given configuration."""
    return create_firecrawl_integration(config)


def get_config_schema() -> Dict[str, Any]:
    """Get the configuration schema."""
    return FirecrawlUIBuilder.create_data_source_config()["config_schema"]


def get_ui_schema() -> Dict[str, Any]:
    """Get the UI schema."""
    return FirecrawlUIBuilder.create_ui_schema()


def validate_config(config: Dict[str, Any]) -> Dict[str, Any]:
    """Validate configuration."""
    try:
        integration = create_firecrawl_integration(config)
        return integration.validate_config(config)
    except Exception as e:
        return {"general": str(e)}


def test_connection(config: Dict[str, Any]) -> Dict[str, Any]:
    """Test connection to Firecrawl API."""
    try:
        integration = create_firecrawl_integration(config)
        return integration.test_connection()
    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "message": "Connection test failed"
        }


# Export main functions and classes
__all__ = [
    "FirecrawlRAGFlowPlugin",
    "get_plugin",
    "get_integration",
    "get_config_schema",
    "get_ui_schema",
    "validate_config",
    "test_connection",
    "RAGFlowFirecrawlIntegration",
    "create_firecrawl_integration"
]
