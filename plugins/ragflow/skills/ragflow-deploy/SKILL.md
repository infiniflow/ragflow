---
name: ragflow-deploy
description: >-
  Deploy RAGFlow with Docker Compose and get it ready for use: the
  vm.max_map_count requirement for Elasticsearch, release tags and
  RAGFLOW_IMAGE, docker/.env settings, health checks, model provider setup and
  creating an API key. Use when installing, upgrading or troubleshooting a
  self-hosted RAGFlow server.
---

# Deploying RAGFlow

Reference: `docs/quickstart.mdx`, `docker/README.md`, `docker/.env`.

## Start the server

```bash
# Elasticsearch (the default document engine) needs this on the Docker host
sysctl vm.max_map_count
sudo sysctl -w vm.max_map_count=262144   # persist in /etc/sysctl.conf

git clone https://github.com/infiniflow/ragflow.git
cd ragflow/docker
git checkout v1.0.0                      # or another release tag
# Before first `up`: set a unique ADMIN_DEFAULT_PASSWORD in docker/.env
# (used only when the superuser is created) and restrict host access to
# ADMIN_SVR_HTTP_PORT (default 9381).
docker compose -f docker-compose.yml up -d
```

- `RAGFLOW_IMAGE` in `docker/.env` must match the checked-out tag.
- `docker/.env` controls `DOC_ENGINE`, service ports and dependency
  credentials. Change default passwords before exposing the host.
- Images exist for Linux x86-64 and ARM64; Docker picks the right one.

## Check it is up

```bash
docker ps
docker compose logs ragflow-cpu
curl -f http://localhost/api/v1/system/healthz   # add SVR_WEB_HTTP_PORT if changed
```

Wait for the health check to return 200 before opening
`http://<host>` in a browser and registering or logging in.

## First-time setup

1. Avatar → **User settings** → **Model providers**: add a provider
   instance with its API key/base URL, then add a Chat model and an
   Embedding model.
2. Under **Set default models**, pick the Chat and Embedding defaults.
3. Create an API key ([guide](https://ragflow.io/docs/dev/acquire_ragflow_api_key)) for the SDK,
   HTTP API and MCP endpoint.

## Troubleshooting

- `Refusing to start: database was migrated by a newer version` on a
  development checkout: set `RAGFLOW_DEV_MODE=true` in `.env` for local dev
  only and rerun Compose. Keep it `false` in production.
- Elasticsearch crash-looping usually means `vm.max_map_count` is too low
  (on Docker Desktop, set it inside its Linux VM).
- After editing `.env` or `service_conf.yaml.template`, recreate the
  containers so the entrypoint re-renders the template:
  `docker compose up -d --force-recreate`.
