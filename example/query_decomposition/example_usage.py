#!/usr/bin/env python3
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
Query Decomposition Retrieval - Example Usage

This example demonstrates how to use the QueryDecompositionRetrieval component
for advanced retrieval with automatic query decomposition and intelligent reranking.

The component is particularly useful for:
- Complex queries with multiple aspects
- Comparison questions
- Research queries requiring comprehensive coverage
"""

import sys
import os

# Add parent directory to path for imports
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '../..')))

from agent.tools.query_decomposition_retrieval import (
    QueryDecompositionRetrieval,
    QueryDecompositionRetrievalParam
)


def example_basic_usage():
    """
    Example 1: Basic Usage
    
    This example shows the simplest way to use query decomposition retrieval
    with default settings.
    """
    print("="*80)
    print("Example 1: Basic Usage with Default Settings")
    print("="*80)
    
    # Create retrieval component
    retrieval = QueryDecompositionRetrieval()
    
    # Configure parameters
    params = QueryDecompositionRetrievalParam()
    params.enable_decomposition = True  # Enable query decomposition
    params.kb_ids = ["your-knowledge-base-id"]  # Replace with actual KB ID
    params.top_n = 8  # Return top 8 results
    
    retrieval._param = params
    
    # Example query: Complex comparison question
    query = "Compare machine learning and deep learning, and explain their applications"
    
    print(f"\nQuery: {query}")
    print("\nProcessing...")
    print("- Decomposing query into sub-questions")
    print("- Retrieving chunks for each sub-question concurrently")
    print("- Deduplicating and reranking globally with LLM scoring")
    print("\nResults would be returned via retrieval.invoke(query=query)")
    print("\n" + "="*80 + "\n")


def example_custom_configuration():
    """
    Example 2: Custom Configuration
    
    This example shows how to customize the retrieval behavior with
    different settings for specific use cases.
    """
    print("="*80)
    print("Example 2: Custom Configuration for High-Precision Research")
    print("="*80)
    
    # Create retrieval component
    retrieval = QueryDecompositionRetrieval()
    
    # Configure for high-precision research mode
    params = QueryDecompositionRetrievalParam()
    params.enable_decomposition = True
    params.kb_ids = ["research-kb-1", "research-kb-2"]
    
    # High-precision settings
    params.score_fusion_weight = 0.9  # Trust LLM scores more (90% LLM, 10% vector)
    params.max_decomposition_count = 4  # Allow up to 4 sub-questions
    params.top_n = 10  # Return more results for comprehensive coverage
    params.similarity_threshold = 0.3  # Higher threshold for quality
    
    retrieval._param = params
    
    # Example query: Multi-faceted research question
    query = "Explain the causes, key events, consequences, and historical significance of World War II"
    
    print(f"\nQuery: {query}")
    print("\nConfiguration:")
    print(f"  - Score fusion weight: {params.score_fusion_weight} (trusts LLM highly)")
    print(f"  - Max sub-questions: {params.max_decomposition_count}")
    print(f"  - Results to return: {params.top_n}")
    print(f"  - Similarity threshold: {params.similarity_threshold}")
    
    print("\nExpected sub-questions:")
    print("  1. What were the main causes that led to World War II?")
    print("  2. What were the most significant events during World War II?")
    print("  3. What were the major consequences of World War II?")
    print("  4. What is the historical significance of World War II?")
    
    print("\n" + "="*80 + "\n")


def example_custom_prompts():
    """
    Example 3: Custom Prompts
    
    This example shows how to provide custom prompts for query decomposition
    and LLM-based reranking.
    """
    print("="*80)
    print("Example 3: Custom Prompts for Domain-Specific Retrieval")
    print("="*80)
    
    # Create retrieval component
    retrieval = QueryDecompositionRetrieval()
    
    params = QueryDecompositionRetrievalParam()
    params.enable_decomposition = True
    params.kb_ids = ["medical-knowledge-base"]
    
    # Custom decomposition prompt for medical domain
    params.decomposition_prompt = """You are a medical information expert. 
Break down this medical query into {max_count} focused sub-questions that cover:
1. Definition/Overview
2. Symptoms/Diagnosis  
3. Treatment/Management

Original Query: {original_query}

Output ONLY a JSON array: ["sub-question 1", "sub-question 2", "sub-question 3"]

Sub-questions:"""
    
    # Custom reranking prompt for medical relevance
    params.reranking_prompt = """You are a medical information relevance expert.

Query: {query}
Medical Information Chunk: {chunk_text}

Rate the relevance of this medical information (1-10):
- 9-10: Contains direct medical answer with clinical details
- 7-8: Contains relevant medical information
- 5-6: Contains related context
- 3-4: Tangentially related
- 1-2: Not medically relevant

Output JSON: {{"score": <1-10>, "reason": "<brief medical justification>"}}

