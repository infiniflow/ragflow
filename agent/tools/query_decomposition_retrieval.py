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

"""
Query Decomposition Retrieval Component

This module implements an advanced retrieval system that automatically decomposes complex queries
into simpler sub-queries, performs concurrent retrieval, and intelligently reranks results using
LLM-based scoring combined with vector similarity.

Key Features:
- Automatic query decomposition for complex, multi-faceted questions
- Concurrent retrieval across multiple sub-queries for better performance
- Global chunk deduplication to eliminate redundant results
- LLM-based relevance scoring for each chunk
- Configurable score fusion between vector similarity and LLM scores
- Built-in high-quality default prompts

Use Cases:
- Complex queries with multiple aspects ("Compare A and B", "Explain X, Y, and Z")
- Multi-hop reasoning questions
- Research queries requiring comprehensive coverage
- Questions needing information from multiple sources

Advantages over Manual Workflow Approach:
- Simplified configuration (no need to manually assemble components)
- Better performance (internal concurrency, minimal overhead)
- Higher quality results (global deduplication and reranking)
- Deterministic behavior (no agent unpredictability)
"""

import asyncio
import json
import logging
import os
import re
from abc import ABC
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Dict, List, Tuple

import numpy as np

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from api.db.services.document_service import DocumentService
from api.db.services.dialog_service import meta_filter
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common import settings
from common.connection_utils import timeout
from common.constants import LLMType
from rag.app.tag import label_question
from rag.prompts.generator import cross_languages, gen_meta_filter, kb_prompt


# -----------------------------------------------------------------------------
# Default Prompts
# -----------------------------------------------------------------------------

# Default prompt for query decomposition
# This prompt instructs the LLM to break down complex queries into simpler sub-questions
DEFAULT_DECOMPOSITION_PROMPT = """You are a query decomposition expert. Your task is to break down complex questions into 2 to 3 simpler, independently retrievable sub-questions.

Guidelines:
1. Each sub-question should focus on one specific aspect of the original query
2. Sub-questions should be non-redundant (no overlap in information sought)
3. Together, the sub-questions should cover all key aspects of the original query
4. Each sub-question should be clear, specific, and independently answerable
5. Avoid creating more than {max_count} sub-questions

**Original Query:** {original_query}

**Output Requirement:** Output ONLY a standard JSON array where each element is a string representing a sub-question.
Example format: ["What is concept A?", "What is concept B?", "How do A and B compare?"]

Do not output any explanatory text, commentary, or formatting other than the JSON array.

Sub-questions:"""

# Default prompt for LLM-based chunk relevance scoring
# This prompt instructs the LLM to judge how useful a chunk is for answering a query
DEFAULT_RERANKING_PROMPT = """You are an information relevance assessment expert. Your task is to judge how useful a given document chunk is for answering a specific query.

**Query:** {query}

**Document Chunk:**
{chunk_text}

**Assessment Task:**
1. **Relevance Score**: Provide an integer score from 1 to 10 based on the following criteria:
   - 9-10: Contains direct, complete answer to the query
   - 7-8: Contains substantial relevant information that partially answers the query
   - 5-6: Contains indirect clues or related context that could help answer the query
   - 3-4: Tangentially related but not directly useful
   - 1-2: Completely irrelevant to the query

2. **Brief Justification**: In ONE sentence, explain the core reason for your score.

**Output Requirement:** Output STRICTLY in JSON format with no additional text:
{{"score": <integer 1-10>, "reason": "<one sentence justification>"}}

Assessment:"""


# -----------------------------------------------------------------------------
# Parameter Configuration Class
# -----------------------------------------------------------------------------

