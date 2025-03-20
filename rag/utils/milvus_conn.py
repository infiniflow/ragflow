#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import json
import logging
import os
import time
from typing import Dict, List, Optional, Tuple, Union, Any

import numpy as np

from pymilvus import connections, Collection, utility
from pymilvus.orm import FieldSchema, CollectionSchema, DataType

from rag.utils.doc_store_conn import (
    DocStoreConnection,
    SparseVector,
    MatchTextExpr,
    MatchDenseExpr,
    MatchSparseExpr,
    MatchTensorExpr,
    FusionExpr
)

logger = logging.getLogger("ragflow")


class MilvusConnection(DocStoreConnection):
    """Milvus connector for RAGFlow."""

    def __init__(self) -> None:
        """Initialize Milvus connection."""
        super().__init__()
        
        self.host = os.environ.get("MILVUS_HOST", "localhost")
        self.port = os.environ.get("MILVUS_PORT", "19530")
        self.user = os.environ.get("MILVUS_USER", "")
        self.password = os.environ.get("MILVUS_PASSWORD", "")
        self.db_name = os.environ.get("MILVUS_DB", "default")
        
        try:
            # Connect to Milvus server
            connections.connect(
                alias="default",
                host=self.host,
                port=self.port,
                user=self.user,
                password=self.password,
                db_name=self.db_name
            )
            logger.info(f"Connected to Milvus at {self.host}:{self.port}")
            
            # Make sure the database exists
            self._migrate_db()
            
        except Exception as e:
            logger.error(f"Failed to connect to Milvus: {e}")
            raise

    def _migrate_db(self) -> None:
        """Create necessary database if it doesn't exist."""
        try:
            # Load collection mapping
            mapping_file = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), 
                                       "conf", "milvus_mapping.json")
            if os.path.exists(mapping_file):
                with open(mapping_file, "r") as f:
                    self.mappings = json.load(f)
            else:
                logger.warning(f"Milvus mapping file not found: {mapping_file}")
                self.mappings = {}
        except Exception as e:
            logger.error(f"Failed to load Milvus mapping: {e}")
            self.mappings = {}

    def dbType(self) -> str:
        """Return database type.
        
        Returns:
            str: The type of database.
        """
        return "milvus"

    def health(self) -> bool:
        """Check if the database connection is healthy.
        
        Returns:
            bool: True if the database is healthy, False otherwise.
        """
        try:
            # A simple check to see if the connection is alive
            utility.has_collection("_health_check_")
            return True
        except Exception as e:
            logger.error(f"Milvus health check failed: {e}")
            return False
            
    def createIdx(self, kb_id: int) -> bool:
        """Create an index for the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            
        Returns:
            bool: True if the index was created successfully, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            # Define the schema for the collection
            fields = [
                FieldSchema(name="id", dtype=DataType.INT64, is_primary=True),
                FieldSchema(name="chunk_id", dtype=DataType.VARCHAR, max_length=65535),
                FieldSchema(name="doc_id", dtype=DataType.VARCHAR, max_length=65535),
                FieldSchema(name="doc_type", dtype=DataType.VARCHAR, max_length=255),
                FieldSchema(name="doc_name", dtype=DataType.VARCHAR, max_length=255),
                FieldSchema(name="doc_text", dtype=DataType.VARCHAR, max_length=65535),
                FieldSchema(name="vector", dtype=DataType.FLOAT_VECTOR, dim=1536),  # Adjust dimension as needed
                FieldSchema(name="kb_id", dtype=DataType.INT64),
                FieldSchema(name="create_time", dtype=DataType.INT64),
                FieldSchema(name="update_time", dtype=DataType.INT64),
                # Add additional fields as needed
            ]
            
            schema = CollectionSchema(fields)
            
            # Create the collection if it doesn't exist
            if not utility.has_collection(collection_name):
                collection = Collection(name=collection_name, schema=schema)
                
                # Create index on vector field for similarity search
                index_params = {
                    "index_type": "HNSW",
                    "metric_type": "IP",  # Inner Product, can also use L2 for Euclidean distance
                    "params": {"M": 16, "efConstruction": 128}
                }
                collection.create_index(field_name="vector", index_params=index_params)
                logger.info(f"Created index for collection {collection_name}")
                
                return True
            else:
                logger.info(f"Collection {collection_name} already exists")
                return True
                
        except Exception as e:
            logger.error(f"Failed to create index for kb_id {kb_id}: {e}")
            return False

    def deleteIdx(self, kb_id: int) -> bool:
        """Delete an index for the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            
        Returns:
            bool: True if the index was deleted successfully, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            if utility.has_collection(collection_name):
                utility.drop_collection(collection_name)
                logger.info(f"Deleted collection {collection_name}")
                return True
            else:
                logger.info(f"Collection {collection_name} does not exist")
                return True
        except Exception as e:
            logger.error(f"Failed to delete index for kb_id {kb_id}: {e}")
            return False

    def indexExist(self, kb_id: int) -> bool:
        """Check if an index exists for the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            
        Returns:
            bool: True if the index exists, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        return utility.has_collection(collection_name)

    def search(self, kb_id: int, text: str = None, limit: int = 10, 
              offset: int = 0, sortField: str = None, sortOrder: str = None,
              conditions: Dict = None, matchText: MatchTextExpr = None,
              matchDense: MatchDenseExpr = None, matchSparse: MatchSparseExpr = None,
              matchTensor: MatchTensorExpr = None, fusion: FusionExpr = None,
              chunk_id: str = None) -> Dict:
        """Search for documents in the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            text (str, optional): Text to search for.
            limit (int, optional): Maximum number of results to return.
            offset (int, optional): Offset for pagination.
            sortField (str, optional): Field to sort by.
            sortOrder (str, optional): Sort order (asc or desc).
            conditions (Dict, optional): Additional search conditions.
            matchText (MatchTextExpr, optional): Text matching expression.
            matchDense (MatchDenseExpr, optional): Dense vector matching expression.
            matchSparse (MatchSparseExpr, optional): Sparse vector matching expression.
            matchTensor (MatchTensorExpr, optional): Tensor matching expression.
            fusion (FusionExpr, optional): Fusion expression.
            chunk_id (str, optional): Specific chunk ID to retrieve.
            
        Returns:
            Dict: Search results.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            if not utility.has_collection(collection_name):
                logger.warning(f"Collection {collection_name} does not exist")
                return {"total": 0, "hits": []}
                
            collection = Collection(name=collection_name)
            collection.load()
            
            # Build search query parameters
            search_params = {
                "limit": limit,
                "offset": offset
            }
            
            expr = None
            if conditions:
                expr_parts = []
                for key, value in conditions.items():
                    if isinstance(value, str):
                        expr_parts.append(f"{key} == '{value}'")
                    else:
                        expr_parts.append(f"{key} == {value}")
                if expr_parts:
                    expr = " && ".join(expr_parts)
            
            # Add specific chunk_id if provided
            if chunk_id:
                chunk_expr = f"chunk_id == '{chunk_id}'"
                expr = chunk_expr if not expr else f"{expr} && {chunk_expr}"
            
            if expr:
                search_params["expr"] = expr
            
            # Perform vector search if matchDense is provided
            if matchDense:
                vector = matchDense.vec
                search_params["data"] = [vector]
                search_params["anns_field"] = "vector"
                search_params["param"] = {
                    "metric_type": "IP",  # Inner Product similarity
                    "params": {"ef": 64}
                }
                
                results = collection.search(**search_params)
                
                hits = []
                for i, result in enumerate(results):
                    for hit in result:
                        # Get the entity by ID
                        entity = collection.query(expr=f"id == {hit.id}")
                        if entity:
                            doc = entity[0]
                            hits.append({
                                "id": doc.get("id"),
                                "chunk_id": doc.get("chunk_id"),
                                "doc_id": doc.get("doc_id"),
                                "doc_type": doc.get("doc_type"),
                                "doc_name": doc.get("doc_name"),
                                "doc_text": doc.get("doc_text"),
                                "kb_id": doc.get("kb_id"),
                                "score": hit.score,
                                # Add other fields as needed
                            })
                
                return {
                    "total": len(hits),
                    "hits": hits
                }
            
            # If no vector search, perform regular query
            elif expr:
                query_results = collection.query(expr=expr, output_fields=["*"], limit=limit, offset=offset)
                
                hits = []
                for doc in query_results:
                    hits.append({
                        "id": doc.get("id"),
                        "chunk_id": doc.get("chunk_id"),
                        "doc_id": doc.get("doc_id"),
                        "doc_type": doc.get("doc_type"),
                        "doc_name": doc.get("doc_name"),
                        "doc_text": doc.get("doc_text"),
                        "kb_id": doc.get("kb_id"),
                        "score": 1.0,  # Default score for non-vector queries
                        # Add other fields as needed
                    })
                
                return {
                    "total": len(hits),
                    "hits": hits
                }
            
            else:
                # If no conditions provided, return empty results
                return {"total": 0, "hits": []}
                
        except Exception as e:
            logger.error(f"Search failed for kb_id {kb_id}: {e}")
            return {"total": 0, "hits": []}
        finally:
            # Release collection from memory
            if 'collection' in locals():
                collection.release()

    def get(self, kb_id: int, chunk_id: str) -> Dict:
        """Get a document by its chunk ID.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            chunk_id (str): The chunk ID of the document.
            
        Returns:
            Dict: The document if found, None otherwise.
        """
        search_result = self.search(kb_id=kb_id, chunk_id=chunk_id, limit=1)
        hits = search_result.get("hits", [])
        return hits[0] if hits else None

    def insert(self, kb_id: int, docs: List[Dict]) -> bool:
        """Insert documents into the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            docs (List[Dict]): List of documents to insert.
            
        Returns:
            bool: True if the documents were inserted successfully, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            # Create collection if it doesn't exist
            if not self.indexExist(kb_id):
                self.createIdx(kb_id)
                
            collection = Collection(name=collection_name)
            
            # Prepare data for insertion
            current_timestamp = int(time.time())
            
            data_to_insert = {
                "id": [],
                "chunk_id": [],
                "doc_id": [],
                "doc_type": [],
                "doc_name": [],
                "doc_text": [],
                "vector": [],
                "kb_id": [],
                "create_time": [],
                "update_time": []
            }
            
            for i, doc in enumerate(docs):
                # Generate a unique ID if not provided
                doc_id = i + 1  # Simple ID generation for example purposes
                
                data_to_insert["id"].append(doc_id)
                data_to_insert["chunk_id"].append(doc.get("chunk_id", ""))
                data_to_insert["doc_id"].append(doc.get("doc_id", ""))
                data_to_insert["doc_type"].append(doc.get("doc_type", ""))
                data_to_insert["doc_name"].append(doc.get("doc_name", ""))
                data_to_insert["doc_text"].append(doc.get("doc_text", ""))
                data_to_insert["vector"].append(doc.get("vector", [0.0] * 1536))  # Default empty vector
                data_to_insert["kb_id"].append(kb_id)
                data_to_insert["create_time"].append(current_timestamp)
                data_to_insert["update_time"].append(current_timestamp)
            
            # Insert data
            collection.insert(data_to_insert)
            logger.info(f"Inserted {len(docs)} documents into collection {collection_name}")
            
            return True
        
        except Exception as e:
            logger.error(f"Failed to insert documents for kb_id {kb_id}: {e}")
            return False

    def update(self, kb_id: int, docs: List[Dict]) -> bool:
        """Update documents in the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            docs (List[Dict]): List of documents to update.
            
        Returns:
            bool: True if the documents were updated successfully, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            if not utility.has_collection(collection_name):
                logger.warning(f"Collection {collection_name} does not exist")
                return False
                
            collection = Collection(name=collection_name)
            
            current_timestamp = int(time.time())
            
            for doc in docs:
                chunk_id = doc.get("chunk_id")
                if not chunk_id:
                    logger.warning("Update operation requires chunk_id")
                    continue
                
                # Prepare update data
                update_data = {k: v for k, v in doc.items() if k != "chunk_id"}
                update_data["update_time"] = current_timestamp
                
                # Execute update
                collection.upsert(
                    {"chunk_id": chunk_id},
                    update_data
                )
            
            logger.info(f"Updated {len(docs)} documents in collection {collection_name}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to update documents for kb_id {kb_id}: {e}")
            return False

    def delete(self, kb_id: int, conditions: Dict = None, chunk_ids: List[str] = None) -> bool:
        """Delete documents from the specified knowledge base.
        
        Args:
            kb_id (int): The ID of the knowledge base.
            conditions (Dict, optional): Conditions for deletion.
            chunk_ids (List[str], optional): List of chunk IDs to delete.
            
        Returns:
            bool: True if the documents were deleted successfully, False otherwise.
        """
        collection_name = f"kb_{kb_id}"
        
        try:
            if not utility.has_collection(collection_name):
                logger.warning(f"Collection {collection_name} does not exist")
                return False
                
            collection = Collection(name=collection_name)
            
            expr = None
            
            # Build deletion expression from conditions
            if conditions:
                expr_parts = []
                for key, value in conditions.items():
                    if isinstance(value, str):
                        expr_parts.append(f"{key} == '{value}'")
                    else:
                        expr_parts.append(f"{key} == {value}")
                if expr_parts:
                    expr = " && ".join(expr_parts)
            
            # Build deletion expression from chunk_ids
            if chunk_ids:
                chunk_expr_parts = [f"chunk_id == '{chunk_id}'" for chunk_id in chunk_ids]
                chunk_expr = " || ".join(chunk_expr_parts)
                expr = chunk_expr if not expr else f"({expr}) && ({chunk_expr})"
            
            if expr:
                collection.delete(expr)
                logger.info(f"Deleted documents from collection {collection_name} with expression: {expr}")
                return True
            else:
                logger.warning("No deletion criteria provided")
                return False
                
        except Exception as e:
            logger.error(f"Failed to delete documents for kb_id {kb_id}: {e}")
            return False

    def getTotal(self, results: Dict) -> int:
        """Get the total number of results.
        
        Args:
            results (Dict): Search results.
            
        Returns:
            int: Total number of results.
        """
        return results.get("total", 0)

    def getChunkIds(self, results: Dict) -> List[str]:
        """Get the chunk IDs from search results.
        
        Args:
            results (Dict): Search results.
            
        Returns:
            List[str]: List of chunk IDs.
        """
        return [hit.get("chunk_id", "") for hit in results.get("hits", [])]

    def getFields(self, results: Dict, field: str) -> List[str]:
        """Get field values from search results.
        
        Args:
            results (Dict): Search results.
            field (str): Field name.
            
        Returns:
            List[str]: List of field values.
        """
        return [hit.get(field, "") for hit in results.get("hits", [])] 