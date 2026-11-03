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

Switch your doc engine from Elasticsearch to Infinity or OpenSearch.

---

RAGFlow uses Elasticsearch by default for storing full text and vectors. To switch to [Infinity](https://github.com/infiniflow/infinity/), follow these steps:

1. Back up the existing deployment before switching engines. Switching engines creates a new document-index store; reindex the documents after startup.

2. Stop the Go deployment:

   ```bash
   docker compose --env-file docker/.env -f docker/docker-compose.yml down
   ```

:::caution WARNING
Do not add `-v` unless you intentionally want to delete the deployment's data volumes.
:::

3. Set `DOC_ENGINE=infinity` in **docker/.env**.

4. Start the Go deployment:

   ```bash
   docker compose --env-file docker/.env -f docker/docker-compose.yml up -d
   ```

5. Reindex the documents and verify retrieval before retiring the previous document-engine data.

## Switch to OpenSearch

OpenSearch is supported as an alternative to Elasticsearch. Hybrid (BM25 + vector) retrieval requires OpenSearch 2.10 or later, which provides the normalization search pipeline the Go engine provisions on startup.

1. Set `DOC_ENGINE=opensearch` in **docker/.env** and configure the connection:

   ```bash
   OS_HOST=opensearch39        # compose service name of the OpenSearch container
   OS_PORT=1201                # host port; the container listens on 9201
   OS_USER=admin
   OPENSEARCH_PASSWORD=infini_rag_flow_OS_01
   ```

2. The `os` section of **conf/service_conf.yaml** mirrors these settings:

   ```yaml
   os:
     hosts: 'http://opensearch39:9201'
     username: 'admin'
     password: 'infini_rag_flow_OS_01'
   ```

3. Start the deployment and reindex the documents, as in the Infinity steps above. The index template in **conf/os_mapping.json** declares `knn_vector` fields for the common embedding dimensions (128–10240); an embedding model whose dimension is not listed there cannot be indexed until a matching template is added.
