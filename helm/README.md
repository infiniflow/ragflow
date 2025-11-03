# RAGFlow 组件交互说明
- 前端 nginx 仅负责静态资源与入口转发，所有动态请求通过 Ingress 进入 Ragflow API。
- Ragflow API 在接到任务时写入 Redis Stream，由 Worker 消费并回写进度；任务相关数据保存在共享存储（数据库、向量库、对象存储）。
- Admin 只对 Ragflow API 发起少量 HTTP 调用（如健康检查、后台管理），不会直接访问其他组件。
- MCP Server 通过 HTTP/SSE 调用 Ragflow API 暴露的检索与工具接口，本身不直接访问底层存储。
- API、Admin、Worker 共同依赖外部的数据库、Redis、向量库与对象存储。

```
Browser    Ingress    Frontend    Ragflow API    Redis Queue    Worker    Admin    MCP Server    Shared Stores (DB/Vector/MinIO)
   |          |           |             |              |            |         |            |                      |
1. |--GET /-->|--/------->|             |              |            |         |            |                      |
   |<--HTML---|<--static--|             |              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
2. |--/api--> |----------/api---------->|              |            |         |            |                      |
   |          |           |             |-------------read/write------------------------------------------------->|
   |<--resp---|<------HTTP resp --------|              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
3. |--task--> |----------/api---------->|--write msg-->|            |         |            |                      |
   |          |           |             |              |--consume-->|         |            |                      |
   |          |           |             |<--progress---|            |------------------read/write---------------->|
   |<--resp---|<------HTTP resp --------|              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
4. |          |           |             |<--HTTP/SSE (MCP tools & calls)------------------>|                      |
   |          |           |             |----------------HTTP/SSE resp---------------------|                      |
   |          |           |             |              |            |         |            |                      |
   |          |           |             |              |            |         |            |                      |
5. |----req-->|------/api/v1/admin----->|---------/api/v1/admin-------------->|            |                      |
   |          |           |             |                                     |-------------read/write----------->|
   |<--resp---|<------HTTP resp --------|<------------HTTP resp --------------|            |                      |
   |          |           |             |<--------/v1/system/ping ------------|            |                      |
   |          |           |             |---------HTTP resp------------------>|            |                      |
```

### 主要配置项列表

下表按照作用域汇总 Helm `values.yaml` 支持的键及默认值；除非特别说明，`null` 表示该字段缺省时不会被渲染，`[]`/`{}` 表示空集合，可按需覆写。

#### 全局与环境

| 参数 | 默认值 | 说明 |
|---|---|---|
| `nameOverride` | `null` | 覆盖 Chart 名称用于生成 Kubernetes 资源名前缀。 |
| `fullnameOverride` | `null` | 直接指定完整资源名前缀。 |
| `imagePullSecrets` | `[]` | 追加到所有工作负载的 `imagePullSecrets`。 |
| `env.DOC_ENGINE` | `infinity` | 选择文档/向量引擎：`infinity`、`elasticsearch` 或 `opensearch`。 |
| `env.STACK_VERSION` | `8.11.3` | 默认 Elastic Stack 版本。 |
| `env.TZ` | `Asia/Shanghai` | 设置容器时区。 |
| `env.DOC_BULK_SIZE` | `4` | 文档批量写入大小。 |
| `env.EMBEDDING_BATCH_SIZE` | `16` | 向量嵌入批次大小。 |
| `env.*` | *(自定义)* | 追加任意环境变量，写入 `*-env-config` Secret 并被所有组件加载。 |

#### externalServices.*

