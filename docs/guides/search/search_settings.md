---
sidebar_position: 2
title: Search Settings
sidebar_label: Search Settings
slug: /search_settings
sidebar_custom_props:
  categoryIcon: LucideSearch
---

# Search Settings

Search Settings allow you to configure the knowledge sources, retrieval strategy, and presentation of search results. After completing the configuration, click **Save**.

## Datasets

Select the knowledge bases that Search will use. When a user submits a query, the system retrieves relevant content from the selected knowledge bases.

You can select one or more knowledge bases. When multiple knowledge bases are selected, Search retrieves relevant content from all of them simultaneously.

Select knowledge bases based on the intended use case of the Search. Avoid adding large numbers of knowledge bases that are unrelated to the search topic, as this may increase the number of irrelevant results.

## Metadata

Use **Metadata** to restrict retrieval by document metadata. Available fields depend on the selected knowledge bases. This setting filters content; it is not a switch for displaying metadata in results.

## Similarity threshold

Sets the minimum similarity threshold for search results. It is used to filter out chunks that have low relevance to the query.

Only content with a similarity score that meets or exceeds the threshold is included in the search results.

- Increase the threshold: Filtering becomes stricter, and the results are generally more relevant, but some useful content may be omitted.
- Decrease the threshold: More content can be retrieved, but the number of irrelevant results may also increase.

If there are too many results and their relevance is low, try increasing this value. If Search frequently fails to return the expected content, try decreasing it.

## Keyword similarity weight

Sets the full-text weight in hybrid retrieval. For example, `0.7` means full-text weight `0.7` and vector weight `0.3`.

- Increase it to favor keywords, product names, identifiers, and exact terms.
- Decrease it to favor semantic matches when the query differs from the source wording.

Start with the default value and adjust it based on search results.

## Rerank candidates

Sets the number of candidate chunks that enter the reranking stage.

The system first retrieves candidate content from the knowledge bases and then determines which results to display from among those candidates. This parameter controls the candidate pool available for subsequent ranking or reranking.

For example, a value of `100` means that up to 100 candidate chunks are selected for subsequent processing.

- Increase the value: Expands the candidate pool and reduces the likelihood that highly relevant content will be missed during initial retrieval, but increases processing overhead.
- Decrease the value: Generally improves search speed, but the smaller candidate pool may omit some relevant content.

For large knowledge bases or use cases that require broader retrieval coverage, you can increase this value as appropriate.

## Rerank model

Controls whether a rerank model is used to rerank the retrieved results.

When enabled, you must select an available rerank model. The system uses the model to further evaluate the relevance between the query and the candidate chunks, and then reorders the search results.

Reranking can generally improve the quality of the final result ordering, but it introduces additional model calls and increases search latency.

Enable it if search accuracy is the priority. Leave it disabled if response speed is more important or no rerank model has been configured.

## AI summary

Controls whether an AI-generated summary is produced for the search results.

When enabled, the system uses a model to summarize the retrieved content. This helps users quickly understand the main information in the search results without reading every chunk individually.

This feature requires a model call and may therefore increase result-generation time and model usage costs.

## Enable related search

Controls whether related searches are generated for the current query.

When enabled, the system can suggest related searches based on the current query and its search results, helping users continue exploring information related to the current topic.

This feature is suitable for content exploration, research, and open-ended searches. If you only need to view the results for the current query, you can leave it disabled.

## Show query mindmap

Controls whether a mind map for the current query is displayed.

When enabled, the system organizes and presents information from the query and related searches as a mind map, helping users quickly understand the relationships between topics and the overall structure of the subject.

This feature is more suitable for knowledge exploration and searches involving complex topics. It is generally unnecessary for simple factual queries.
