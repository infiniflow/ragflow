# RAGFlow Helm Chart

A Helm chart to deploy RAGFlow and its dependencies on Kubernetes.

- Components: RAGFlow (web/api) and optional dependencies (Infinity/Elasticsearch/OpenSearch, MySQL, MinIO, Redis)
- Requirements: Kubernetes >= 1.24, Helm >= 3.10

## Install

```bash
helm upgrade --install ragflow ./ \
  --namespace ragflow --create-namespace
```

Uninstall:
```bash
helm uninstall ragflow -n ragflow
```

## Global Settings

- `global.repo`: Prepend a global image registry prefix for all images.
  - Behavior: Replaces the registry part and keeps the image path (e.g., `quay.io/minio/minio` -> `registry.example.com/myproj/minio/minio`).
  - Example: `global.repo: "registry.example.com/myproj"`
- `global.imagePullSecrets`: List of image pull secrets applied to all Pods.
  - Example:
    ```yaml
    global:
      imagePullSecrets:
        - name: regcred
    ```

## External Services (MySQL / MinIO / Redis)

The chart can deploy in-cluster services or connect to external ones. Toggle with `*.enabled`. When disabled, provide host/port via `env.*`.

- MySQL
  - `mysql.enabled`: default `true`
  - If `false`, set:
    - `env.MYSQL_HOST` (required), `env.MYSQL_PORT` (default `3306`)
    - `env.MYSQL_DBNAME` (default `rag_flow`), `env.MYSQL_PASSWORD` (required)
    - `env.MYSQL_USER` (default `root` if omitted)
- MinIO
  - `minio.enabled`: default `true`
  - Configure:
    - `env.MINIO_HOST` (optional external host), `env.MINIO_PORT` (default `9000`)
    - `env.MINIO_ROOT_USER` (default `rag_flow`), `env.MINIO_PASSWORD` (optional)
- Redis (Valkey)
  - `redis.enabled`: default `true`
  - If `false`, set:
    - `env.REDIS_HOST` (required), `env.REDIS_PORT` (default `6379`)
    - `env.REDIS_PASSWORD` (optional; empty disables auth if server allows)

Notes:
- When `*.enabled=true`, the chart renders in-cluster resources and injects corresponding `*_HOST`/`*_PORT` automatically.
- Sensitive variables like `MYSQL_PASSWORD` are required; `MINIO_PASSWORD` and `REDIS_PASSWORD` are optional. All secrets are stored in a Secret.

### Example: use external MySQL, MinIO, and Redis

```yaml
# values.override.yaml
mysql:
  enabled: false  # use external MySQL
minio:
  enabled: false  # use external MinIO (S3 compatible)
redis:
  enabled: false  # use external Redis/Valkey

env:
  # MySQL
  MYSQL_HOST: mydb.example.com
  MYSQL_PORT: "3306"
  MYSQL_USER: root
  MYSQL_DBNAME: rag_flow
  MYSQL_PASSWORD: "<your-mysql-password>"

  # MinIO
  MINIO_HOST: s3.example.com
  MINIO_PORT: "9000"
  MINIO_ROOT_USER: rag_flow
  MINIO_PASSWORD: "<your-minio-secret>"

  # Redis
  REDIS_HOST: redis.example.com
  REDIS_PORT: "6379"
  REDIS_PASSWORD: "<your-redis-pass>"
```

Apply:
```bash
helm upgrade --install ragflow ./helm -n ragflow -f values.override.yaml
```

## Document Engine Selection

Choose one of `infinity` (default), `elasticsearch`, or `opensearch` via `env.DOC_ENGINE`. The chart renders only the selected engine and sets the appropriate host variables.

```yaml
env:
  DOC_ENGINE: infinity   # or: elasticsearch | opensearch
  # For elasticsearch
  ELASTIC_PASSWORD: "<es-pass>"
  # For opensearch
  OPENSEARCH_PASSWORD: "<os-pass>"
```

## Ingress

Expose the web UI via Ingress:

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: ragflow.example.com
      paths:
        - path: /
          pathType: Prefix
```

## OpenShift and restricted Pod Security

On OpenShift the default `restricted-v2` SCC runs every pod under a random UID
taken from the namespace range, with GID 0 as the only supplementary group. Root
containers, privileged containers and added capabilities are rejected. The same
applies to any cluster that enforces the `restricted` Pod Security Standard.

Install with the overlay:

```bash
helm upgrade --install ragflow ./helm -n ragflow \
  -f ./helm/values-openshift.yaml
```

What the overlay changes:

- `ragflow.http.port: 8080`, because a non-root process cannot bind port 80. The
  Service still listens on 80, so Routes and Ingress are unaffected.
- Drops the privileged `sysctl` init container of Elasticsearch and OpenSearch
  and sets `node.store.allow_mmap: "false"` instead. If you prefer mmap, raise
  `vm.max_map_count` on the nodes with a Machine Config or Tuned profile and
  leave the setting out.
- Drops the `chown` init container that fixed the data volume ownership. The
  namespace `fsGroup` handles that on OpenShift.
- Removes the hardcoded `runAsUser` and the `IPC_LOCK` capability, and sets
  `runAsNonRoot`, `seccompProfile: RuntimeDefault` and `drop: [ALL]` everywhere.

The RAGFlow image itself keeps its home directory, nginx pid, temp and log
paths, and its writable state under `/ragflow`, owned by group 0. No `fsGroup`
or SCC exception is needed for it.

One limitation: `USE_DOCLING=true` installs Docling into the virtualenv on every
start. The virtualenv stays read-only so the image does not carry a second copy
of it, so that option needs an image with Docling already baked in.

### MySQL

The upstream `mysql` image assumes UID 999 and does not start under an arbitrary
UID. Either use an external database:

```yaml
mysql:
  enabled: false
env:
  MYSQL_HOST: mydb.example.com
  MYSQL_PASSWORD: "<your-mysql-password>"
```

or point the chart at an image that supports it, for example:

```yaml
mysql:
  image:
    repository: registry.redhat.io/rhel9/mysql-84
    tag: latest
```

## Validate the Chart

```bash
helm lint ./helm
helm template ragflow ./helm > rendered.yaml
```

## Notes

- By default, the chart uses `DOC_ENGINE: infinity` and deploys in-cluster MySQL, MinIO, and Redis.
- The chart injects derived `*_HOST`/`*_PORT` and required secrets into a single Secret (`<release>-ragflow-env-config`).
- `global.repo` and `global.imagePullSecrets` apply to all Pods; per-component `*.image.pullSecrets` still work and are merged with global settings.
