---
sidebar_position: 7
slug: /upgrade_ragflow
---

# Upgrade RAGFlow

You can upgrade RAGFlow to dev version or the latest version:

- A Dev version (Development version) is the latest, tested image.
- The latest version is the most recent, officially published release.

## Upgrade RAGFlow to the dev version

1. Update **ragflow/docker/.env** as follows:

   ```bash
   RAGFLOW_IMAGE=infiniflow/ragflow:dev
   ```

2. Update the RAGFlow image and restart RAGFlow:

   ```bash
   docker compose -f docker/docker-compose.yml pull
   docker compose -f docker/docker-compose.yml up -d
   ```

## Upgrade RAGFlow to the latest version

1. Update **ragflow/docker/.env** as follows:

   ```bash
   RAGFLOW_IMAGE=infiniflow/ragflow:latest
   ```

2. Update the RAGFlow image and restart RAGFlow:

   ```bash
   docker compose -f docker/docker-compose.yml pull
   docker compose -f docker/docker-compose.yml up -d
   ```
