---
sidebar_position: 7
slug: /upgrade_ragflow
---

# Upgrade RAGFlow

You can upgrade RAGFlow to dev version or the latest version:

- Dev versions are executable files published nightly, incorporating our latest features and bug fixes.
- The latest version is the most recent, officially published release. A key disction between the latest version and dev versions is that the latest version consistently features a incrementing release number.

## Upgrade RAGFlow to the dev version

1. Update **ragflow/docker/.env** as follows:

   ```bash
   RAGFLOW_IMAGE=infiniflow/ragflow:dev
   ```

2. Update RAGFlow image and restart RAGFlow:

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
