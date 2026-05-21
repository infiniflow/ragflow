"""
Elasticsearch 8+ Client for RAGFlow data migration.
"""

import logging
from typing import Any, Iterator

from elasticsearch import Elasticsearch

logger = logging.getLogger(__name__)


class ESClient:
    """Elasticsearch client wrapper for RAGFlow migration operations."""

    def __init__(
        self,
        host: str = "localhost",
        port: int = 9200,
        username: str | None = None,
        password: str | None = None,
        api_key: str | None = None,
        use_ssl: bool = False,
        verify_certs: bool = True,
    ):
        """
        Initialize ES client.

        Args:
            host: ES host address
            port: ES port
            username: Basic auth username
            password: Basic auth password
            api_key: API key for authentication
            use_ssl: Whether to use SSL
            verify_certs: Whether to verify SSL certificates
        """
        self.host = host
        self.port = port

        # Build connection URL
        scheme = "https" if use_ssl else "http"
        url = f"{scheme}://{host}:{port}"

        # Build connection arguments
        conn_args: dict[str, Any] = {
            "hosts": [url],
            "verify_certs": verify_certs,
        }

        if api_key:
            conn_args["api_key"] = api_key
        elif username and password:
            conn_args["basic_auth"] = (username, password)

        self.client = Elasticsearch(**conn_args)
        logger.info(f"Connected to Elasticsearch at {url}")

    def health_check(self) -> dict[str, Any]:
        """Check cluster health."""
        return self.client.cluster.health().body

    def get_cluster_info(self) -> dict[str, Any]:
        """Get cluster information."""
        return self.client.info().body

    def list_indices(self, pattern: str = "*") -> list[str]:
        """List all indices matching pattern."""
        response = self.client.indices.get(index=pattern)
        return list(response.keys())

    def list_ragflow_indices(self) -> list[str]:
        """
        List all RAGFlow-related indices.
        
        Returns indices matching patterns:
        - ragflow_* (document chunks)
        - ragflow_doc_meta_* (document metadata)
        
        Returns:
            List of RAGFlow index names
        """
        try:
            # Get all ragflow_* indices
            ragflow_indices = self.list_indices("ragflow_*")
            return sorted(ragflow_indices)
        except Exception:
            # If no indices match, return empty list
            return []

    def get_index_mapping(self, index_name: str) -> dict[str, Any]:
        """
        Get index mapping.

        Args:
            index_name: Name of the index

        Returns:
            Index mapping dictionary
        """
        response = self.client.indices.get_mapping(index=index_name)
        return response[index_name]["mappings"]

    def get_index_settings(self, index_name: str) -> dict[str, Any]:
        """Get index settings."""
        response = self.client.indices.get_settings(index=index_name)
        return response[index_name]["settings"]

    def count_documents(self, index_name: str) -> int:
        """Count documents in an index."""
        response = self.client.count(index=index_name)
        return response["count"]

    def count_documents_with_filter(
        self, 
        index_name: str, 
        filters: dict[str, Any]
    ) -> int:
        """
        Count documents with filter conditions.
        
        Args:
            index_name: Index name
            filters: Filter conditions (e.g., {"kb_id": "xxx"})
            
        Returns:
            Document count
        """
        # Build bool query with filters
        must_clauses = []
        for field, value in filters.items():
            if isinstance(value, list):
                must_clauses.append({"terms": {field: value}})
            else:
                must_clauses.append({"term": {field: value}})
        
        query = {
            "bool": {
                "must": must_clauses
            }
        } if must_clauses else {"match_all": {}}
        
        response = self.client.count(index=index_name, query=query)
        return response["count"]

    def aggregate_field(
        self, 
        index_name: str, 
        field: str,
        size: int = 10000,
    ) -> dict[str, Any]:
        """
        Aggregate field values (like getting all unique kb_ids).
        
        Args:
            index_name: Index name
            field: Field to aggregate
            size: Max number of buckets
            
        Returns:
            Aggregation result with buckets
        """
        response = self.client.search(
            index=index_name,
            size=0,
            aggs={
                "field_values": {
                    "terms": {
                        "field": field,
                        "size": size,
                    }
                }
            }
        )
        return response["aggregations"]["field_values"]

    def scroll_documents(
        self,
        index_name: str,
        batch_size: int = 1000,
        query: dict[str, Any] | None = None,
        sort_field: str = "_doc",
    ) -> Iterator[list[dict[str, Any]]]:
        """
        Scroll through all documents in an index using search_after (ES 8+).

        This is the recommended approach for ES 8+ instead of scroll API.
        Uses search_after for efficient deep pagination.

        Args:
            index_name: Name of the index
            batch_size: Number of documents per batch
            query: Optional query filter
            sort_field: Field to sort by (default: _doc for efficiency)

        Yields:
            Batches of documents
        """
        search_body: dict[str, Any] = {
            "size": batch_size,
            "sort": [{sort_field: "asc"}, {"_id": "asc"}],
        }

        if query:
            search_body["query"] = query
        else:
            search_body["query"] = {"match_all": {}}

        # Initial search
        response = self.client.search(index=index_name, body=search_body)
        hits = response["hits"]["hits"]

        while hits:
            # Extract documents with _id
            documents = []
            for hit in hits:
                doc = hit["_source"].copy()
                doc["_id"] = hit["_id"]
                if "_score" in hit:
                    doc["_score"] = hit["_score"]
                documents.append(doc)

            yield documents

            # Check if there are more results
            if len(hits) < batch_size:
                break

            # Get search_after value from last hit
            search_after = hits[-1]["sort"]
            search_body["search_after"] = search_after

            response = self.client.search(index=index_name, body=search_body)
            hits = response["hits"]["hits"]

    def get_document(self, index_name: str, doc_id: str) -> dict[str, Any] | None:
        """Get a single document by ID."""
        try:
            response = self.client.get(index=index_name, id=doc_id)
            doc = response["_source"].copy()
            doc["_id"] = response["_id"]
            return doc
        except Exception:
            return None

    def get_sample_documents(
        self, 
        index_name: str, 
        size: int = 10,
        query: dict[str, Any] | None = None,
    ) -> list[dict[str, Any]]:
        """
        Get sample documents from an index.
        
        Args:
            index_name: Index name
            size: Number of samples
            query: Optional query filter
        """
        search_body = {
            "query": query if query else {"match_all": {}},
            "size": size
        }
        
        response = self.client.search(index=index_name, body=search_body)
        documents = []
        for hit in response["hits"]["hits"]:
            doc = hit["_source"].copy()
            doc["_id"] = hit["_id"]
            documents.append(doc)
        return documents

    def get_document_ids(
        self, 
        index_name: str, 
        size: int = 1000,
        query: dict[str, Any] | None = None,
    ) -> list[str]:
        """Get list of document IDs."""
        search_body = {
            "query": query if query else {"match_all": {}},
            "size": size,
            "_source": False,
        }
        
        response = self.client.search(index=index_name, body=search_body)
        return [hit["_id"] for hit in response["hits"]["hits"]]

    def close(self):
        """Close the ES client connection."""
        self.client.close()
        logger.info("Elasticsearch connection closed")