| 参数 | 默认值 | 说明 |
|---|---|---|
| `externalServices.redis.enabled` | `false` | 复用外部 Redis 时开启，禁用内置 Redis 部署。 |
| `externalServices.redis.host` | `redis:6379` | 外部 Redis 连接地址。 |
| `externalServices.redis.password` | `password` | 外部 Redis 密码。 |
| `externalServices.redis.db` | `1` | 外部 Redis 数据库序号。 |
| `externalServices.mysql.enabled` | `false` | 复用外部 MySQL 时开启，禁用内置 MySQL。 |
| `externalServices.mysql.host` | `mysql` | 外部 MySQL 主机名。 |
| `externalServices.mysql.port` | `3306` | 外部 MySQL 端口。 |
| `externalServices.mysql.name` | `rag_flow` | 外部 MySQL 数据库名。 |
| `externalServices.mysql.user` | `root` | 外部 MySQL 用户名。 |
| `externalServices.mysql.password` | `password` | 外部 MySQL 密码。 |
| `externalServices.mysql.max_connections` | `900` | 可选，覆盖最大连接数。 |
| `externalServices.mysql.stale_timeout` | `300` | 可选，连接超时时间（秒）。 |
| `externalServices.mysql.max_allowed_packet` | `1073741824` | 可选，MySQL `max_allowed_packet`。 |
| `externalServices.s3.enabled` | `false` | 使用外部 S3 兼容存储时开启，禁用内置 MinIO。 |
| `externalServices.s3.access_key` | `""` | S3 访问 key。 |
| `externalServices.s3.secret_key` | `""` | S3 密钥。 |
| `externalServices.s3.session_token` | `""` | 临时凭证 token。 |
| `externalServices.s3.region_name` | `""` | S3 Region（若留空，可结合 `region`/`endpoint_url`）。 |
| `externalServices.s3.endpoint_url` | `""` | S3 Endpoint。 |
| `externalServices.s3.bucket` | `""` | 目标桶名称。 |
| `externalServices.s3.prefix_path` | `""` | 上传前缀。 |
| `externalServices.s3.signature_version` | `""` | 自定义签名版本，如 `s3v4`。 |
| `externalServices.s3.addressing_style` | `""` | 自定义桶寻址方式：`virtual`/`path`/`auto`。 |
| `externalServices.elasticsearch.enabled` | `false` | 使用外部 Elasticsearch 时开启，禁用内置 ES。 |
| `externalServices.elasticsearch.host` | `http://elasticsearch:9200` | 示例默认值；仅为向后兼容，占位使用。 |
| `externalServices.elasticsearch.hosts` | `null` | 启用外部 ES 时必须提供的地址（可为字符串或字符串数组）。 |
| `externalServices.elasticsearch.username` | `elastic` | 外部 Elasticsearch 用户名。 |
| `externalServices.elasticsearch.password` | `password` | 外部 Elasticsearch 密码。 |

#### ragflow 通用配置

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.image.repository` | `infiniflow/ragflow` | 默认镜像仓库，所有组件继承。 |
| `ragflow.image.tag` | `v0.21.1-slim` | 默认镜像标签。 |
| `ragflow.image.pullPolicy` | `IfNotPresent` | 默认镜像拉取策略。 |
| `ragflow.image.pullSecrets` | `[]` | 全局镜像拉取凭据。 |
| `ragflow.service_conf` | `null` | 追加到 `local.service_conf.yaml` 的自定义内容。 |
| `ragflow.llm_factories` | `null` | 渲染至 `llm_factories.json` 的自定义配置。 |

#### ragflow.frontend

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.frontend.replicaCount` | `2` | 前端 Deployment 副本数。 |
| `ragflow.frontend.deployment.strategy` | `null` | 自定义 Deployment strategy。 |
| `ragflow.frontend.deployment.resources` | `limits: {cpu: 500m, memory: 500Mi}; requests: {cpu: 200m, memory: 200Mi}` | Pod 资源请求与限制。 |
| `ragflow.frontend.podAnnotations` | `{}` | 额外 Pod 注解。 |
| `ragflow.frontend.probes` | `{}` | 可自定义 liveness/readiness/startup 探针，留空时使用内置默认值。 |
| `ragflow.frontend.extraEnv` | `[]` | 追加前端容器环境变量。 |
| `ragflow.frontend.image.*` | `null` | 可按需覆盖仓库/tag/pullPolicy/pullSecrets，缺省继承全局设置。 |
| `ragflow.frontend.service.type` | `ClusterIP` | Service 类型。 |
| `ragflow.frontend.service.port` | `80` | Service 端口。 |