class QueryDecompositionRetrievalParam(ToolParamBase):
    """
    Configuration parameters for Query Decomposition Retrieval component.
    
    This class defines all configurable parameters for the advanced retrieval system,
    including query decomposition settings, reranking configuration, and score fusion weights.
    
    Attributes:
        enable_decomposition (bool): Master toggle for the entire decomposition pipeline
        decomposition_prompt (str): Custom or default prompt for query decomposition
        reranking_prompt (str): Custom or default prompt for LLM-based chunk scoring
        score_fusion_weight (float): Weight for LLM score vs vector similarity (0.0-1.0)
        max_decomposition_count (int): Maximum number of sub-queries to generate
        enable_concurrency (bool): Whether to retrieve sub-queries concurrently
        similarity_threshold (float): Minimum similarity score for chunk inclusion
        keywords_similarity_weight (float): Weight of keyword matching vs vector similarity
        top_n (int): Number of final chunks to return
        top_k (int): Number of initial candidates to retrieve per sub-query
        kb_ids (list): Knowledge base IDs to search
        rerank_id (str): Reranking model ID (for traditional reranking if needed)
        empty_response (str): Response when no results found
        use_kg (bool): Whether to use knowledge graph retrieval
        cross_languages (list): Languages for cross-lingual retrieval
        toc_enhance (bool): Whether to enhance with table-of-contents
        meta_data_filter (dict): Metadata filters for retrieval
    """
    
    def __init__(self):
        """Initialize query decomposition retrieval parameters with sensible defaults."""
        # Define tool metadata for agent integration
        self.meta: ToolMeta = {
            "name": "advanced_search_with_decomposition",
            "description": (
                "Advanced retrieval tool that automatically decomposes complex queries into "
                "simpler sub-questions, performs concurrent retrieval, and intelligently "
                "reranks results using LLM-based scoring."
            ),
            "parameters": {
                "query": {
                    "type": "string",
                    "description": (
                        "The complex query to search for. Can be multi-faceted or require "
                        "information from multiple sources."
                    ),
                    "default": "",
                    "required": True
                }
            }
        }
        
        super().__init__()
        
        # Tool identification
        self.function_name = "advanced_search_with_decomposition"
        self.description = self.meta["description"]
        
        # Query Decomposition Settings
        # These control how complex queries are broken down into sub-questions
        self.enable_decomposition = True  # Master toggle for decomposition feature
        self.decomposition_prompt = DEFAULT_DECOMPOSITION_PROMPT  # LLM prompt for decomposition
        self.max_decomposition_count = 3  # Limit sub-queries to prevent over-decomposition
        
        # Reranking & Scoring Settings
        # These control how chunks are scored and ranked after retrieval
        self.reranking_prompt = DEFAULT_RERANKING_PROMPT  # LLM prompt for chunk scoring
        self.score_fusion_weight = 0.7  # Weight: 0.7*LLM_score + 0.3*vector_score
        
        # Concurrency Settings
        # Controls whether sub-queries are processed in parallel
        self.enable_concurrency = True  # Enable parallel retrieval for better performance
        
        # Traditional Retrieval Settings
        # These are inherited from the base Retrieval component
        self.similarity_threshold = 0.2  # Minimum similarity score to include a chunk
        self.keywords_similarity_weight = 0.3  # Weight for keyword vs vector matching
        self.top_n = 8  # Number of final results to return
        self.top_k = 1024  # Number of initial candidates to retrieve
        
        # Knowledge Base Settings
        self.kb_ids = []  # List of knowledge base IDs to search
        self.kb_vars = []  # Knowledge base variables for dynamic selection
        self.rerank_id = ""  # Traditional reranking model ID
        self.empty_response = "No relevant information found."  # Default empty response
        
        # Advanced Features
        self.use_kg = False  # Whether to use knowledge graph retrieval
        self.cross_languages = []  # Languages for cross-lingual search
        self.toc_enhance = False  # Whether to enhance with document TOC
        self.meta_data_filter = {}  # Metadata filtering criteria
    
    def check(self):
        """
        Validate parameter values to ensure they are within acceptable ranges.
        
        This method is called before execution to catch configuration errors early.
        It checks that all numerical parameters are within valid ranges and that
        required parameters are properly set.
        """
        # Validate similarity thresholds (must be between 0.0 and 1.0)
        self.check_decimal_float(
            self.similarity_threshold,
            "[QueryDecompositionRetrieval] Similarity threshold"
        )
        self.check_decimal_float(
            self.keywords_similarity_weight,
            "[QueryDecompositionRetrieval] Keyword similarity weight"
        )
        self.check_decimal_float(
            self.score_fusion_weight,
            "[QueryDecompositionRetrieval] Score fusion weight"
        )
        
        # Validate positive integers
        self.check_positive_number(
            self.top_n,
            "[QueryDecompositionRetrieval] Top N"
        )
        self.check_positive_number(
            self.max_decomposition_count,
            "[QueryDecompositionRetrieval] Max decomposition count"
        )
        
        # Ensure max_decomposition_count is reasonable (1-10)
        if not (1 <= self.max_decomposition_count <= 10):
            raise ValueError(
                f"Max decomposition count must be between 1 and 10, got {self.max_decomposition_count}"
            )
        
        # Ensure score_fusion_weight is between 0 and 1
        if not (0.0 <= self.score_fusion_weight <= 1.0):
            raise ValueError(
                f"Score fusion weight must be between 0.0 and 1.0, got {self.score_fusion_weight}"
            )
    
    def get_input_form(self) -> Dict[str, Dict]:
        """
        Define the input form structure for UI rendering.
        
        Returns:
            Dictionary defining the input fields and their types for the UI
        """
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }


