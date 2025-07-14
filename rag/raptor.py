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
import asyncio
import logging
import re
import time
from typing import List, Tuple, Optional, Any, Dict
import psutil
import umap
import numpy as np
from sklearn.mixture import GaussianMixture
import trio

from graphrag.utils import (
    get_llm_cache,
    get_embed_cache,
    set_embed_cache,
    set_llm_cache,
    chat_limiter,
)
from rag.utils import truncate
from rag.utils.timeout_manager import run_with_timeout
from rag.utils.raptor_monitor import get_raptor_monitor


# Custom Exception Classes for RAPTOR
class RaptorError(Exception):
    """Base exception for RAPTOR-related errors."""
    pass


class RaptorValidationError(RaptorError):
    """Raised when input validation fails."""
    pass


class RaptorLLMError(RaptorError):
    """Raised when LLM operations fail."""
    pass


class RaptorEmbeddingError(RaptorError):
    """Raised when embedding operations fail."""
    pass


class RaptorClusteringError(RaptorError):
    """Raised when clustering operations fail."""
    pass


class RaptorResourceError(RaptorError):
    """Raised when resource limits are exceeded."""
    pass


class RaptorTimeoutError(RaptorError):
    """Raised when operations timeout."""
    pass


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    """
    Enhanced RAPTOR implementation with comprehensive error handling and resource management.

    This class implements the Recursive Abstractive Processing for Tree-Organized Retrieval
    algorithm with production-ready robustness features including input validation,
    retry mechanisms, timeout handling, and resource monitoring.
    """

    # Class constants for validation
    MIN_MAX_CLUSTER = 1
    MAX_MAX_CLUSTER = 1024
    MIN_MAX_TOKEN = 1
    MAX_MAX_TOKEN = 2048
    MIN_THRESHOLD = 0.0
    MAX_THRESHOLD = 1.0
    MAX_CHUNK_SIZE = 1000000  # 1MB per chunk
    MAX_TOTAL_CHUNKS = 10000
    MAX_MEMORY_MB = 2048  # 2GB memory limit

    def __init__(
        self, max_cluster: int, llm_model, embd_model, prompt: str,
        max_token: int = 512, threshold: float = 0.1, max_retries: int = 3,
        timeout_seconds: float = 300.0, enable_monitoring: bool = True
    ):
        """
        Initialize RAPTOR with comprehensive validation and configuration.

        Args:
            max_cluster: Maximum number of clusters (1-1024)
            llm_model: Language model for summarization
            embd_model: Embedding model for vector generation
            prompt: Template prompt for summarization
            max_token: Maximum tokens for LLM response (1-2048)
            threshold: Clustering threshold (0.0-1.0)
            max_retries: Maximum retry attempts for failed operations
            timeout_seconds: Timeout for individual operations
            enable_monitoring: Whether to enable resource monitoring

        Raises:
            RaptorValidationError: If any parameter is invalid
        """
        self._validate_init_params(max_cluster, prompt, max_token, threshold, max_retries, timeout_seconds)

        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token
        self._max_retries = max_retries
        self._timeout_seconds = timeout_seconds
        self._enable_monitoring = enable_monitoring

        # Performance tracking
        self._stats = {
            'total_chunks_processed': 0,
            'total_llm_calls': 0,
            'total_embedding_calls': 0,
            'total_clustering_operations': 0,
            'failed_operations': 0,
            'cache_hits': 0,
            'processing_time': 0.0
        }

        # Resource monitoring
        self._process = psutil.Process() if enable_monitoring else None
        self._initial_memory = self._get_memory_usage() if enable_monitoring else 0

        # Monitoring integration
        self._monitor = get_raptor_monitor() if enable_monitoring else None

        logging.info(f"RAPTOR initialized with max_cluster={max_cluster}, max_token={max_token}, "
                    f"threshold={threshold}, max_retries={max_retries}, timeout={timeout_seconds}s")

    def _validate_init_params(self, max_cluster: int, prompt: str, max_token: int,
                             threshold: float, max_retries: int, timeout_seconds: float) -> None:
        """Validate initialization parameters."""
        if not isinstance(max_cluster, int) or not (self.MIN_MAX_CLUSTER <= max_cluster <= self.MAX_MAX_CLUSTER):
            raise RaptorValidationError(f"max_cluster must be an integer between {self.MIN_MAX_CLUSTER} and {self.MAX_MAX_CLUSTER}")

        if not isinstance(prompt, str) or not prompt.strip():
            raise RaptorValidationError("prompt must be a non-empty string")

        if "{cluster_content}" not in prompt:
            raise RaptorValidationError("prompt must contain '{cluster_content}' placeholder")

        if not isinstance(max_token, int) or not (self.MIN_MAX_TOKEN <= max_token <= self.MAX_MAX_TOKEN):
            raise RaptorValidationError(f"max_token must be an integer between {self.MIN_MAX_TOKEN} and {self.MAX_MAX_TOKEN}")

        if not isinstance(threshold, (int, float)) or not (self.MIN_THRESHOLD <= threshold <= self.MAX_THRESHOLD):
            raise RaptorValidationError(f"threshold must be a number between {self.MIN_THRESHOLD} and {self.MAX_THRESHOLD}")

        if not isinstance(max_retries, int) or max_retries < 0:
            raise RaptorValidationError("max_retries must be a non-negative integer")

        if not isinstance(timeout_seconds, (int, float)) or timeout_seconds <= 0:
            raise RaptorValidationError("timeout_seconds must be a positive number")

    def _get_memory_usage(self) -> float:
        """Get current memory usage in MB."""
        if not self._process:
            return 0.0
        try:
            return self._process.memory_info().rss / 1024 / 1024
        except Exception:
            return 0.0

    def _check_memory_limit(self) -> None:
        """Check if memory usage exceeds limits."""
        if not self._enable_monitoring:
            return

        current_memory = self._get_memory_usage()
        memory_increase = current_memory - self._initial_memory

        if memory_increase > self.MAX_MEMORY_MB:
            raise RaptorResourceError(f"Memory usage exceeded limit: {memory_increase:.1f}MB > {self.MAX_MEMORY_MB}MB")

    def get_stats(self) -> Dict[str, Any]:
        """Get performance statistics."""
        stats = self._stats.copy()

        # Add monitoring data if available
        if self._monitor:
            monitor_stats = self._monitor.get_metrics()
            stats['monitoring'] = monitor_stats

        return stats

    def get_monitoring_report(self) -> str:
        """Get comprehensive monitoring report."""
        if not self._monitor:
            return "Monitoring is disabled for this RAPTOR instance"

        return self._monitor.get_summary_report()

    def export_monitoring_data(self, filepath: str) -> None:
        """Export monitoring data to file."""
        if not self._monitor:
            logging.warning("Cannot export monitoring data: monitoring is disabled")
            return

        self._monitor.export_metrics(filepath)

    async def _chat(self, system: str, history: List[Dict[str, str]], gen_conf: Dict[str, Any]) -> str:
        """
        Enhanced chat method with retry logic and comprehensive error handling.

        Args:
            system: System prompt
            history: Chat history
            gen_conf: Generation configuration

        Returns:
            Generated response text

        Raises:
            RaptorLLMError: If LLM operation fails after retries
            RaptorTimeoutError: If operation times out
        """
        # Validate inputs
        if not isinstance(system, str) or not system.strip():
            raise RaptorValidationError("system prompt must be a non-empty string")

        if not isinstance(history, list):
            raise RaptorValidationError("history must be a list")

        # Check cache first
        response = get_llm_cache(self._llm_model.llm_name, system, history, gen_conf)
        if response:
            self._stats['cache_hits'] += 1
            if self._monitor:
                self._monitor.end_operation("llm_cache_hit", success=True, cache_hit=True)
            logging.debug("Cache hit for LLM request")
            return response

        # Attempt LLM call with retries
        last_exception = None
        for attempt in range(self._max_retries + 1):
            operation_id = f"llm_call_{attempt}_{time.time()}"
            try:
                self._stats['total_llm_calls'] += 1

                # Start monitoring
                if self._monitor:
                    self._monitor.start_operation(operation_id, "llm")

                # Use timeout wrapper
                async def llm_call():
                    return await trio.to_thread.run_sync(
                        lambda: self._llm_model.chat(system, history, gen_conf)
                    )

                response = await run_with_timeout(
                    llm_call(),
                    "external_api",
                    f"llm_chat_attempt_{attempt}"
                )

                # Validate response
                if not response or not isinstance(response, str):
                    raise RaptorLLMError(f"Invalid LLM response: {type(response)}")

                # Clean response
                response = re.sub(r"^.*</think>", "", response, flags=re.DOTALL)

                # Check for error markers
                if response.find("**ERROR**") >= 0:
                    raise RaptorLLMError(f"LLM returned error: {response}")

                # Cache successful response
                set_llm_cache(self._llm_model.llm_name, system, response, history, gen_conf)

                # End monitoring with success
                if self._monitor:
                    token_count = len(response.split())  # Rough token estimate
                    self._monitor.end_operation(operation_id, success=True, tokens_used=token_count)

                if attempt > 0:
                    logging.info(f"LLM call succeeded on attempt {attempt + 1}")

                return response

            except asyncio.TimeoutError as e:
                last_exception = RaptorTimeoutError(f"LLM call timed out on attempt {attempt + 1}: {e}")
                logging.warning(f"LLM timeout on attempt {attempt + 1}: {e}")
                if self._monitor:
                    self._monitor.end_operation(operation_id, success=False)
                    self._monitor.record_error("timeout")

            except Exception as e:
                last_exception = RaptorLLMError(f"LLM call failed on attempt {attempt + 1}: {e}")
                logging.warning(f"LLM error on attempt {attempt + 1}: {e}")
                if self._monitor:
                    self._monitor.end_operation(operation_id, success=False)
                    self._monitor.record_error("other")

            # Wait before retry (exponential backoff)
            if attempt < self._max_retries:
                wait_time = min(2 ** attempt, 30)  # Cap at 30 seconds
                logging.info(f"Retrying LLM call in {wait_time} seconds...")
                await trio.sleep(wait_time)

        # All retries failed
        self._stats['failed_operations'] += 1
        error_msg = f"LLM call failed after {self._max_retries + 1} attempts"
        if last_exception:
            error_msg += f": {last_exception}"

        logging.error(error_msg)
        raise RaptorLLMError(error_msg)

    async def _embedding_encode(self, txt: str) -> np.ndarray:
        """
        Enhanced embedding encoding with validation and retry logic.

        Args:
            txt: Text to encode

        Returns:
            Embedding vector as numpy array

        Raises:
            RaptorEmbeddingError: If embedding operation fails
            RaptorValidationError: If input is invalid
        """
        # Validate input
        if not isinstance(txt, str):
            raise RaptorValidationError(f"Text must be string, got {type(txt)}")

        if not txt.strip():
            raise RaptorValidationError("Text cannot be empty")

        if len(txt) > self.MAX_CHUNK_SIZE:
            raise RaptorValidationError(f"Text too large: {len(txt)} > {self.MAX_CHUNK_SIZE}")

        # Check cache first
        response = get_embed_cache(self._embd_model.llm_name, txt)
        if response is not None:
            self._stats['cache_hits'] += 1
            logging.debug("Cache hit for embedding request")
            # Validate cached response
            if isinstance(response, np.ndarray) and response.size > 0:
                return response
            else:
                logging.warning("Invalid cached embedding, regenerating")

        # Attempt embedding with retries
        last_exception = None
        for attempt in range(self._max_retries + 1):
            try:
                self._stats['total_embedding_calls'] += 1

                # Use timeout wrapper
                async def embed_call():
                    return await trio.to_thread.run_sync(
                        lambda: self._embd_model.encode([txt])
                    )

                embds, token_count = await run_with_timeout(
                    embed_call(),
                    "external_api",
                    f"embedding_attempt_{attempt}"
                )

                # Validate response
                if not embds or len(embds) < 1:
                    raise RaptorEmbeddingError("Empty embedding response")

                embedding = embds[0]
                if not isinstance(embedding, np.ndarray) or embedding.size == 0:
                    raise RaptorEmbeddingError(f"Invalid embedding format: {type(embedding)}")

                # Additional validation
                if np.any(np.isnan(embedding)) or np.any(np.isinf(embedding)):
                    raise RaptorEmbeddingError("Embedding contains NaN or infinite values")

                # Cache successful result
                set_embed_cache(self._embd_model.llm_name, txt, embedding)

                if attempt > 0:
                    logging.info(f"Embedding call succeeded on attempt {attempt + 1}")

                return embedding

            except asyncio.TimeoutError as e:
                last_exception = RaptorTimeoutError(f"Embedding call timed out on attempt {attempt + 1}: {e}")
                logging.warning(f"Embedding timeout on attempt {attempt + 1}: {e}")

            except Exception as e:
                last_exception = RaptorEmbeddingError(f"Embedding call failed on attempt {attempt + 1}: {e}")
                logging.warning(f"Embedding error on attempt {attempt + 1}: {e}")

            # Wait before retry
            if attempt < self._max_retries:
                wait_time = min(2 ** attempt, 30)
                logging.info(f"Retrying embedding call in {wait_time} seconds...")
                await trio.sleep(wait_time)

        # All retries failed
        self._stats['failed_operations'] += 1
        error_msg = f"Embedding call failed after {self._max_retries + 1} attempts"
        if last_exception:
            error_msg += f": {last_exception}"

        logging.error(error_msg)
        raise RaptorEmbeddingError(error_msg)

    def _get_optimal_clusters(self, embeddings: np.ndarray, random_state: int) -> int:
        """
        Enhanced clustering with validation and error handling.

        Args:
            embeddings: Array of embeddings to cluster
            random_state: Random seed for reproducibility

        Returns:
            Optimal number of clusters

        Raises:
            RaptorClusteringError: If clustering fails
            RaptorValidationError: If inputs are invalid
        """
        # Validate inputs
        if not isinstance(embeddings, np.ndarray):
            raise RaptorValidationError(f"embeddings must be numpy array, got {type(embeddings)}")

        if embeddings.size == 0:
            raise RaptorValidationError("embeddings array is empty")

        if len(embeddings.shape) != 2:
            raise RaptorValidationError(f"embeddings must be 2D array, got shape {embeddings.shape}")

        if not isinstance(random_state, int) or random_state < 0:
            raise RaptorValidationError("random_state must be non-negative integer")

        # Check for degenerate cases
        if len(embeddings) == 1:
            return 1

        if len(embeddings) == 2:
            return 2

        # Check for identical embeddings
        if np.allclose(embeddings, embeddings[0], rtol=1e-10):
            logging.warning("All embeddings are identical, using single cluster")
            return 1

        # Check for NaN or infinite values
        if np.any(np.isnan(embeddings)) or np.any(np.isinf(embeddings)):
            raise RaptorClusteringError("Embeddings contain NaN or infinite values")

        try:
            self._stats['total_clustering_operations'] += 1

            max_clusters = min(self._max_cluster, len(embeddings))
            n_clusters_range = np.arange(1, max_clusters + 1)
            bics = []

            for n in n_clusters_range:
                try:
                    gm = GaussianMixture(
                        n_components=n,
                        random_state=random_state,
                        max_iter=100,  # Limit iterations
                        tol=1e-3,      # Convergence tolerance
                        reg_covar=1e-6  # Regularization
                    )
                    gm.fit(embeddings)

                    # Check if model converged
                    if not gm.converged_:
                        logging.warning(f"GaussianMixture with {n} components did not converge")
                        bics.append(float('inf'))  # Penalize non-convergence
                    else:
                        bics.append(gm.bic(embeddings))

                except Exception as e:
                    logging.warning(f"Failed to fit GaussianMixture with {n} components: {e}")
                    bics.append(float('inf'))

            # Find optimal clusters
            if not bics or all(bic == float('inf') for bic in bics):
                logging.warning("All clustering attempts failed, defaulting to 2 clusters")
                return min(2, len(embeddings))

            optimal_idx = np.argmin(bics)
            optimal_clusters = n_clusters_range[optimal_idx]

            logging.debug(f"Optimal clusters: {optimal_clusters} (BIC: {bics[optimal_idx]:.2f})")
            return optimal_clusters

        except Exception as e:
            self._stats['failed_operations'] += 1
            raise RaptorClusteringError(f"Clustering optimization failed: {e}")

    async def __call__(self, chunks: List[Tuple[str, np.ndarray]], random_state: int,
                      callback: Optional[callable] = None) -> List[Tuple[str, np.ndarray]]:
        """
        Enhanced main processing method with comprehensive validation and monitoring.

        Args:
            chunks: List of (text, embedding) tuples
            random_state: Random seed for reproducibility
            callback: Optional progress callback function

        Returns:
            Processed chunks with hierarchical summaries

        Raises:
            RaptorValidationError: If inputs are invalid
            RaptorResourceError: If resource limits are exceeded
            RaptorError: For other processing errors
        """
        start_time = time.time()

        try:
            # Comprehensive input validation
            self._validate_call_inputs(chunks, random_state)

            # Check resource limits
            self._check_memory_limit()

            # Early return for trivial cases
            if len(chunks) <= 1:
                logging.info("Insufficient chunks for RAPTOR processing")
                return []

            # Filter and validate chunks
            original_count = len(chunks)
            chunks = self._filter_and_validate_chunks(chunks)

            if len(chunks) != original_count:
                logging.info(f"Filtered chunks: {original_count} -> {len(chunks)}")

            if len(chunks) <= 1:
                logging.info("Insufficient valid chunks after filtering")
                return []

            # Initialize processing state
            self._stats['total_chunks_processed'] = len(chunks)
            layers = [(0, len(chunks))]
            start, end = 0, len(chunks)

            # Resource monitoring setup
            max_concurrent_tasks = min(self._max_cluster, 10)  # Limit concurrency
            semaphore = trio.Semaphore(max_concurrent_tasks)

            logging.info(f"Starting RAPTOR processing: {len(chunks)} chunks, "
                        f"max_cluster={self._max_cluster}, max_concurrent={max_concurrent_tasks}")

            return await self._process_chunks_hierarchically(
                chunks, layers, start, end, random_state, callback, semaphore
            )

        except Exception as e:
            self._stats['failed_operations'] += 1
            if isinstance(e, RaptorError):
                raise
            else:
                raise RaptorError(f"Unexpected error in RAPTOR processing: {e}")

        finally:
            # Update processing time
            processing_time = time.time() - start_time
            self._stats['processing_time'] = processing_time

            # Record document processing in monitor
            if self._monitor:
                # Safely get variables that might not be defined if early exception occurred
                chunks_result = locals().get('chunks', [])
                original_count_val = locals().get('original_count', 0)

                summaries_generated = max(0, len(chunks_result) - original_count_val) if chunks_result else 0
                self._monitor.record_document_processed(
                    chunks_count=original_count_val,
                    summaries_count=summaries_generated,
                    processing_time=processing_time
                )

            logging.info(f"RAPTOR processing completed in {processing_time:.2f}s")

    def _validate_call_inputs(self, chunks: List[Tuple[str, np.ndarray]], random_state: int) -> None:
        """Validate inputs to __call__ method."""
        if not isinstance(chunks, list):
            raise RaptorValidationError(f"chunks must be a list, got {type(chunks)}")

        if len(chunks) > self.MAX_TOTAL_CHUNKS:
            raise RaptorValidationError(f"Too many chunks: {len(chunks)} > {self.MAX_TOTAL_CHUNKS}")

        if not isinstance(random_state, int) or random_state < 0:
            raise RaptorValidationError("random_state must be non-negative integer")

    def _filter_and_validate_chunks(self, chunks: List[Tuple[str, np.ndarray]]) -> List[Tuple[str, np.ndarray]]:
        """Filter and validate chunk data."""
        valid_chunks = []

        for i, chunk in enumerate(chunks):
            try:
                if not isinstance(chunk, (list, tuple)) or len(chunk) != 2:
                    logging.warning(f"Skipping chunk {i}: invalid format")
                    continue

                text, embedding = chunk

                if not isinstance(text, str) or not text.strip():
                    logging.warning(f"Skipping chunk {i}: invalid text")
                    continue

                if len(text) > self.MAX_CHUNK_SIZE:
                    logging.warning(f"Skipping chunk {i}: text too large ({len(text)} bytes)")
                    continue

                if not isinstance(embedding, np.ndarray) or embedding.size == 0:
                    logging.warning(f"Skipping chunk {i}: invalid embedding")
                    continue

                if np.any(np.isnan(embedding)) or np.any(np.isinf(embedding)):
                    logging.warning(f"Skipping chunk {i}: embedding contains NaN/inf")
                    continue

                valid_chunks.append((text, embedding))

            except Exception as e:
                logging.warning(f"Error validating chunk {i}: {e}")
                continue

        return valid_chunks

    async def _process_chunks_hierarchically(self, chunks: List[Tuple[str, np.ndarray]],
                                           layers: List[Tuple[int, int]], start: int, end: int,
                                           random_state: int, callback: Optional[callable],
                                           semaphore: trio.Semaphore) -> List[Tuple[str, np.ndarray]]:
        """Process chunks hierarchically with resource management."""

        async def summarize(ck_idx: List[int]):
            """Enhanced summarize function with resource management."""
            nonlocal chunks

            async with semaphore:  # Limit concurrent operations
                try:
                    # Check memory before processing
                    self._check_memory_limit()

                    # Validate cluster indices
                    if not ck_idx or any(i >= len(chunks) or i < 0 for i in ck_idx):
                        raise RaptorValidationError(f"Invalid cluster indices: {ck_idx}")

                    texts = [chunks[i][0] for i in ck_idx]

                    # Calculate safe chunk length
                    available_length = max(self._llm_model.max_length - self._max_token - 100, 100)
                    len_per_chunk = max(1, available_length // len(texts))

                    # Prepare cluster content with safe truncation
                    cluster_content = "\n".join([
                        truncate(t, len_per_chunk) for t in texts
                    ])

                    # Validate cluster content
                    if not cluster_content.strip():
                        raise RaptorValidationError("Empty cluster content after truncation")

                    # Generate summary with rate limiting
                    async with chat_limiter:
                        cnt = await self._chat(
                            "You're a helpful assistant.",
                            [
                                {
                                    "role": "user",
                                    "content": self._prompt.format(
                                        cluster_content=cluster_content
                                    ),
                                }
                            ],
                            {"temperature": 0.3, "max_tokens": self._max_token},
                        )

                    # Clean response
                    cnt = re.sub(
                        r"(······\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?)",
                        "",
                        cnt,
                    )

                    # Validate summary
                    if not cnt.strip():
                        raise RaptorLLMError("Empty summary generated")

                    logging.debug(f"Generated summary ({len(cnt)} chars): {cnt[:100]}...")

                    # Generate embedding for summary
                    embds = await self._embedding_encode(cnt)
                    chunks.append((cnt, embds))

                    logging.debug(f"Successfully processed cluster with {len(ck_idx)} chunks")

                except Exception as e:
                    logging.error(f"Failed to summarize cluster {ck_idx}: {e}")
                    # Don't add failed summaries to chunks
                    raise

        # Main hierarchical processing loop
        labels = []
        layer_count = 0
        max_layers = 10  # Prevent infinite loops

        while end - start > 1 and layer_count < max_layers:
            try:
                layer_count += 1
                logging.info(f"Processing layer {layer_count}: {end - start} chunks")

                # Check memory and time limits
                self._check_memory_limit()

                # Extract embeddings for current layer
                embeddings = np.array([embd for _, embd in chunks[start:end]])

                # Handle simple case with 2 embeddings
                if len(embeddings) == 2:
                    await summarize([start, start + 1])
                    if callback:
                        callback(msg=f"Layer {layer_count}: {end - start} -> {len(chunks) - end}")
                    labels.extend([0, 0])
                    layers.append((end, len(chunks)))
                    start = end
                    end = len(chunks)
                    continue

                # Perform dimensionality reduction with error handling
                try:
                    n_neighbors = max(2, min(int((len(embeddings) - 1) ** 0.8), len(embeddings) - 1))
                    n_components = max(2, min(12, len(embeddings) - 2))

                    reduced_embeddings = umap.UMAP(
                        n_neighbors=n_neighbors,
                        n_components=n_components,
                        metric="cosine",
                        random_state=random_state,
                        verbose=False
                    ).fit_transform(embeddings)

                except Exception as e:
                    logging.warning(f"UMAP failed, using original embeddings: {e}")
                    reduced_embeddings = embeddings

                # Get optimal number of clusters
                n_clusters = self._get_optimal_clusters(reduced_embeddings, random_state)

                # Generate cluster labels
                if n_clusters == 1:
                    lbls = [0 for _ in range(len(reduced_embeddings))]
                else:
                    try:
                        gm = GaussianMixture(
                            n_components=n_clusters,
                            random_state=random_state,
                            max_iter=100,
                            tol=1e-3
                        )
                        gm.fit(reduced_embeddings)
                        probs = gm.predict_proba(reduced_embeddings)

                        # Assign clusters based on threshold
                        lbls = []
                        for prob in probs:
                            cluster_candidates = np.where(prob > self._threshold)[0]
                            if len(cluster_candidates) > 0:
                                lbls.append(cluster_candidates[0])
                            else:
                                lbls.append(np.argmax(prob))  # Fallback to highest probability

                    except Exception as e:
                        logging.warning(f"Clustering failed, using simple assignment: {e}")
                        lbls = list(range(min(n_clusters, len(reduced_embeddings))))

                # Process clusters in parallel with error handling
                cluster_tasks = []

                async with trio.open_nursery() as nursery:
                    for c in range(n_clusters):
                        ck_idx = [i + start for i in range(len(lbls)) if lbls[i] == c]
                        if len(ck_idx) > 0:
                            cluster_tasks.append(ck_idx)
                            nursery.start_soon(summarize, ck_idx)

                # Validate results
                expected_new_chunks = len(cluster_tasks)
                actual_new_chunks = len(chunks) - end

                if actual_new_chunks != expected_new_chunks:
                    logging.warning(f"Cluster processing mismatch: expected {expected_new_chunks}, got {actual_new_chunks}")
                    # Continue processing anyway

                labels.extend(lbls)
                layers.append((end, len(chunks)))

                if callback:
                    callback(msg=f"Layer {layer_count}: {end - start} -> {len(chunks) - end}")

                # Update for next iteration
                start = end
                end = len(chunks)

                logging.info(f"Layer {layer_count} completed: generated {actual_new_chunks} summaries")

            except Exception as e:
                logging.error(f"Error in layer {layer_count}: {e}")
                # Try to continue with remaining chunks
                break

        if layer_count >= max_layers:
            logging.warning(f"Reached maximum layer limit ({max_layers})")

        logging.info(f"RAPTOR processing completed: {layer_count} layers, {len(chunks)} total chunks")
        return chunks
