---
sidebar_position: -4
slug: /configure_child_chunking_strategy
sidebar_custom_props: {
  categoryIcon: LucideGroup
}
---
# Configure child chunking strategy

Set parent-child chunking strategy to improve retrieval.

---

A persistent challenge in practical RAG applications lies in a structural tension within the traditional "chunk-embed-retrieve" pipeline: a single text chunk is tasked with both semantic matching (recall) and contextual understanding (utilization)—two inherently conflicting objectives. Recall demands fine-grained, precise chunks, while answer generation requires coherent, informationally complete context.

To resolve this tension, RAGFlow previously introduced the Table of Contents (TOC) enhancement feature, which uses a large language model (LLM) to generate document structure and automatically supplements missing context during retrieval based on that TOC. In version 0.23.0, this capability has been systematically integrated into the Ingestion Pipeline, and a novel parent-child chunking mechanism has been introduced.

Under this mechanism, a document is first segmented into larger parent chunks, each maintaining a relatively complete semantic unit to ensure logical and background integrity. Each parent chunk can then be further subdivided into multiple child chunks for precise recall. During retrieval, the system first locates the most relevant text segments based on the child chunks while automatically associating and recalling their parent chunk. This approach maintains high recall relevance while providing ample semantic background for the generation phase.

For instance, when processing a *Compliance Handbook*, a user query about "liability for breach" might precisely retrieve a child chunk stating, "The penalty for breach is 20% of the total contract value," but without context, it cannot clarify whether this clause applies to "minor breach" or "material breach." Leveraging the parent-child chunking mechanism, the system returns this child chunk along with its parent chunk, which contains the complete section of the clause. This allows the LLM to make accurate judgments based on broader context, avoiding misinterpretation.

Through this dual-layer structure of "precise localization + contextual supplementation," RAGFlow ensures retrieval accuracy while significantly enhancing the reliability and completeness of generated answers.


## Procedure

1. On your dataset's **Configuration** page, find the **Child chunk are used for retrieval** toggle:

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/child_chunking.png)


2. Set the delimiter for child chunks.

3. This configuration applies to the **Chunker** component when it comes to ingestion pipeline settings:

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/child_chunking_parser.png)