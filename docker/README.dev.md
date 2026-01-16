# RAGFlow Local Development Environment

This is a fully local development setup for RAGFlow on macOS. All services run in Docker containers on your Mac with no external dependencies.

## Services Included

- **PostgreSQL** (port 5433): Primary database
- **Elasticsearch** (port 9201): Search and vector storage
- **Redis** (port 6380): Cache and session storage
- **MinIO** (ports 9001, 9002): S3-compatible object storage
- **RAGFlow** (ports 9380, 8080, 8443): Main application with hot reloading

## Quick Start

1. **Start all services:**
   ```bash
   cd docker
   docker compose -f docker-compose.dev.yml up -d
   ```

2. **View logs:**
   ```bash
   docker compose -f docker-compose.dev.yml logs -f ragflow-dev
   ```

3. **Stop all services:**
   ```bash
   docker compose -f docker-compose.dev.yml down
   ```

4. **Clean up (removes volumes):**
   ```bash
   docker compose -f docker-compose.dev.yml down -v
   ```

## Access Points

- **RAGFlow API**: http://localhost:9380
- **RAGFlow Web UI**: http://localhost:8080
- **MinIO Console**: http://localhost:9002 (user: ragflow, pass: ragflow_dev_pass)
- **Elasticsearch**: http://localhost:9201 (user: elastic, pass: ragflow_dev_pass)
- **PostgreSQL**: localhost:5433 (user: ragflow, pass: ragflow_dev_pass, db: ragflow)
- **Redis**: localhost:6380 (pass: ragflow_dev_pass)

## Hot Reloading

The following directories are mounted as volumes for hot reloading:
- `api/` - Backend API
- `common/` - Shared utilities
- `rag/` - RAG core logic
- `agent/` - Agent components
- `agentic_reasoning/` - Reasoning modules
- `deepdoc/` - Document parsing
- `memory/` - Memory management
- `graphrag/` - Graph RAG
- `plugin/` - Plugins
- `conf/` - Configuration

Changes to these directories will be reflected in the running container.

## Configuration

All configuration is in:
- `.env.dev` - Environment variables
- `service_conf.yaml.dev.template` - Service configuration template

Default credentials (change for production):
- All passwords: `ragflow_dev_pass`
- All usernames: `ragflow` (or `elastic` for Elasticsearch)

## Troubleshooting

**Services won't start:**
```bash
# Check logs
docker compose -f docker-compose.dev.yml logs

# Check if ports are already in use
lsof -i :9380 -i :9201 -i :6380 -i :5433
```

**Database connection issues:**
```bash
# Wait for health checks to pass
docker compose -f docker-compose.dev.yml ps

# Manually test connection
docker exec -it ragflow-postgres-dev psql -U ragflow -d ragflow
```

**Reset everything:**
```bash
docker compose -f docker-compose.dev.yml down -v
docker compose -f docker-compose.dev.yml up -d
```

## Development Workflow

1. Make changes to source code
2. The RAGFlow container will auto-reload (if the framework supports it)
3. For backend changes, you may need to restart: `docker compose -f docker-compose.dev.yml restart ragflow-dev`
4. Check logs: `docker compose -f docker-compose.dev.yml logs -f ragflow-dev`

## Differences from Production

- Smaller resource limits (suitable for local dev)
- All services run locally (no external dependencies)
- Hot reloading enabled via volume mounts
- Development-friendly ports (no conflicts with common services)
- Simplified configuration with sensible defaults
- Uses `.dev` naming convention for all artifacts
