---
sidebar_position: 3
slug: /switch_doc_engine
sidebar_custom_props: {
  categoryIcon: LucideShuffle
}
---
# Switch document engine

Switch your doc engine from Elasticsearch to Infinity.

---

RAGFlow uses Elasticsearch by default for storing full text and vectors. To switch to [Infinity](https://github.com/infiniflow/infinity/), follow these steps:

:::caution WARNING
Switching to Infinity on a Linux/arm64 machine is not yet officially supported.
:::

1. Stop all running containers:

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```

:::caution WARNING
`-v` will delete the docker container volumes, and the existing data will be cleared.
:::

2. Set `DOC_ENGINE` in **docker/.env** to `infinity`.

3. Start the containers:

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```