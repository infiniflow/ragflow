---
sidebar_position: 40
slug: /indexer_component
sidebar_custom_props: {
  categoryIcon: LucideListPlus
}
---
# Indexer component

A component that defines how chunks are indexed.

---

An **Indexer** component indexes chunks and configures their storage formats in the document engine.

## Scenario

An **Indexer** component is the mandatory ending component for all ingestion pipelines.

## Configurations

### Search method

This setting configures how chunks are stored in the document engine: as full-text, embeddings, or both.

### Filename embedding weight

This setting defines the filename's contribution to the final embedding, which is a weighted combination of both the chunk content and the filename. Essentially, a higher value gives the filename more influence in the final *composite* embedding.

- 0.1: Filename contributes 10% (chunk content 90%)
- 0.5 (maximum): Filename contributes 50% (chunk content 90%)