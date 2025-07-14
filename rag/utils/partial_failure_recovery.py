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

import asyncio
import logging
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable, Dict, List, Optional
import uuid


class BatchItemStatus(Enum):
    """Status of individual batch items."""
    PENDING = "pending"
    PROCESSING = "processing"
    SUCCESS = "success"
    FAILED = "failed"
    RETRYING = "retrying"
    SKIPPED = "skipped"


@dataclass
class BatchItem:
    """Individual item in a batch operation."""
    item_id: str
    data: Any
    status: BatchItemStatus = BatchItemStatus.PENDING
    error: Optional[Exception] = None
    retry_count: int = 0
    processing_time: float = 0.0
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class BatchResult:
    """Result of a batch operation."""
    total_items: int
    successful_items: int
    failed_items: int
    skipped_items: int
    processing_time: float
    items: List[BatchItem]
    partial_success: bool
    
    @property
    def success_rate(self) -> float:
        """Calculate success rate."""
        if self.total_items == 0:
            return 1.0
        return self.successful_items / self.total_items
    
    @property
    def has_failures(self) -> bool:
        """Check if there were any failures."""
        return self.failed_items > 0


@dataclass
class BatchConfig:
    """Configuration for batch processing."""
    max_retries: int = 3
    retry_delay: float = 1.0
    fail_fast: bool = False  # Stop on first failure
    continue_on_error: bool = True  # Continue processing other items on error
    max_concurrent: int = 10  # Maximum concurrent operations
    timeout_per_item: float = 30.0  # Timeout for individual items
    min_success_rate: float = 0.0  # Minimum success rate to consider batch successful


