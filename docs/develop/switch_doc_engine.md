---
sidebar_position: 3
title: Switch Document Engine
sidebar_label: Switch Document Engine
slug: /switch_doc_engine
sidebar_custom_props: {
  categoryIcon: LucideShuffle
}
---
# Switch Document Engine

Switch your doc engine from Elasticsearch to Infinity.

---

RAGFlow uses Elasticsearch by default for storing full text and vectors. To switch to [Infinity](https://github.com/infiniflow/infinity/), follow these steps:

1. Back up the existing deployment before switching engines. Switching engines
   creates a new document-index store; reindex the documents after startup.

2. Stop the Go deployment:

   ```bash
   docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml down
   ```

:::caution WARNING
Do not add `-v` unless you intentionally want to delete the deployment's data volumes.
:::

3. Set `DOC_ENGINE=infinity` in **docker/.env-go**.

4. Start the Go deployment:

   ```bash
   docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml up -d
   ```

5. Reindex the documents and verify retrieval before retiring the previous
   document-engine data.
