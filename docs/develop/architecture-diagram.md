---
sidebar_position: 2
slug: /architecture_diagram
sidebar_custom_props: {
  categoryIcon: LucideBookOpen
}
---

# Docker Compose architecture

The default CPU deployment uses Elasticsearch for the document index and MySQL for application metadata. The [Compose files](https://github.com/infiniflow/ragflow/tree/main/docker) start these services:

| Service | Role |
| --- | --- |
| `ragflow-cpu` | Runs nginx and `ragflow_server`. By default, one Go process runs the API, admin server, ingestion worker, data source syncer, and chat-channel service. |
| `es01` | Indexes document chunks for search and retrieval. |
| `mysql` | Stores application metadata, including documents, datasets, and ingestion task state. |
| `minio` | Provides S3-compatible storage for uploaded documents and generated assets. |
| `kvrocks` | Caches ingestion data, including extracted images while a pipeline runs. |
| `nats` | Uses JetStream to queue background work, including ingestion tasks. |

## From upload to search

1. The API stores the uploaded file in `minio` and records its metadata in `mysql`.
2. When parsing starts, the API records an ingestion task in `mysql` and publishes a message to the `RAGFLOW_TASKS` JetStream stream.
3. The ingestion worker consumes the message, reads the file from `minio`, and runs the selected ingestion pipeline. It writes the resulting chunks to `es01`, where search and retrieval can find them.

Run `docker compose -f docker-compose.yml config --services` from `docker/` to list the services for your configured profiles.

## External architecture diagram

[Explore the Docker Compose services and request flows on IceShore](https://app.iceshore.ai/en/design/d92456af3f788da5b99.38526979/).

:::caution NOTE
IceShore (Launchers Co., Ltd.) maintains the diagram independently. It shows [commit `9a81eddf`](https://github.com/infiniflow/ragflow/commit/9a81eddfd8ab9ad3a44f391e182c73dcd2936369), when RAGFlow started the Go services as separate processes. The diagram may lag behind current releases. See the [current container entrypoint](https://github.com/infiniflow/ragflow/blob/main/docker/entrypoint.sh) for the default startup behavior.
:::