class PartialFailureRecovery:
    """
    Handles partial failure recovery for batch operations.
    
    Allows batch operations to continue processing even when individual items fail,
    with configurable retry logic and failure handling strategies.
    """
    
    def __init__(self, config: Optional[BatchConfig] = None):
        self.config = config or BatchConfig()
        self._stats = {
            "total_batches": 0,
            "successful_batches": 0,
            "partial_batches": 0,
            "failed_batches": 0
        }
    
    async def process_batch_async(
        self,
        items: List[Any],
        processor: Callable,
        item_id_func: Optional[Callable[[Any], str]] = None,
        progress_callback: Optional[Callable[[int, int], None]] = None
    ) -> BatchResult:
        """
        Process a batch of items asynchronously with partial failure recovery.
        
        Args:
            items: List of items to process
            processor: Async function to process each item
            item_id_func: Function to generate ID for each item
            progress_callback: Optional callback for progress updates
            
        Returns:
            BatchResult with processing results
        """
        start_time = time.time()
        
        # Create batch items
        batch_items = []
        for i, item in enumerate(items):
            item_id = item_id_func(item) if item_id_func else f"item_{i}"
            batch_items.append(BatchItem(item_id=item_id, data=item))
        
        # Process items with concurrency control
        semaphore = asyncio.Semaphore(self.config.max_concurrent)
        tasks = []
        
        for batch_item in batch_items:
            task = asyncio.create_task(
                self._process_single_item_async(batch_item, processor, semaphore)
            )
            tasks.append(task)
        
        # Wait for all tasks to complete
        completed = 0
        for task in asyncio.as_completed(tasks):
            await task
            completed += 1
            
            if progress_callback:
                progress_callback(completed, len(batch_items))
            
            # Check fail-fast condition
            if self.config.fail_fast:
                failed_count = sum(1 for item in batch_items if item.status == BatchItemStatus.FAILED)
                if failed_count > 0:
                    # Cancel remaining tasks
                    for remaining_task in tasks:
                        if not remaining_task.done():
                            remaining_task.cancel()
                    break
        
        # Calculate results
        processing_time = time.time() - start_time
        result = self._create_batch_result(batch_items, processing_time)
        
        # Update statistics
        self._update_stats(result)
        
        logging.info(
            f"Batch processing completed: {result.successful_items}/{result.total_items} successful "
            f"({result.success_rate:.2%}) in {processing_time:.2f}s"
        )
        
        return result
    
    def process_batch_sync(
        self,
        items: List[Any],
        processor: Callable,
        item_id_func: Optional[Callable[[Any], str]] = None,
        progress_callback: Optional[Callable[[int, int], None]] = None
    ) -> BatchResult:
        """
        Process a batch of items synchronously with partial failure recovery.
        
        Args:
            items: List of items to process
            processor: Function to process each item
            item_id_func: Function to generate ID for each item
            progress_callback: Optional callback for progress updates
            
        Returns:
            BatchResult with processing results
        """
        start_time = time.time()
        
        # Create batch items
        batch_items = []
        for i, item in enumerate(items):
            item_id = item_id_func(item) if item_id_func else f"item_{i}"
            batch_items.append(BatchItem(item_id=item_id, data=item))
        
        # Process items sequentially
        for i, batch_item in enumerate(batch_items):
            self._process_single_item_sync(batch_item, processor)
            
            if progress_callback:
                progress_callback(i + 1, len(batch_items))
            
            # Check fail-fast condition
            if self.config.fail_fast and batch_item.status == BatchItemStatus.FAILED:
                # Mark remaining items as skipped
                for remaining_item in batch_items[i + 1:]:
                    remaining_item.status = BatchItemStatus.SKIPPED
                break
        
        # Calculate results
        processing_time = time.time() - start_time
        result = self._create_batch_result(batch_items, processing_time)
        
        # Update statistics
        self._update_stats(result)
        
        logging.info(
            f"Batch processing completed: {result.successful_items}/{result.total_items} successful "
            f"({result.success_rate:.2%}) in {processing_time:.2f}s"
        )
        
        return result
    
    async def _process_single_item_async(
        self,
        batch_item: BatchItem,
        processor: Callable,
        semaphore: asyncio.Semaphore
    ):
        """Process a single item asynchronously with retry logic."""
        async with semaphore:
            await self._process_with_retry_async(batch_item, processor)
    
    def _process_single_item_sync(self, batch_item: BatchItem, processor: Callable):
        """Process a single item synchronously with retry logic."""
        self._process_with_retry_sync(batch_item, processor)
    
    async def _process_with_retry_async(self, batch_item: BatchItem, processor: Callable):
        """Process item with retry logic (async)."""
        for attempt in range(self.config.max_retries + 1):
            batch_item.status = BatchItemStatus.PROCESSING if attempt == 0 else BatchItemStatus.RETRYING
            batch_item.retry_count = attempt
            
            start_time = time.time()
            
            try:
                # Execute with timeout
                result = await asyncio.wait_for(
                    processor(batch_item.data),
                    timeout=self.config.timeout_per_item
                )
                
                batch_item.processing_time = time.time() - start_time
                batch_item.status = BatchItemStatus.SUCCESS
                batch_item.metadata["result"] = result
                return
                
            except Exception as e:
                batch_item.processing_time = time.time() - start_time
                batch_item.error = e
                
                if attempt < self.config.max_retries:
                    logging.warning(
                        f"Item {batch_item.item_id} failed (attempt {attempt + 1}): {e}. Retrying..."
                    )
                    if self.config.retry_delay > 0:
                        await asyncio.sleep(self.config.retry_delay * (2 ** attempt))  # Exponential backoff
                else:
                    logging.error(f"Item {batch_item.item_id} failed after {attempt + 1} attempts: {e}")
                    batch_item.status = BatchItemStatus.FAILED
                    return
    
    def _process_with_retry_sync(self, batch_item: BatchItem, processor: Callable):
        """Process item with retry logic (sync)."""
        for attempt in range(self.config.max_retries + 1):
            batch_item.status = BatchItemStatus.PROCESSING if attempt == 0 else BatchItemStatus.RETRYING
            batch_item.retry_count = attempt
            
            start_time = time.time()
            
            try:
                result = processor(batch_item.data)
                
                batch_item.processing_time = time.time() - start_time
                batch_item.status = BatchItemStatus.SUCCESS
                batch_item.metadata["result"] = result
                return
                
            except Exception as e:
                batch_item.processing_time = time.time() - start_time
                batch_item.error = e
                
                if attempt < self.config.max_retries:
                    logging.warning(
                        f"Item {batch_item.item_id} failed (attempt {attempt + 1}): {e}. Retrying..."
                    )
                    if self.config.retry_delay > 0:
                        time.sleep(self.config.retry_delay * (2 ** attempt))  # Exponential backoff
                else:
                    logging.error(f"Item {batch_item.item_id} failed after {attempt + 1} attempts: {e}")
                    batch_item.status = BatchItemStatus.FAILED
                    return
    
    def _create_batch_result(self, batch_items: List[BatchItem], processing_time: float) -> BatchResult:
        """Create batch result from processed items."""
        successful_items = sum(1 for item in batch_items if item.status == BatchItemStatus.SUCCESS)
        failed_items = sum(1 for item in batch_items if item.status == BatchItemStatus.FAILED)
        skipped_items = sum(1 for item in batch_items if item.status == BatchItemStatus.SKIPPED)
        
        partial_success = (
            successful_items > 0 and 
            failed_items > 0 and 
            successful_items / len(batch_items) >= self.config.min_success_rate
        )
        
        return BatchResult(
            total_items=len(batch_items),
            successful_items=successful_items,
            failed_items=failed_items,
            skipped_items=skipped_items,
            processing_time=processing_time,
            items=batch_items,
            partial_success=partial_success
        )
    
    def _update_stats(self, result: BatchResult):
        """Update processing statistics."""
        self._stats["total_batches"] += 1
        
        if result.failed_items == 0:
            self._stats["successful_batches"] += 1
        elif result.successful_items > 0:
            self._stats["partial_batches"] += 1
        else:
            self._stats["failed_batches"] += 1
    
    def get_stats(self) -> Dict[str, Any]:
        """Get processing statistics."""
        return self._stats.copy()
    
    def reset_stats(self):
        """Reset processing statistics."""
        self._stats = {
            "total_batches": 0,
            "successful_batches": 0,
            "partial_batches": 0,
            "failed_batches": 0
        }


# Convenience functions for common batch operations
async def process_chunks_with_recovery(
    chunks: List[Any],
    processor: Callable,
    config: Optional[BatchConfig] = None,
    progress_callback: Optional[Callable[[int, int], None]] = None
) -> BatchResult:
    """Process document chunks with partial failure recovery."""
    recovery = PartialFailureRecovery(config)
    return await recovery.process_batch_async(
        chunks, processor, lambda chunk: chunk.get("id", str(uuid.uuid4())), progress_callback
    )


def process_items_with_recovery(
    items: List[Any],
    processor: Callable,
    config: Optional[BatchConfig] = None,
    progress_callback: Optional[Callable[[int, int], None]] = None
) -> BatchResult:
    """Process items synchronously with partial failure recovery."""
    recovery = PartialFailureRecovery(config)
    return recovery.process_batch_sync(items, processor, None, progress_callback)