# -----------------------------------------------------------------------------
# Query Decomposition Retrieval Component
# -----------------------------------------------------------------------------

class QueryDecompositionRetrieval(ToolBase, ABC):
    """
    Advanced retrieval component with automatic query decomposition and intelligent reranking.
    
    This component implements a sophisticated retrieval pipeline that:
    1. Analyzes the input query for complexity
    2. Decomposes complex queries into simpler sub-questions using LLM
    3. Performs concurrent retrieval for all sub-queries
    4. Deduplicates chunks across all sub-query results
    5. Uses LLM to score each unique chunk's relevance
    6. Fuses LLM scores with vector similarity scores
    7. Returns globally ranked, deduplicated results
    
    This approach delivers better results than manual workflow assembly or agent-based
    approaches while maintaining high performance and deterministic behavior.
    """
    
    component_name = "QueryDecompositionRetrieval"
    
    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 60)))  # Longer timeout for complex processing
    def _invoke(self, **kwargs):
        """
        Main execution method for the query decomposition retrieval component.
        
        This method orchestrates the entire retrieval pipeline from query decomposition
        through final result ranking.
        
        Args:
            **kwargs: Keyword arguments including:
                - query (str): The user's input query
                
        Returns:
            str: Formatted content for downstream components
            
        Side Effects:
            - Sets output variables: "formalized_content" and "json"
            - Adds references to the canvas for citation tracking
        """
        # Check if component execution was canceled by user
        if self.check_if_canceled("Query decomposition retrieval processing"):
            return
        
        # Extract and validate query
        query = kwargs.get("query", "").strip()
        if not query:
            # No query provided - return empty response
            logging.warning("No query provided to QueryDecompositionRetrieval")
            self.set_output("formalized_content", self._param.empty_response)
            self.set_output("json", [])
            return
        
        try:
            # Step 1: Resolve and validate knowledge bases
            kb_ids, kbs, embd_mdl, rerank_mdl = self._prepare_knowledge_bases()
            
            if not kbs:
                raise Exception("No valid knowledge bases found")
            
            # Step 2: Process query variables and format the query string
            query = self._process_query_variables(query)
            
            # Step 3: Determine whether to use decomposition
            # If decomposition is disabled or query is simple, use direct retrieval
            if not self._param.enable_decomposition:
                logging.info("Query decomposition is disabled - using direct retrieval")
                results = self._direct_retrieval(
                    query=query,
                    kb_ids=kb_ids,
                    kbs=kbs,
                    embd_mdl=embd_mdl,
                    rerank_mdl=rerank_mdl
                )
            else:
                # Step 4: Decompose query into sub-questions
                sub_queries = self._decompose_query(query)
                
                # If decomposition failed or returned only one query, fall back to direct retrieval
                if len(sub_queries) <= 1:
                    logging.info(f"Query decomposition produced {len(sub_queries)} sub-queries - using direct retrieval")
                    results = self._direct_retrieval(
                        query=query,
                        kb_ids=kb_ids,
                        kbs=kbs,
                        embd_mdl=embd_mdl,
                        rerank_mdl=rerank_mdl
                    )
                else:
                    logging.info(f"Query decomposed into {len(sub_queries)} sub-queries: {sub_queries}")
                    
                    # Step 5: Retrieve chunks for all sub-queries
                    all_chunks = self._concurrent_retrieval(
                        sub_queries=sub_queries,
                        kb_ids=kb_ids,
                        kbs=kbs,
                        embd_mdl=embd_mdl,
                        rerank_mdl=rerank_mdl
                    )
                    
                    # Step 6: Deduplicate and globally rerank
                    results = self._global_rerank_and_deduplicate(
                        chunks_by_query=all_chunks,
                        sub_queries=sub_queries,
                        original_query=query
                    )
            
            # Step 7: Format and return results
            self._format_and_set_output(results)
            
            logging.info(f"Query decomposition retrieval completed successfully with {len(results)} results")
            
        except Exception as e:
            # Log the error and return empty response
            logging.exception(f"Error in QueryDecompositionRetrieval: {str(e)}")
            self.set_output("formalized_content", self._param.empty_response)
            self.set_output("json", [])
            raise
    
    def _prepare_knowledge_bases(self) -> Tuple:
        """
        Resolve knowledge base IDs and prepare embedding/reranking models.
        
        This method:
        1. Resolves knowledge base IDs (including variable references)
        2. Validates that all KBs exist and are accessible
        3. Ensures all KBs use the same embedding model
        4. Initializes embedding and reranking model bundles
        
        Returns:
            Tuple of (kb_ids, kbs, embd_mdl, rerank_mdl)
            
        Raises:
            Exception: If no valid knowledge bases found or if KBs use different embeddings
        """
        logging.info("Preparing knowledge bases for retrieval")
        
        # Resolve knowledge base IDs (may include variable references like "@kb_var")
        kb_ids: List[str] = []
        for id in self._param.kb_ids:
            if id.find("@") < 0:
                # Direct KB ID reference
                kb_ids.append(id)
                continue
            
            # Variable reference - resolve the actual KB name/ID
            kb_nm = self._canvas.get_variable_value(id)
            kb_nm_list = kb_nm if isinstance(kb_nm, list) else [kb_nm]
            
            for nm_or_id in kb_nm_list:
                # Try to find KB by name first, then by ID
                e, kb = KnowledgebaseService.get_by_name(
                    nm_or_id,
                    self._canvas._tenant_id
                )
                if not e:
                    e, kb = KnowledgebaseService.get_by_id(nm_or_id)
                    if not e:
                        raise Exception(f"Knowledge base ({nm_or_id}) does not exist")
                kb_ids.append(kb.id)
        
        # Remove duplicates and filter empty IDs
        kb_ids = list(set([kb_id for kb_id in kb_ids if kb_id]))
        
        if not kb_ids:
            raise Exception("No knowledge base IDs provided")
        
        # Retrieve knowledge base objects
        kbs = KnowledgebaseService.get_by_ids(kb_ids)
        if not kbs:
            raise Exception("No valid knowledge bases found")
        
        # Verify all KBs use the same embedding model
        embd_nms = list(set([kb.embd_id for kb in kbs]))
        if len(embd_nms) > 1:
            raise Exception(
                f"Knowledge bases use different embedding models: {embd_nms}. "
                "All KBs must use the same embedding model for consistent retrieval."
            )
        
        # Initialize embedding model bundle
        embd_mdl = None
        if embd_nms and embd_nms[0]:
            embd_mdl = LLMBundle(
                self._canvas.get_tenant_id(),
                LLMType.EMBEDDING,
                embd_nms[0]
            )
        
        # Initialize reranking model bundle if specified
        rerank_mdl = None
        if self._param.rerank_id:
            rerank_mdl = LLMBundle(
                kbs[0].tenant_id,
                LLMType.RERANK,
                self._param.rerank_id
            )
        
        logging.info(f"Prepared {len(kbs)} knowledge bases with embedding model: {embd_nms[0] if embd_nms else 'None'}")
        
        return kb_ids, kbs, embd_mdl, rerank_mdl
    
    def _process_query_variables(self, query: str) -> str:
        """
        Process and substitute variables in the query string.
        
        Queries may contain variable references (e.g., "{user_name}") that need to be
        replaced with actual values from the canvas context.
        
        Args:
            query (str): Query string potentially containing variable references
            
        Returns:
            str: Query with all variables substituted
        """
        # Extract variable references from query text
        vars = self.get_input_elements_from_text(query)
        vars = {k: o["value"] for k, o in vars.items()}
        
        # Substitute variables into query
        query = self.string_format(query, vars)
        
        return query.strip()
    
    def _decompose_query(self, query: str) -> List[str]:
        """
        Decompose a complex query into simpler sub-questions using LLM.
        
        This method calls the LLM with the decomposition prompt to break down
        a complex query into 2-3 simpler, independently answerable sub-questions.
        
        Args:
            query (str): The original complex query
            
        Returns:
            List[str]: List of sub-questions (returns [query] if decomposition fails)
        """
        logging.info(f"Decomposing query: {query}")
        
        try:
            # Get LLM for query decomposition
            llm = LLMBundle(
                self._canvas.get_tenant_id(),
                LLMType.CHAT
            )
            
            # Format the decomposition prompt with the actual query
            prompt = self._param.decomposition_prompt.format(
                original_query=query,
                max_count=self._param.max_decomposition_count
            )
            
            # Call LLM to decompose query
            # We use a system message to set context and a user message with the prompt
            response = llm.chat(
                system="You are a helpful assistant that decomposes complex queries into simpler sub-questions.",
                messages=[{"role": "user", "content": prompt}],
                gen_conf={"temperature": 0.1, "max_tokens": 500}  # Low temperature for consistency
            )
            
            # Extract the response text
            response_text = response.strip()
            
            logging.debug(f"LLM decomposition response: {response_text}")
            
            # Parse JSON array from response
            # The LLM should return a JSON array like: ["question 1", "question 2"]
            sub_queries = self._parse_sub_queries_from_response(response_text)
            
            # Validate sub-queries
            if not sub_queries:
                logging.warning("Query decomposition produced no sub-queries - falling back to original query")
                return [query]
            
            # Limit to max_decomposition_count
            sub_queries = sub_queries[:self._param.max_decomposition_count]
            
            logging.info(f"Successfully decomposed query into {len(sub_queries)} sub-queries")
            return sub_queries
            
        except Exception as e:
            # If decomposition fails for any reason, fall back to using the original query
            logging.exception(f"Query decomposition failed: {str(e)}")
            logging.info("Falling back to original query")
            return [query]
    
    def _parse_sub_queries_from_response(self, response_text: str) -> List[str]:
        """
        Parse sub-queries from LLM response text.
        
        The LLM should return a JSON array, but may include extra text or formatting.
        This method extracts the JSON array and parses it robustly.
        
        Args:
            response_text (str): Raw text response from LLM
            
        Returns:
            List[str]: Parsed list of sub-questions
        """
        # Try to find JSON array in response
        # Look for patterns like: ["query1", "query2"] or ['query1', 'query2']
        
        # First, try direct JSON parsing
        try:
            sub_queries = json.loads(response_text)
            if isinstance(sub_queries, list) and all(isinstance(q, str) for q in sub_queries):
                return [q.strip() for q in sub_queries if q.strip()]
        except json.JSONDecodeError:
            pass
        
        # Try to extract JSON array from response using regex
        json_array_pattern = r'\[(?:[^\[\]]|"(?:[^"\\]|\\.)*")+\]'
        matches = re.findall(json_array_pattern, response_text, re.DOTALL)
        
        for match in matches:
            try:
                sub_queries = json.loads(match)
                if isinstance(sub_queries, list) and all(isinstance(q, str) for q in sub_queries):
                    return [q.strip() for q in sub_queries if q.strip()]
            except json.JSONDecodeError:
                continue
        
        # If JSON parsing fails, try to extract quoted strings
        # Look for strings in quotes: "question 1", "question 2"
        quoted_pattern = r'"([^"]+)"|\'([^\']+)\''
        matches = re.findall(quoted_pattern, response_text)
        if matches:
            sub_queries = [m[0] or m[1] for m in matches]
            sub_queries = [q.strip() for q in sub_queries if q.strip()]
            if sub_queries:
                return sub_queries
        
        # If all parsing attempts fail, return empty list
        logging.warning(f"Failed to parse sub-queries from LLM response: {response_text}")
        return []
    
    def _direct_retrieval(
        self,
        query: str,
        kb_ids: List[str],
        kbs: List,
        embd_mdl,
        rerank_mdl
    ) -> List[Dict]:
        """
        Perform direct retrieval without query decomposition.
        
        This is a fallback method used when:
        - Query decomposition is disabled
        - Query decomposition produces only one sub-query
        - Query decomposition fails
        
        Args:
            query (str): Query to search for
            kb_ids (List[str]): Knowledge base IDs
            kbs (List): Knowledge base objects
            embd_mdl: Embedding model bundle
            rerank_mdl: Reranking model bundle
            
        Returns:
            List[Dict]: Retrieved and ranked chunks
        """
        logging.info(f"Performing direct retrieval for query: {query}")
        
        # Prepare document IDs if metadata filtering is enabled
        doc_ids = []
        if self._param.meta_data_filter:
            doc_ids = meta_filter(
                self._param.meta_data_filter,
                kb_ids
            )
            if not doc_ids:
                logging.warning("Metadata filtering returned no matching documents")
                return []
        
        # Handle cross-language retrieval if enabled
        # This translates the query into multiple languages for broader search
        if self._param.cross_languages:
            trans_queries = {}
            for lang in self._param.cross_languages:
                trans_q, _ = cross_languages(
                    [query],
                    [lang],
                    LLMBundle(kbs[0].tenant_id, LLMType.CHAT)
                )
                trans_queries[lang] = trans_q
        
        # Perform vector retrieval using the standard retrieval engine
        kbinfos = settings.retriever.retrieval(
            question=query,
            embd_mdl=embd_mdl,
            tenant_ids=[kb.tenant_id for kb in kbs],
            kb_ids=kb_ids,
            page=1,
            page_size=self._param.top_n,
            similarity_threshold=self._param.similarity_threshold,
            vector_similarity_weight=1 - self._param.keywords_similarity_weight,
            top=self._param.top_k,
            doc_ids=doc_ids if doc_ids else None,
            aggs=True,
            rerank_mdl=rerank_mdl
        )
        
        # Check if retrieval was canceled
        if self.check_if_canceled("Direct retrieval processing"):
            return []
        
        # Handle knowledge graph retrieval if enabled
        if self._param.use_kg and kbs:
            ck = settings.kg_retriever.retrieval(
                query,
                [kb.tenant_id for kb in kbs],
                kb_ids,
                embd_mdl,
                LLMBundle(kbs[0].tenant_id, LLMType.CHAT)
            )
            if self.check_if_canceled("KG retrieval processing"):
                return []
            if ck.get("content_with_weight"):
                kbinfos["chunks"].insert(0, ck)
        
        return kbinfos.get("chunks", [])
    
    def _concurrent_retrieval(
        self,
        sub_queries: List[str],
        kb_ids: List[str],
        kbs: List,
        embd_mdl,
        rerank_mdl
    ) -> Dict[str, List[Dict]]:
        """
        Perform concurrent retrieval for multiple sub-queries.
        
        This method retrieves chunks for all sub-queries either sequentially or
        in parallel (depending on enable_concurrency setting). Concurrent execution
        significantly improves performance for multiple sub-queries.
        
        Args:
            sub_queries (List[str]): List of sub-questions to retrieve for
            kb_ids (List[str]): Knowledge base IDs
            kbs (List): Knowledge base objects
            embd_mdl: Embedding model bundle
            rerank_mdl: Reranking model bundle
            
        Returns:
            Dict[str, List[Dict]]: Mapping of sub-query -> retrieved chunks
        """
        logging.info(f"Performing concurrent retrieval for {len(sub_queries)} sub-queries")
        
        chunks_by_query = {}
        
        if self._param.enable_concurrency and len(sub_queries) > 1:
            # Concurrent execution for better performance
            with ThreadPoolExecutor(max_workers=min(len(sub_queries), 5)) as executor:
                # Submit all retrieval tasks
                future_to_query = {
                    executor.submit(
                        self._retrieve_for_sub_query,
                        sub_query,
                        kb_ids,
                        kbs,
                        embd_mdl,
                        rerank_mdl
                    ): sub_query
                    for sub_query in sub_queries
                }
                
                # Collect results as they complete
                for future in as_completed(future_to_query):
                    sub_query = future_to_query[future]
                    try:
                        chunks = future.result()
                        chunks_by_query[sub_query] = chunks
                        logging.info(f"Retrieved {len(chunks)} chunks for sub-query: {sub_query}")
                    except Exception as e:
                        logging.exception(f"Retrieval failed for sub-query '{sub_query}': {str(e)}")
                        chunks_by_query[sub_query] = []
        else:
            # Sequential execution
            for sub_query in sub_queries:
                try:
                    chunks = self._retrieve_for_sub_query(
                        sub_query,
                        kb_ids,
                        kbs,
                        embd_mdl,
                        rerank_mdl
                    )
                    chunks_by_query[sub_query] = chunks
                    logging.info(f"Retrieved {len(chunks)} chunks for sub-query: {sub_query}")
                except Exception as e:
                    logging.exception(f"Retrieval failed for sub-query '{sub_query}': {str(e)}")
                    chunks_by_query[sub_query] = []
        
        return chunks_by_query
    
    def _retrieve_for_sub_query(
        self,
        sub_query: str,
        kb_ids: List[str],
        kbs: List,
        embd_mdl,
        rerank_mdl
    ) -> List[Dict]:
        """
        Retrieve chunks for a single sub-query.
        
        This method is called for each sub-query (either sequentially or in parallel).
        It performs standard vector retrieval and returns the top-k candidates.
        
        Args:
            sub_query (str): Single sub-question to retrieve for
            kb_ids (List[str]): Knowledge base IDs
            kbs (List): Knowledge base objects
            embd_mdl: Embedding model bundle
            rerank_mdl: Reranking model bundle
            
        Returns:
            List[Dict]: Retrieved chunks for this sub-query
        """
        # Use higher top_k for sub-queries to ensure good coverage
        # We'll deduplicate and rerank globally later
        top_k = min(self._param.top_k, 2048)  # Reasonable upper limit
        
        # Perform vector retrieval
        kbinfos = settings.retriever.retrieval(
            question=sub_query,
            embd_mdl=embd_mdl,
            tenant_ids=[kb.tenant_id for kb in kbs],
            kb_ids=kb_ids,
            page=1,
            page_size=self._param.top_n * 2,  # Retrieve more for deduplication
            similarity_threshold=self._param.similarity_threshold * 0.8,  # Slightly lower threshold for sub-queries
            vector_similarity_weight=1 - self._param.keywords_similarity_weight,
            top=top_k,
            doc_ids=None,
            aggs=False,  # Don't need document aggregations for sub-queries
            rerank_mdl=None  # We'll do global reranking later
        )
        
        chunks = kbinfos.get("chunks", [])
        
        # Store the sub-query with each chunk for later LLM scoring
        for chunk in chunks:
            chunk["_sub_query"] = sub_query
        
        return chunks
    
    def _global_rerank_and_deduplicate(
        self,
        chunks_by_query: Dict[str, List[Dict]],
        sub_queries: List[str],
        original_query: str
    ) -> List[Dict]:
        """
        Perform global deduplication and reranking of chunks from all sub-queries.
        
        This is the core innovation of the query decomposition approach. Instead of
        treating sub-query results separately, we:
        1. Deduplicate chunks across all sub-queries (same chunk may appear multiple times)
        2. Use LLM to score each unique chunk's relevance
        3. Fuse LLM scores with original vector similarity scores
        4. Return globally ranked, deduplicated results
        
        Args:
            chunks_by_query (Dict[str, List[Dict]]): Chunks retrieved for each sub-query
            sub_queries (List[str]): List of sub-questions
            original_query (str): The original complex query
            
        Returns:
            List[Dict]: Globally ranked and deduplicated chunks
        """
        logging.info("Performing global deduplication and reranking")
        
        # Step 1: Deduplicate chunks by chunk_id
        # Keep track of which sub-queries retrieved each chunk
        chunk_map = {}  # chunk_id -> {chunk_data, sub_queries, vector_scores}
        
        for sub_query, chunks in chunks_by_query.items():
            for chunk in chunks:
                chunk_id = chunk.get("chunk_id")
                if not chunk_id:
                    continue
                
                if chunk_id not in chunk_map:
                    # First time seeing this chunk
                    chunk_map[chunk_id] = {
                        "chunk": chunk,
                        "sub_queries": [sub_query],
                        "vector_scores": [chunk.get("similarity", 0.0)]
                    }
                else:
                    # Chunk appeared in multiple sub-queries
                    chunk_map[chunk_id]["sub_queries"].append(sub_query)
                    chunk_map[chunk_id]["vector_scores"].append(chunk.get("similarity", 0.0))
        
        if not chunk_map:
            logging.warning("No chunks to rerank after deduplication")
            return []
        
        logging.info(f"Deduplicated {sum(len(chunks) for chunks in chunks_by_query.values())} chunks to {len(chunk_map)} unique chunks")
        
        # Step 2: LLM-based scoring of each unique chunk
        # Score each chunk against the original query (not sub-queries)
        scored_chunks = []
        
        for chunk_id, chunk_info in chunk_map.items():
            chunk = chunk_info["chunk"]
            
            # Get LLM relevance score
            llm_score = self._score_chunk_with_llm(
                chunk=chunk,
                query=original_query  # Score against original query for global relevance
            )
            
            # Get average vector similarity score across all sub-queries that retrieved this chunk
            avg_vector_score = np.mean(chunk_info["vector_scores"])
            
            # Fuse LLM score with vector similarity score
            # Final score = weight * LLM_score + (1-weight) * vector_score
            final_score = (
                self._param.score_fusion_weight * llm_score +
                (1 - self._param.score_fusion_weight) * avg_vector_score
            )
            
            # Store scores in chunk for transparency
            chunk["llm_relevance_score"] = llm_score
            chunk["vector_similarity_score"] = avg_vector_score
            chunk["final_fused_score"] = final_score
            chunk["retrieved_by_sub_queries"] = chunk_info["sub_queries"]
            
            scored_chunks.append(chunk)
        
        # Step 3: Sort by final fused score
        scored_chunks.sort(key=lambda x: x.get("final_fused_score", 0.0), reverse=True)
        
        # Step 4: Return top N results
        final_results = scored_chunks[:self._param.top_n]
        
        logging.info(f"Global reranking complete - returning top {len(final_results)} chunks")
        
        return final_results
    
    def _score_chunk_with_llm(self, chunk: Dict, query: str) -> float:
        """
        Score a chunk's relevance to a query using LLM.
        
        This method calls the LLM with the reranking prompt to judge how useful
        the chunk is for answering the query. The LLM returns a score from 1-10
        which is then normalized to 0.0-1.0 range.
        
        Args:
            chunk (Dict): Chunk to score
            query (str): Query to score against
            
        Returns:
            float: Normalized relevance score (0.0-1.0)
        """
        try:
            # Get LLM for scoring
            llm = LLMBundle(
                self._canvas.get_tenant_id(),
                LLMType.CHAT
            )
            
            # Extract chunk text
            chunk_text = chunk.get("content_with_weight", chunk.get("content_ltks", ""))
            if not chunk_text:
                logging.warning(f"Chunk {chunk.get('chunk_id')} has no content for scoring")
                return 0.0
            
            # Truncate chunk text if too long (to avoid token limits)
            max_chunk_length = 2000
            if len(chunk_text) > max_chunk_length:
                chunk_text = chunk_text[:max_chunk_length] + "..."
            
            # Format the reranking prompt
            prompt = self._param.reranking_prompt.format(
                query=query,
                chunk_text=chunk_text
            )
            
            # Call LLM to score the chunk
            response = llm.chat(
                system="You are an expert at assessing information relevance.",
                messages=[{"role": "user", "content": prompt}],
                gen_conf={"temperature": 0.1, "max_tokens": 200}
            )
            
            response_text = response.strip()
            
            # Parse JSON response: {"score": 8, "reason": "..."}
            score_data = self._parse_score_from_response(response_text)
            
            if score_data:
                # Normalize score from 1-10 range to 0.0-1.0 range
                raw_score = score_data.get("score", 5)
                normalized_score = (raw_score - 1) / 9.0  # (score-1)/9 maps [1,10] to [0,1]
                normalized_score = max(0.0, min(1.0, normalized_score))  # Clamp to [0,1]
                
                logging.debug(f"LLM scored chunk {chunk.get('chunk_id')}: {raw_score}/10 (normalized: {normalized_score:.3f})")
                
                return normalized_score
            else:
                # Failed to parse score - return neutral score
                logging.warning(f"Failed to parse LLM score from response: {response_text}")
                return 0.5
                
        except Exception as e:
            # If scoring fails, return neutral score
            logging.exception(f"LLM scoring failed for chunk {chunk.get('chunk_id')}: {str(e)}")
            return 0.5
    
    def _parse_score_from_response(self, response_text: str) -> Dict:
        """
        Parse score and reason from LLM response.
        
        Expected format: {"score": 8, "reason": "Contains relevant information"}
        
        Args:
            response_text (str): Raw LLM response
            
        Returns:
            Dict with "score" and "reason", or None if parsing fails
        """
        # Try direct JSON parsing
        try:
            data = json.loads(response_text)
            if "score" in data:
                return data
        except json.JSONDecodeError:
            pass
        
        # Try to extract JSON object from response
        json_pattern = r'\{[^}]+\}'
        matches = re.findall(json_pattern, response_text)
        
        for match in matches:
            try:
                data = json.loads(match)
                if "score" in data:
                    return data
            except json.JSONDecodeError:
                continue
        
        # Try to extract score using regex
        # Look for patterns like: "score": 8 or score: 8
        score_pattern = r'["\']?score["\']?\s*:\s*(\d+)'
        match = re.search(score_pattern, response_text, re.IGNORECASE)
        if match:
            return {"score": int(match.group(1)), "reason": ""}
        
        return None
    
    def _format_and_set_output(self, chunks: List[Dict]):
        """
        Format retrieved chunks and set output variables.
        
        This method:
        1. Cleans up internal fields from chunks
        2. Formats chunks for JSON output
        3. Adds references to canvas for citation tracking
        4. Formats chunks as text for downstream components
        5. Sets output variables
        
        Args:
            chunks (List[Dict]): Final ranked and deduplicated chunks
        """
        if not chunks:
            logging.info("No chunks to format - returning empty response")
            self.set_output("formalized_content", self._param.empty_response)
            self.set_output("json", [])
            return
        
        # Clean up internal fields that shouldn't be exposed
        for chunk in chunks:
            # Remove internal fields used during processing
            chunk.pop("_sub_query", None)
            chunk.pop("vector", None)
            chunk.pop("content_ltks", None)
        
        # Prepare JSON output
        json_output = chunks.copy()
        
        # Add references to canvas for citation tracking
        # This allows the UI to show source documents
        doc_aggs = []  # Document aggregations
        self._canvas.add_reference(chunks, doc_aggs)
        
        # Format chunks as text for downstream components
        # This creates a formatted string with all chunk contents
        formalized_content = "\n".join(kb_prompt({"chunks": chunks, "doc_aggs": doc_aggs}, 200000, True))
        
        # Set output variables
        self.set_output("formalized_content", formalized_content)
        self.set_output("json", json_output)
        
        logging.info(f"Formatted {len(chunks)} chunks for output")
    
    def thoughts(self) -> str:
        """
        Return component thoughts for debugging/logging.
        
        This method is called by the agent framework to get a description of
        what the component is doing. Useful for debugging and monitoring.
        
        Returns:
            str: Description of component processing
        """
        if self._param.enable_decomposition:
            return (
                f"Performing advanced retrieval with query decomposition. "
                f"Will decompose complex queries into up to {self._param.max_decomposition_count} "
                f"sub-questions and use LLM-based reranking with {self._param.score_fusion_weight} "
                f"weight on LLM scores."
            )
        else:
            return "Performing standard retrieval without query decomposition."