#### ragflow.api

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.api.replicaCount` | `2` | API Deployment 副本数。 |
| `ragflow.api.deployment.strategy` | `null` | 自定义 Deployment strategy。 |
| `ragflow.api.deployment.resources` | `limits: {cpu: "1", memory: 2Gi}; requests: {cpu: 500m, memory: 1Gi}` | Pod 资源请求与限制。 |
| `ragflow.api.podAnnotations` | `{}` | 额外 Pod 注解。 |
| `ragflow.api.debug` | `false` | 启用 Flask 调试/自动重载。 |
| `ragflow.api.extraEnv` | `[]` | 追加 API 环境变量。 |
| `ragflow.api.extraArgs` | `[]` | 追加启动参数（写到 `python3 api/ragflow_server.py` 命令后）。 |
| `ragflow.api.probes` | `{}` | 自定义探针，留空时使用 HTTP `/v1/system/healthz` 默认配置。 |
| `ragflow.api.image.*` | `null` | 按需覆盖镜像配置，缺省继承全局。 |
| `ragflow.api.service.type` | `ClusterIP` | Service 类型。 |
| `ragflow.api.service.port` | `80` | Service 端口（Pod 监听 9380）。 |

#### ragflow.worker

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.worker.replicaCount` | `2` | Worker Deployment 副本数。 |
| `ragflow.worker.deployment.strategy` | `null` | 自定义 Deployment strategy。 |
| `ragflow.worker.deployment.resources` | `limits: {cpu: "2", memory: 4Gi}; requests: {cpu: "1", memory: 2Gi}` | Worker 资源请求与限制。 |
| `ragflow.worker.podAnnotations` | `{}` | 额外 Pod 注解。 |
| `ragflow.worker.consumerRange.enabled` | `false` | 启用消费者编号区间。 |
| `ragflow.worker.consumerRange.begin` | `0` | 区间起始编号，与 `enabled` 联动使用。 |
| `ragflow.worker.extraArgs` | `[]` | 追加 Task Executor 启动参数。 |
| `ragflow.worker.extraEnv` | `[]` | 追加 Worker 环境变量。 |
| `ragflow.worker.probes` | `{}` | 自定义探针，留空时使用内置命令行探活。 |
| `ragflow.worker.image.*` | `null` | 覆盖 Worker 镜像配置，缺省继承全局。 |

#### ragflow.admin

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.admin.enabled` | `false` | 是否部署 Admin 服务。 |
| `ragflow.admin.replicaCount` | `1` | Admin Deployment 副本数。 |
| `ragflow.admin.deployment.strategy` | `null` | 自定义 Deployment strategy。 |
| `ragflow.admin.deployment.resources` | `limits: {cpu: "1", memory: 2Gi}; requests: {cpu: 500m, memory: 1Gi}` | Admin 资源请求与限制。 |
| `ragflow.admin.podAnnotations` | `{}` | 额外 Pod 注解。 |
| `ragflow.admin.debug` | `false` | 启用 Flask 调试模式。 |
| `ragflow.admin.extraArgs` | `[]` | 追加 Admin 启动参数。 |
| `ragflow.admin.extraEnv` | `[]` | 追加 Admin 环境变量。 |
| `ragflow.admin.probes` | `{}` | 自定义探针，留空时使用内置 HTTP 健康检查。 |
| `ragflow.admin.image.*` | `null` | 覆盖 Admin 镜像配置。 |
| `ragflow.admin.service.type` | `ClusterIP` | Service 类型。 |
| `ragflow.admin.service.port` | `80` | Service 端口（Pod 监听 9381）。 |

#### ragflow.mcp

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ragflow.mcp.enabled` | `false` | 是否部署 MCP Server。 |
| `ragflow.mcp.replicaCount` | `1` | MCP Deployment 副本数。 |
| `ragflow.mcp.deployment.strategy` | `null` | 自定义 Deployment strategy。 |
| `ragflow.mcp.deployment.resources` | `limits: {cpu: "1", memory: 1Gi}; requests: {cpu: "1", memory: 1Gi}` | MCP 资源请求与限制。 |
| `ragflow.mcp.podAnnotations` | `{}` | 额外 Pod 注解。 |
| `ragflow.mcp.mode` | `self-host` | MCP 运行模式。 |
| `ragflow.mcp.hostApiKey` | `""` | 以 self-host 模式调用 API 时使用的 key。 |
| `ragflow.mcp.transport.sse` | `true` | 是否启用 SSE 传输。 |
| `ragflow.mcp.transport.streamableHttp` | `true` | 是否启用 Streamable HTTP。 |
| `ragflow.mcp.transport.jsonResponse` | `true` | Streamable HTTP 是否使用 JSON 响应。 |
| `ragflow.mcp.extraArgs` | `[]` | 追加 MCP 启动参数。 |
| `ragflow.mcp.extraEnv` | `[]` | 追加 MCP 环境变量。 |
| `ragflow.mcp.probes` | `{}` | 自定义探针，留空时使用 TCP 探活。 |
| `ragflow.mcp.image.*` | `null` | 覆盖 MCP 镜像配置。 |
| `ragflow.mcp.port` | `9382` | MCP 容器监听端口，模板内置默认值，需时可显式覆写。 |
| `ragflow.mcp.service.type` | `ClusterIP` | Service 类型。 |
| `ragflow.mcp.service.port` | `80` | Service 端口（Pod 监听 9382）。 |

