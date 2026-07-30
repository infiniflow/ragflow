---
sidebar_position: 10
title: Run Retrieval Test
sidebar_label: Run Retrieval Test
slug: /run_retrieval_test
sidebar_custom_props: {
  categoryIcon: LucideTextSearch
}
---
# Run Retrieval Test

Conduct a retrieval test on your dataset to check whether the intended chunks can be retrieved.

---

After your files are uploaded and parsed, it is recommended that you run a retrieval test before proceeding with the chat assistant configuration. Running a retrieval test is *not* an unnecessary or superfluous step at all! Just like fine-tuning a precision instrument, RAGFlow requires careful tuning to deliver optimal question answering performance. Your dataset settings, chat assistant configurations, and the specified large and small models can all significantly impact the final results. Running a retrieval test verifies whether the intended chunks can be recovered, allowing you to quickly identify areas for improvement or pinpoint any issue that needs addressing. For instance, when debugging your question answering system, if you know that the correct chunks can be retrieved, you can focus your efforts elsewhere. For example, in issue [#5627](https://github.com/infiniflow/ragflow/issues/5627), the problem was found to be due to the LLM's limitations.

During a retrieval test, chunks created from your specified chunking method are retrieved using a hybrid search. This search combines weighted keyword similarity with either weighted vector cosine similarity or a weighted reranking score, depending on your settings:

- If no rerank model is selected, weighted keyword similarity will be combined with weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

In contrast, chunks created from [knowledge graph construction](./advanced/construct_knowledge_graph.md) are retrieved solely using vector cosine similarity.

## Prerequisites

- Your files are uploaded and successfully parsed before running a retrieval test.
- A knowledge graph must be successfully built before enabling **Use knowledge graph**.

## Configurations

### Similarity Threshold

This sets the bar for retrieving chunks: chunks with similarities below the threshold will be filtered out. By default, the threshold is set to 0.2. This means that only chunks with hybrid similarity score of 20 or higher will be retrieved.

### Vector Similarity Weight

This sets the weight of vector similarity in the composite similarity score, whether used with vector cosine similarity or a reranking score. By default, it is set to 0.3, making the weight of the other component 0.7 (1 - 0.3).

### Rerank Model

- If left empty, RAGFlow will use a combination of weighted keyword similarity and weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

:::danger IMPORTANT
Using a rerank model will significantly increase the time to receive a response.
:::

### Use Knowledge Graph

In a knowledge graph, an entity description, a relationship description, or a community report each exists as an independent chunk. This switch indicates whether to add these chunks to the retrieval.

The switch is disabled by default. When enabled, RAGFlow performs the following during a retrieval test:

1. Extract entities and entity types from your query using the LLM.
2. Retrieve top N entities from the graph based on their PageRank values, using the extracted entity types.
3. Find similar entities and their N-hop relationships from the graph using the embeddings of the extracted query entities.
4. Retrieve similar relationships from the graph using the query embedding.
5. Rank these retrieved entities and relationships by multiplying each one's PageRank value with its similarity score to the query, returning the top n as the final retrieval.
6. Retrieve the report for the community involving the most entities in the final retrieval.  
   *The retrieved entity descriptions, relationship descriptions, and the top 1 community report are sent to the LLM for content generation.*

:::danger IMPORTANT
Using a knowledge graph in a retrieval test will significantly increase the time to receive a response.
:::

### Cross-Language Search

To perform a [cross-language search](../../references/glossary.mdx#cross-language-search), select one or more target languages from the dropdown menu. The system’s default chat model will then translate your query entered in the Test text field into the selected target language(s). This translation ensures accurate semantic matching across languages, allowing you to retrieve relevant results regardless of language differences.

:::tip NOTE
- When selecting target languages, please ensure that these languages are present in the dataset to guarantee an effective search.
- If no target language is selected, the system will search only in the language of your query, which may cause relevant information in other languages to be missed.
:::

### Test Text

This field is where you put in your testing query.

## Procedure

1. Navigate to the **Retrieval testing** page of your dataset, enter your query in **Test text**, and click **Testing** to run the test.
2. If the results are unsatisfactory, tune the options listed in the Configuration section and rerun the test.

   A retrieval test without a knowledge graph uses hybrid search, combining weighted keyword similarity and weighted vector cosine similarity.

   A retrieval test using a knowledge graph uses vector similarity for knowledge graph-generated chunks.

:::caution WARNING
If you have adjusted the default settings, such as keyword similarity weight or similarity threshold, to achieve the optimal results, be aware that these changes will not be automatically saved. You must apply them to your chat assistant settings or the **Retrieval** agent component settings.
:::

## Frequently Asked Questions

### Is an LLM Used When the Use Knowledge Graph Switch Is Enabled?

Yes, your LLM will be involved to analyze your query and extract the related entities and relationship from the knowledge graph. This also explains why additional tokens and time will be consumed.
