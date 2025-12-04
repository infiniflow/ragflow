#!/usr/bin/env python3
#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
Complete working example demonstrating checkpoint/resume functionality.

This example shows:
1. Creating a checkpoint for a RAPTOR task
2. Processing documents with progress tracking
3. Simulating a crash and resume
4. Handling failures with retry logic
5. Pausing and resuming tasks

Run this example:
    python examples/checkpoint_resume_demo.py
"""

import sys
import time
import random

# Add parent directory to path for imports
sys.path.insert(0, '/root/ragflow')

from api.db.services.checkpoint_service import CheckpointService
from api.db.db_models import DB


def print_section(title: str):
    """Print a section header"""
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}\n")


def print_status(checkpoint_id: str):
    """Print current checkpoint status"""
    status = CheckpointService.get_checkpoint_status(checkpoint_id)
    if status:
        print(f"Status: {status['status']}")
        print(f"Progress: {status['progress']*100:.1f}%")
        print(f"Completed: {status['completed_documents']}/{status['total_documents']}")
        print(f"Failed: {status['failed_documents']}")
        print(f"Pending: {status['pending_documents']}")
        print(f"Tokens: {status['token_count']:,}")


def simulate_document_processing(doc_id: str, should_fail: bool = False) -> tuple:
    """
    Simulate processing a single document.
    
    Returns:
        (success, token_count, chunks, error)
    """
    print(f"  Processing {doc_id}...", end=" ", flush=True)
    time.sleep(0.5)  # Simulate processing time
    
    if should_fail:
        print("❌ FAILED")
        return (False, 0, 0, "Simulated API timeout")
    
    # Simulate successful processing
    token_count = random.randint(1000, 3000)
    chunks = random.randint(30, 90)
    print(f"✓ Done ({token_count} tokens, {chunks} chunks)")
    return (True, token_count, chunks, None)


def example_1_basic_checkpoint():
    """Example 1: Basic checkpoint creation and completion"""
    print_section("Example 1: Basic Checkpoint Creation")
    
    # Create checkpoint for 5 documents
    doc_ids = [f"doc_{i}" for i in range(1, 6)]
    
    print(f"Creating checkpoint for {len(doc_ids)} documents...")
    checkpoint = CheckpointService.create_checkpoint(
        task_id="demo_task_001",
        task_type="raptor",
        doc_ids=doc_ids,
        config={"max_cluster": 64, "threshold": 0.5}
    )
    
    print(f"✓ Checkpoint created: {checkpoint.id}\n")
    print_status(checkpoint.id)
    
    # Process all documents
    print("\nProcessing documents:")
    for doc_id in doc_ids:
        success, tokens, chunks, error = simulate_document_processing(doc_id)
        
        if success:
            CheckpointService.save_document_completion(
                checkpoint.id,
                doc_id,
                token_count=tokens,
                chunks=chunks
            )
    
    print("\n✓ All documents processed!")
    print_status(checkpoint.id)
    
    return checkpoint.id


def example_2_crash_and_resume():
    """Example 2: Simulating crash and resume"""
    print_section("Example 2: Crash and Resume")
    
    # Create checkpoint for 10 documents
    doc_ids = [f"doc_{i}" for i in range(1, 11)]
    
    print(f"Creating checkpoint for {len(doc_ids)} documents...")
    checkpoint = CheckpointService.create_checkpoint(
        task_id="demo_task_002",
        task_type="raptor",
        doc_ids=doc_ids,
        config={}
    )
    
    print(f"✓ Checkpoint created: {checkpoint.id}\n")
    
    # Process first 4 documents
    print("Processing first batch (4 documents):")
    for doc_id in doc_ids[:4]:
        success, tokens, chunks, error = simulate_document_processing(doc_id)
        CheckpointService.save_document_completion(
            checkpoint.id, doc_id, tokens, chunks
        )
    
    print("\n💥 CRASH! System went down...\n")
    time.sleep(1)
    
    # Simulate restart - retrieve checkpoint
    print("🔄 System restarted. Resuming from checkpoint...")
    resumed_checkpoint = CheckpointService.get_by_task_id("demo_task_002")
    
    if resumed_checkpoint:
        print(f"✓ Found checkpoint: {resumed_checkpoint.id}")
        print_status(resumed_checkpoint.id)
        
        # Get pending documents (should skip completed ones)
        pending = CheckpointService.get_pending_documents(resumed_checkpoint.id)
        print(f"\n📋 Resuming with {len(pending)} pending documents:")
        print(f"   {', '.join(pending)}\n")
        
        # Continue processing remaining documents
        print("Processing remaining documents:")
        for doc_id in pending:
            success, tokens, chunks, error = simulate_document_processing(doc_id)
            CheckpointService.save_document_completion(
                resumed_checkpoint.id, doc_id, tokens, chunks
            )
        
        print("\n✓ All documents completed after resume!")
        print_status(resumed_checkpoint.id)
    
    return checkpoint.id


def example_3_failure_and_retry():
    """Example 3: Handling failures with retry logic"""
    print_section("Example 3: Failure Handling and Retry")
    
    # Create checkpoint
    doc_ids = [f"doc_{i}" for i in range(1, 6)]
    
    checkpoint = CheckpointService.create_checkpoint(
        task_id="demo_task_003",
        task_type="raptor",
        doc_ids=doc_ids,
        config={}
    )
    
    print(f"Checkpoint created: {checkpoint.id}\n")
    
    # Process documents with one failure
    print("Processing documents (doc_3 will fail):")
    for doc_id in doc_ids:
        should_fail = (doc_id == "doc_3")
        success, tokens, chunks, error = simulate_document_processing(doc_id, should_fail)
        
        if success:
            CheckpointService.save_document_completion(
                checkpoint.id, doc_id, tokens, chunks
            )
        else:
            CheckpointService.save_document_failure(
                checkpoint.id, doc_id, error
            )
    
    print("\n📊 Current status:")
    print_status(checkpoint.id)
    
    # Check failed documents
    failed = CheckpointService.get_failed_documents(checkpoint.id)
    print(f"\n❌ Failed documents: {len(failed)}")
    for fail in failed:
        print(f"   - {fail['doc_id']}: {fail['error']} (retry #{fail['retry_count']})")
    
    # Retry failed documents
    print("\n🔄 Retrying failed documents...")
    for fail in failed:
        doc_id = fail['doc_id']
        
        if CheckpointService.should_retry(checkpoint.id, doc_id, max_retries=3):
            print(f"  Retrying {doc_id}...")
            CheckpointService.reset_document_for_retry(checkpoint.id, doc_id)
            
            # Retry (this time it succeeds)
            success, tokens, chunks, error = simulate_document_processing(doc_id, should_fail=False)
            CheckpointService.save_document_completion(
                checkpoint.id, doc_id, tokens, chunks
            )
    
    print("\n✓ All documents completed after retry!")
    print_status(checkpoint.id)
    
    return checkpoint.id


def example_4_pause_and_resume():
    """Example 4: Pausing and resuming a task"""
    print_section("Example 4: Pause and Resume")
    
    # Create checkpoint
    doc_ids = [f"doc_{i}" for i in range(1, 8)]
    
    checkpoint = CheckpointService.create_checkpoint(
        task_id="demo_task_004",
        task_type="raptor",
        doc_ids=doc_ids,
        config={}
    )
    
    print(f"Checkpoint created: {checkpoint.id}\n")
    
    # Process first 3 documents
    print("Processing first 3 documents:")
    for doc_id in doc_ids[:3]:
        success, tokens, chunks, error = simulate_document_processing(doc_id)
        CheckpointService.save_document_completion(
            checkpoint.id, doc_id, tokens, chunks
        )
    
    # Pause
    print("\n⏸️  Pausing task...")
    CheckpointService.pause_checkpoint(checkpoint.id)
    print(f"   Is paused: {CheckpointService.is_paused(checkpoint.id)}")
    print_status(checkpoint.id)
    
    time.sleep(1)
    
    # Resume
    print("\n▶️  Resuming task...")
    CheckpointService.resume_checkpoint(checkpoint.id)
    print(f"   Is paused: {CheckpointService.is_paused(checkpoint.id)}")
    
    # Continue processing
    pending = CheckpointService.get_pending_documents(checkpoint.id)
    print(f"\n📋 Continuing with {len(pending)} pending documents:")
    for doc_id in pending:
        success, tokens, chunks, error = simulate_document_processing(doc_id)
        CheckpointService.save_document_completion(
            checkpoint.id, doc_id, tokens, chunks
        )
    
    print("\n✓ Task completed!")
    print_status(checkpoint.id)
    
    return checkpoint.id


def main():
    """Run all examples"""
    print("\n" + "="*60)
    print("  RAGFlow Checkpoint/Resume Demo")
    print("  Demonstrating task checkpoint and resume functionality")
    print("="*60)
    
    try:
        # Initialize database connection
        print("\n🔌 Connecting to database...")
        DB.connect(reuse_if_open=True)
        print("✓ Database connected\n")
        
        # Run examples
        example_1_basic_checkpoint()
        example_2_crash_and_resume()
        example_3_failure_and_retry()
        example_4_pause_and_resume()
        
        print_section("Demo Complete!")
        print("✓ All examples completed successfully")
        print("\nKey features demonstrated:")
        print("  1. ✓ Checkpoint creation and tracking")
        print("  2. ✓ Crash recovery and resume")
        print("  3. ✓ Failure handling with retry logic")
        print("  4. ✓ Pause and resume functionality")
        print("  5. ✓ Progress tracking and status reporting")
        
    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
    finally:
        DB.close()


if __name__ == "__main__":
    main()
