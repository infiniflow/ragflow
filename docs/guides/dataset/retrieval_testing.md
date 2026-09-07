---
sidebar_position: 7
title: "Retrieval Testing: Retrieval Test"
sidebar_label: "Retrieval Testing: Retrieval Test"
slug: /retrieval_testing
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Retrieval Testing: Retrieval Test

## Retrieval Testing Overview

**Retrieval Testing** is used to verify whether a dataset can recall the expected chunks based on user queries. After documents complete parsing, it is recommended to use typical questions in **Retrieval Testing** to test retrieval results first. Confirm whether the recalled chunks are correct, whether the content is complete, and whether the ranking is reasonable before using the dataset in **Chat**, **Search**, or **Agent**.

Retrieval testing can also help troubleshoot Q&A result problems. If the correct chunk can already be recalled but the final answer is still unsatisfactory, further check the model, prompt, or application configuration. If the target chunk is not recalled, continue checking document parsing, chunks, metadata, and retrieval parameters.

> Tip: Documents used for testing must have completed parsing and be enabled.

## Configure Retrieval Parameters

The **Setting** area on the **Retrieval testing** page is used to adjust retrieval parameters for the current test. You can modify parameters and run tests repeatedly to compare recall results under different configurations. The main parameters include:

- **Similarity threshold**: Candidate chunks below this threshold are filtered out. Raising the threshold narrows the recall range and makes results stricter; lowering the threshold expands the recall range. The default value is `0.2`.
- **Vector similarity weight**: The weight of vector similarity in the comprehensive similarity calculation. The other part of the weight is used for full-text or keyword similarity. For example, when this is set to `0.3`, the other part is `0.7`.
- **Rerank model**: The reranking model used to further calculate relevance and sort candidate results. If no rerank model is selected, retrieval results combine keyword similarity and vector cosine similarity. If selected, reranking results participate in the comprehensive score. Using a rerank model may increase retrieval latency.
- **Cross-language search**: Select one or more target languages so a query can match related content in other languages in the dataset. If no language is selected, retrieval is mainly performed in the current language.
- **Metadata**: Restricts the retrieval range by metadata conditions. After it is set, retrieval is performed only in content that meets the metadata conditions.
- **Top**: Sets the maximum number of candidate results returned. For example, if **Top 10** is selected, up to 10 eligible results are returned.

> Note: Parameter changes in **Retrieval Testing** are only used for the current test and are not automatically synchronized to **Chat Assistant** or **Agent**. After suitable parameters are determined, configure the corresponding parameters in the application that actually uses the dataset or in the **Retrieval** component.

## Execute Retrieval Testing

Enter the query to test under the **Setting** area, then click **Run** to execute retrieval. It is recommended to use questions close to real user expressions and prepare multiple types of queries to check recall for different content.

After the test is executed, chunks matching the current retrieval parameters are displayed in the **Results** area on the right.

## View Retrieval Results

After executing a test, recalled chunks are displayed in the **Results** area on the right, together with the total number of results for the test. Each result mainly displays the recalled chunk content, relevance information, and source document, and is sorted according to the current retrieval configuration.

When viewing retrieval results, focus on:

- Whether the target chunk is successfully recalled.
- Whether the recalled chunk comes from the correct document.
- Whether the chunk content is related to the query and contains the information needed to answer the question.
- Whether highly relevant content appears near the top.
- Whether many chunks unrelated to the query are recalled.
- Whether required content is excluded by metadata or other filter conditions.

The upper-right corner of **Results** provides a **File** filter. You can select a specified document and view only retrieval results from that document. When a dataset contains many documents, this helps check recall for a specific document.

The focus of retrieval testing is not to compare a single score in isolation, but to confirm whether the needed chunks can be accurately recalled and reasonably ranked under the current retrieval configuration.

## Adjust Retrieval Results

If retrieval results do not meet expectations, first determine whether the problem is a content problem or a retrieval configuration problem, then adjust accordingly.

If the target content does not appear in retrieval results at all, check whether the document was parsed successfully, whether the target content has generated corresponding chunks, and whether metadata conditions exclude the related content.

If the target chunk can be recalled but many irrelevant results also appear, raise **Similarity threshold** appropriately to narrow the recall range. If retrieval results rely more on semantic expression or keyword matching, adjust **Vector similarity weight** to rebalance vector retrieval and full-text retrieval. If the target chunk can already be recalled but ranking is unsatisfactory, try configuring **Rerank model** to rerank candidate results.

If you need to see more or fewer candidate results, adjust **Top**. For cross-language datasets, also check whether **Cross-language search** includes the target languages to retrieve. It is recommended to use multiple representative queries for repeated testing and determine final parameters based on the overall recall result instead of adjusting configuration based only on a single test.

Debugging suggestion: when retrieval results are unsatisfactory, troubleshoot in the order of document parsing -> chunk -> metadata -> retrieval parameters. First confirm that the knowledge content itself has correctly entered the dataset, then adjust retrieval parameters. This helps locate the problem faster.
