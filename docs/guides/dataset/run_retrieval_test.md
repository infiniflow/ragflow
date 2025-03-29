---
sidebar_position: 10
slug: /run_retrieval_test
---

# Run retrieval test

Conduct a retrieval test on your knowledge base to check whether the intended chunks can be retrieved.

---

After your files are uploaded and parsed, it is recommended that you run a retrieval test before proceeding with the chat assistant configuration. Running a retrieval test is *not* an unnecessary or superfluous step at all! Just like fine-tuning a precision instrument, RAGFlow requires careful tuning to deliver optimal question answering performance. Your knowledge base settings, chat assistant configurations, and the specified large and small models can all significantly impact the final results. Running a retrieval test verifies whether the intended chunks can be recovered, allowing you to quickly identify areas for improvement or pinpoint any issue that needs addressing. For instance, when debugging your question answering system, if you know that the correct chunks can be retrieved, you can focus your efforts elsewhere. For example, in issue [#5627](https://github.com/infiniflow/ragflow/issues/5627), the problem was found to be due to the LLM's limitations.

During a retrieval test, chunks created from your specified chunk method are retrieved using a hybrid search. This search combines weighted keyword similarity with either weighted vector cosine similarity or a weighted reranking score, depending on your settings:

- If no rerank model is selected, weighted keyword similarity will be combined with weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

In contrast, chunks created from [knowledge graph construction](./construct_knowledge_graph.md) are retrieved solely using vector cosine similarity.

## Prerequisites

- Your files are uploaded and successfully parsed before running a retrieval test.
- A knowledge graph must be successfully built before enabling **Use knowledge graph**.

## Configurations

### Similarity threshold

This sets the bar for retrieving chunks: chunks with similarities below the threshold will be filtered out. By default, the threshold is set to 0.2. This means that only chunks with hybrid similarity score of 20 or higher will be retrieved.

### Keyword similarity weight

This sets the weight of keyword similarity in the combined similarity score, whether used with vector cosine similarity or a reranking score. By default, it is set to 0.7, making the weight of the other component 0.3 (1 - 0.7).

### Rerank model

- If left empty, RAGFlow will use a combination of weighted keyword similarity and weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

:::danger IMPORTANT
Using a rerank model will significantly increase the time to receive a response.
:::

### Use knowledge graph

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

### Test text

This field is where you put in your testing query.

## Procedure

1. Navigate to the **Retrieval testing** page of your knowledge base, enter your query in **Test text**, and click **Testing** to run the test.
2. If the results are unsatisfactory, tune the options listed in the Configuration section and rerun the test.

   *The following is a screenshot of a retrieval test conducted without using knowledge graph. It demonstrates a hybrid search combining weighted keyword similarity and weighted vector cosine similarity. The overall hybrid similarity score is 28.56, calculated as 25.17 (term similarity score) x 0.7 + 36.49 (vector similarity score) x 0.3:*  
   ![Image](https://github.com/user-attachments/assets/541554d4-3f3e-44e1-954b-0ae77d7372c6)

   *The following is a screenshot of a retrieval test conducted using a knowledge graph. It shows that only vector similarity is used for knowledge graph-generated chunks:*  
   ![Image](https://github.com/user-attachments/assets/30a03091-0f7b-4058-901a-f4dc5ca5aa6b)

:::caution WARNING
If you have adjusted the default settings, such as keyword similarity weight or similarity threshold, to achieve the optimal results, be aware that these changes will not be automatically saved. You must apply them to your chat assistant settings or the **Retrieval** agent component settings.
:::

## Frequently asked questions

### Is an LLM used when the Use Knowledge Graph switch is enabled?

Yes, your LLM will be involved to analyze your query and extract the related entities and relationship from the knowledge graph. This also explains why additional tokens and time will be consumed.