Assessment:"""
    
    retrieval._param = params
    
    # Example medical query
    query = "What is type 2 diabetes and how is it treated?"
    
    print(f"\nQuery: {query}")
    print("\nCustom Prompts:")
    print("  ✓ Domain-specific decomposition (medical focus)")
    print("  ✓ Domain-specific reranking (clinical relevance)")
    
    print("\nExpected sub-questions:")
    print("  1. What is type 2 diabetes? (Definition/Overview)")
    print("  2. What are the symptoms and how is type 2 diabetes diagnosed?")
    print("  3. What are the treatment options and management strategies for type 2 diabetes?")
    
    print("\n" + "="*80 + "\n")


def example_fast_mode():
    """
    Example 4: Fast Response Mode
    
    This example shows configuration for quick responses when speed is
    more important than comprehensive coverage.
    """
    print("="*80)
    print("Example 4: Fast Response Mode for Interactive Applications")
    print("="*80)
    
    # Create retrieval component
    retrieval = QueryDecompositionRetrieval()
    
    # Configure for fast response
    params = QueryDecompositionRetrievalParam()
    params.enable_decomposition = True
    params.kb_ids = ["faq-knowledge-base"]
    
    # Fast mode settings
    params.max_decomposition_count = 2  # Fewer sub-questions for speed
    params.enable_concurrency = True  # Parallel processing enabled
    params.top_n = 5  # Fewer results for faster processing
    params.top_k = 512  # Smaller initial candidate pool
    params.score_fusion_weight = 0.6  # Balanced scoring
    
    retrieval._param = params
    
    # Example query
    query = "How do I reset my password and update my email?"
    
    print(f"\nQuery: {query}")
    print("\nConfiguration for Speed:")
    print(f"  - Max sub-questions: {params.max_decomposition_count} (faster)")
    print(f"  - Concurrent retrieval: {params.enable_concurrency}")
    print(f"  - Results: {params.top_n} (quick response)")
    print(f"  - Initial candidates: {params.top_k} (smaller pool)")
    
    print("\nExpected sub-questions:")
    print("  1. How do I reset my password?")
    print("  2. How do I update my email address?")
    
    print("\nExpected performance:")
    print("  ⚡ Fast query decomposition (2 sub-queries only)")
    print("  ⚡ Parallel retrieval for both sub-queries")
    print("  ⚡ Quick LLM scoring (5 chunks only)")
    print("  ⚡ Total time: ~1-2 seconds")
    
    print("\n" + "="*80 + "\n")


def example_comparison_with_direct_retrieval():
    """
    Example 5: Comparison with Direct Retrieval
    
    This example compares query decomposition retrieval with standard
    direct retrieval to show the benefits.
    """
    print("="*80)
    print("Example 5: Comparison - Decomposition vs. Direct Retrieval")
    print("="*80)
    
    query = "Compare Python and JavaScript for web development"
    
    print(f"\nQuery: {query}\n")
    
    print("Approach 1: Direct Retrieval (decomposition disabled)")
    print("-" * 60)
    print("  Process:")
    print("    1. Single vector search for entire query")
    print("    2. Return top-N most similar chunks")
    print("  ")
    print("  Potential Issues:")
    print("    ⚠️  May favor one language over the other in results")
    print("    ⚠️  May miss important aspects of comparison")
    print("    ⚠️  Limited coverage of both technologies")
    print()
    
    print("Approach 2: Query Decomposition Retrieval (enabled)")
    print("-" * 60)
    print("  Process:")
    print("    1. Decompose into sub-questions:")
    print("       - 'What are Python's strengths for web development?'")
    print("       - 'What are JavaScript's strengths for web development?'")
    print("       - 'What are key differences between Python and JavaScript?'")
    print("    2. Retrieve chunks for each sub-question concurrently")
    print("    3. Deduplicate across all results")
    print("    4. LLM scores each chunk's relevance to original query")
    print("    5. Global ranking and selection of top-N")
    print("  ")
    print("  Benefits:")
    print("    ✅ Balanced coverage of both languages")
    print("    ✅ Comprehensive comparison information")
    print("    ✅ No duplicate chunks across aspects")
    print("    ✅ Intelligent relevance scoring")
    
    print("\n" + "="*80 + "\n")


def main():
    """Run all examples."""
    print("\n")
    print("╔" + "="*78 + "╗")
    print("║" + " " * 20 + "Query Decomposition Retrieval Examples" + " " * 20 + "║")
    print("╚" + "="*78 + "╝")
    print()
    
    # Run all examples
    example_basic_usage()
    example_custom_configuration()
    example_custom_prompts()
    example_fast_mode()
    example_comparison_with_direct_retrieval()
    
    print("="*80)
    print("Examples Complete!")
    print("="*80)
    print()
    print("Next Steps:")
    print("1. Replace 'your-knowledge-base-id' with actual KB IDs")
    print("2. Integrate into your agent workflow")
    print("3. Customize prompts for your domain")
    print("4. Tune score_fusion_weight based on results")
    print("5. Monitor performance and adjust settings")
    print()
    print("Documentation: docs/guides/query_decomposition_retrieval.md")
    print("="*80)
    print()


if __name__ == "__main__":
    main()

