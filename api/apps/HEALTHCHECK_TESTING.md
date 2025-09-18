# 健康检查与 Kubernetes 探针简明说明

本文件说明：什么是 K8s 探针、如何用 `/v1/system/healthz` 做健康检查，以及下文用例中的关键词含义。

## 什么是 K8s 探针（Probe）
- 探针是 K8s 用来“探测”容器是否健康/可对外服务的机制。
- 常见三类：
  - livenessProbe：活性探针。失败时 K8s 会重启容器，用于“应用卡死/失去连接时自愈”。
  - readinessProbe：就绪探针。失败时 Endpoint 不会被加入 Service 负载均衡，用于“应用尚未准备好时不接流量”。
  - startupProbe：启动探针。给慢启动应用更长的初始化窗口，期间不执行 liveness/readiness。
- 这些探针通常通过 HTTP GET 访问一个公开且轻量的健康端点（无需鉴权），以 HTTP 状态码判定结果：200=通过；5xx/超时=失败。

## 本项目健康端点
- 已实现：`GET /v1/system/healthz`（无需认证）。
- 语义：
  - 200：关键依赖正常。
  - 500：任一关键依赖异常（当前判定为 DB 或 Chat）。
  - 响应体：JSON，最小字段 `status, db, chat`；并包含 `redis, doc_engine, storage` 等可观测项。失败项会在 `_meta` 中包含 `error/elapsed`。
- 示例（DB 故障）：
```json
{"status":"nok","chat":"ok","db":"nok"}
```

## 用例背景（Problem/use case）
- 现状：Ragflow 跑在 K8s，数据库是 AWS RDS Postgres，凭证由 Secret Manager 管理并每 7 天轮换。轮换后应用连接失效，需要手动重启 Pod 才能重新建立连接。
- 目标：通过 K8s 探针自动化检测并重启异常 Pod，减少人工操作。
- 需求：一个“无需鉴权”的公共健康端点，能在依赖异常时返回非 200（如 500）且提供 JSON 详情。
- 现已满足：`/v1/system/healthz` 正是为此设计。

## 关键术语解释（对应你提供的描述）
- Ragflow instance：部署在 K8s 的 Ragflow 服务。
- AWS RDS Postgres：托管的 PostgreSQL 数据库实例。
- Secret Manager rotation：Secrets 定期轮换（每 7 天），会导致旧连接失效。
- Probes（K8s 探针）：liveness/readiness，用于自动重启或摘除不健康实例。
- Public endpoint without API key：无需 Authorization 的 HTTP 路由，便于探针直接访问。
- Dependencies statuses：依赖健康状态（db、chat、redis、doc_engine、storage 等）。
- HTTP 500 with JSON：当依赖异常时返回 500，并附带 JSON 说明哪个子系统失败。

## 快速测试
- 正常：
```bash
curl -i http://<host>/v1/system/healthz
```
- 制造 DB 故障（docker-compose 示例）：
```bash
docker compose stop db && curl -i http://<host>/v1/system/healthz
```
（预期 500，JSON 中 `db:"nok"`）

## 更完整的测试清单
### 1) 仅查看 HTTP 状态码
```bash
curl -s -o /dev/null -w "%{http_code}\n" http://<host>/v1/system/healthz
```
期望：`200` 或 `500`。

### 2) Windows PowerShell
```powershell
# 状态码
(Invoke-WebRequest -Uri "http://<host>/v1/system/healthz" -Method GET -TimeoutSec 3 -ErrorAction SilentlyContinue).StatusCode
# 完整响应
Invoke-RestMethod -Uri "http://<host>/v1/system/healthz" -Method GET
```

### 3) 通过 kubectl 端口转发本地测试
```bash
# 前端/网关暴露端口不同环境自行调整
kubectl port-forward deploy/<your-deploy> 8080:80 -n <ns>
curl -i http://127.0.0.1:8080/v1/system/healthz
```

### 4) 制造常见失败场景
- DB 失败（推荐）：
```bash
docker compose stop db
curl -i http://<host>/v1/system/healthz   # 预期 500
```
- Chat 失败（可选）：将 `CHAT_CFG` 的 `factory`/`base_url` 设为无效并重启后端，再请求应为 500，且 `chat:"nok"`。
- Redis/存储/文档引擎：停用对应服务后再次请求，可在 JSON 中看到相应字段为 `"nok"`（不影响 200/500 判定）。

### 5) 浏览器验证
- 直接打开 `http://<host>/v1/system/healthz`，在 DevTools Network 查看 200/500；页面正文就是 JSON。
- 反向代理注意：若有自定义 500 错页，需对 `/healthz` 关闭错误页拦截（如 `proxy_intercept_errors off;`）。

## K8s 探针示例
```yaml
readinessProbe:
  httpGet:
    path: /v1/system/healthz
    port: 80
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 2
  failureThreshold: 1
livenessProbe:
  httpGet:
    path: /v1/system/healthz
    port: 80
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 2
  failureThreshold: 3
```

提示：如有反向代理（Nginx）自定义 500 错页，需对 `/healthz` 关闭错误页拦截，以便保留 JSON。