#### 内置中间件（按需启用）

| 参数 | 默认值 | 说明 |
|---|---|---|
| `infinity.image.repository` | `infiniflow/infinity` | Infinity 镜像仓库（`env.DOC_ENGINE=infinity` 时部署）。 |
| `infinity.image.tag` | `v0.6.1` | Infinity 镜像标签。 |
| `infinity.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `infinity.image.pullSecrets` | `[]` | 镜像凭据。 |
| `infinity.storage.className` | `null` | 挂载使用的 StorageClass。 |
| `infinity.storage.capacity` | `5Gi` | PVC 容量。 |
| `infinity.deployment.strategy` | `null` | Deployment strategy。 |
| `infinity.deployment.resources` | `null` | 资源请求/限制（缺省未设置）。 |
| `infinity.service.type` | `ClusterIP` | Service 类型。 |
| `elasticsearch.credentials.username` | `elastic` | 内置 Elasticsearch 默认用户名。 |
| `elasticsearch.credentials.password` | `infini_rag_flow_helm` | 内置 Elasticsearch 默认密码。 |
| `elasticsearch.image.repository` | `elasticsearch` | Elasticsearch 镜像仓库。 |
| `elasticsearch.image.tag` | `8.11.3` | Elasticsearch 镜像标签。 |
| `elasticsearch.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `elasticsearch.image.pullSecrets` | `[]` | 镜像凭据。 |
| `elasticsearch.initContainers.alpine.repository` | `alpine` | 初始化容器镜像。 |
| `elasticsearch.initContainers.alpine.tag` | `latest` | 初始化容器标签。 |
| `elasticsearch.initContainers.busybox.repository` | `busybox` | 初始化容器镜像。 |
| `elasticsearch.initContainers.busybox.tag` | `latest` | 初始化容器标签。 |
| `elasticsearch.storage.className` | `null` | ElasticSearch PVC 的 StorageClass。 |
| `elasticsearch.storage.capacity` | `20Gi` | Elasticsearch 数据卷容量。 |
| `elasticsearch.deployment.strategy` | `null` | Deployment strategy。 |
| `elasticsearch.deployment.resources.requests.cpu` | `"4"` | CPU 请求。 |
| `elasticsearch.deployment.resources.requests.memory` | `16Gi` | 内存请求。 |
| `elasticsearch.service.type` | `ClusterIP` | Service 类型。 |
| `opensearch.image.repository` | `opensearchproject/opensearch` | Opensearch 镜像仓库。 |
| `opensearch.image.tag` | `2.19.1` | Opensearch 镜像标签。 |
| `opensearch.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `opensearch.image.pullSecrets` | `[]` | 镜像凭据。 |
| `opensearch.initContainers.alpine.repository` | `alpine` | 初始化容器镜像。 |
| `opensearch.initContainers.alpine.tag` | `latest` | 初始化容器标签。 |
| `opensearch.initContainers.busybox.repository` | `busybox` | 初始化容器镜像。 |
| `opensearch.initContainers.busybox.tag` | `latest` | 初始化容器标签。 |
| `opensearch.storage.className` | `null` | Opensearch PVC 的 StorageClass。 |
| `opensearch.storage.capacity` | `20Gi` | Opensearch 数据卷容量。 |
| `opensearch.deployment.strategy` | `null` | Deployment strategy。 |
| `opensearch.deployment.resources.requests.cpu` | `"4"` | CPU 请求。 |
| `opensearch.deployment.resources.requests.memory` | `16Gi` | 内存请求。 |
| `opensearch.service.type` | `ClusterIP` | Service 类型。 |
| `minio.credentials.user` | `rag_flow` | 内置 MinIO 用户名。 |
| `minio.credentials.password` | `infini_rag_flow_helm` | MinIO 密码。 |
| `minio.image.repository` | `quay.io/minio/minio` | MinIO 镜像仓库。 |
| `minio.image.tag` | `RELEASE.2023-12-20T01-00-02Z` | MinIO 镜像标签。 |
| `minio.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `minio.image.pullSecrets` | `[]` | 镜像凭据。 |
| `minio.storage.className` | `null` | MinIO PVC 的 StorageClass。 |
| `minio.storage.capacity` | `5Gi` | MinIO 数据卷容量。 |
| `minio.deployment.strategy` | `null` | Deployment strategy。 |
| `minio.deployment.resources` | `null` | 资源请求/限制（缺省未设置）。 |
| `minio.service.type` | `ClusterIP` | Service 类型。 |
| `mysql.credentials.name` | `rag_flow` | 内置 MySQL 默认数据库。 |
| `mysql.credentials.user` | `root` | MySQL 默认用户。 |
| `mysql.credentials.password` | `infini_rag_flow_helm` | MySQL 用户密码。 |
| `mysql.image.repository` | `mysql` | MySQL 镜像仓库。 |
| `mysql.image.tag` | `8.0.39` | MySQL 镜像标签。 |
| `mysql.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `mysql.image.pullSecrets` | `[]` | 镜像凭据。 |
| `mysql.storage.className` | `null` | MySQL PVC 的 StorageClass。 |
| `mysql.storage.capacity` | `5Gi` | MySQL 数据卷容量。 |
| `mysql.deployment.strategy` | `null` | Deployment strategy。 |
| `mysql.deployment.resources` | `null` | 资源请求/限制（缺省未设置）。 |
| `mysql.service.type` | `ClusterIP` | Service 类型。 |
| `redis.credentials.password` | `infini_rag_flow_helm` | 内置 Redis 密码。 |
| `redis.credentials.db` | `1` | 内置 Redis 默认数据库。 |
| `redis.image.repository` | `valkey/valkey` | Redis (Valkey) 镜像仓库。 |
| `redis.image.tag` | `8` | Redis 镜像标签。 |
| `redis.image.pullPolicy` | `IfNotPresent` | 镜像拉取策略。 |
| `redis.image.pullSecrets` | `[]` | 镜像凭据。 |
| `redis.storage.className` | `null` | Redis PVC 的 StorageClass。 |
| `redis.storage.capacity` | `5Gi` | Redis 数据卷容量。 |
| `redis.persistence.enabled` | `true` | 是否启用有状态持久化 (StatefulSet+PVC)。 |
| `redis.persistence.retentionPolicy.whenDeleted` | `null` | 可选，自定义 PVC 删除行为。 |
| `redis.persistence.retentionPolicy.whenScaled` | `null` | 可选，自定义缩容时的 PVC 保留策略。 |
| `redis.deployment.strategy` | `null` | StatefulSet updateStrategy（缺省未设置）。 |
| `redis.deployment.resources` | `null` | 资源请求/限制（缺省未设置）。 |
| `redis.service.type` | `ClusterIP` | Service 类型。 |

#### Ingress

| 参数 | 默认值 | 说明 |
|---|---|---|
| `ingress.enabled` | `false` | 是否创建 Ingress。 |
| `ingress.className` | `""` | 绑定的 IngressClass，留空时使用集群默认。 |
| `ingress.annotations` | `{}` | 自定义 Ingress 注解。 |
| `ingress.hosts` | `[{"host":"chart-example.local","paths":[{path:"/",component:"frontend"},{path:"/api",component:"api"},{path:"/v1",component:"api"}]}]` | 路由规则配置，可按组件映射到不同 Service/端口。 |
| `ingress.tls` | `[]` | TLS 配置列表。 |
