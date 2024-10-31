---
sidebar_position: 7
slug: /upgrade_ragflow
---

# Upgrade RAGFlow

You can upgrade RAGFlow to dev version or the latest version:

- Dev versions are for developers and contributors. They are published on a nightly basis and may crash because they are not fully tested. We cannot guarantee their validity and you are at your own risk trying out latest, untested features.
- The latest version refers to the most recent, officially published release. It is stable and works best with regular users.

To upgrade RAGFlow to the dev version:

Update the RAGFlow image and restart RAGFlow:

1. Update **ragflow/docker/.env** as follows:

   ```bash
   RAGFLOW_IMAGE=infiniflow/ragflow:dev
   ```

2. Update ragflow image and restart ragflow:

   ```bash
   docker compose -f docker/docker-compose.yml pull
   docker compose -f docker/docker-compose.yml up -d
   ```

To upgrade RAGFlow to the latest version:

1. Update **ragflow/docker/.env** as follows:

   ```bash
   RAGFLOW_IMAGE=infiniflow/ragflow:latest
   ```

2. Update the RAGFlow image and restart RAGFlow:

   ```bash
   docker compose -f docker/docker-compose.yml pull
   docker compose -f docker/docker-compose.yml up -d
   ```
