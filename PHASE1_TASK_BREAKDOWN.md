# Phase 1 任务拆分：PDF 端到端链路

## 目标

上传 PDF → Go ingestor 从 NATS 拉取任务 → HTTP 调 Python PDF 解析服务 → Go 后处理 → 写入 ES/Infinity

## 架构

```
Python API Server
     │
     │ (创建 task 记录 + publish 到 NATS)
     ▼
┌────────────────┐
│  NATS Server   │
│  subject:      │
│  task.ingest   │
└───────┬────────┘
        │ Subscribe (JetStream pull consumer)
        ▼
┌──────────────────────────────────────────────────┐
│                Go Ingestor                        │
│                                                    │
│  1. NATS consumer: pull task 消息                  │
│  2. 解析 task → doc_id, kb_id, parser_config...    │
│  3. MinioStorage.Get(doc_id) → PDF bytes           │
│  4. HTTP POST → Python PDF Parse Service           │
│  5. 后处理 (image → MinIO, chunk ID, mother chunk)  │
│  6. DocEngine.InsertChunks() → ES/Infinity         │
│  7. 更新 task progress / result                    │
│                                                    │
│  (同时通过 gRPC 向 admin 上报心跳，接受 shutdown)     │
└──────────────────────────────────────────────────┘
         │
         │ HTTP POST /api/v1/parse
         ▼
┌─────────────────────────────┐
│   Python PDF Parse Service  │
│   (独立 Quart 进程)           │
│                              │
│  naive.py::chunk()           │
│  → pdfplumber + ONNX OCR     │
│  → NLP 切块 + 分词            │
│  → 返回 chunks JSON          │
└─────────────────────────────┘
```

## 当前状态

| 组件 | 状态 | 说明 |
|------|------|------|
| Go ingestor NATS | ❌ 需新增 | Go 侧没有 NATS client |
| Go ingestor gRPC | ✅ 已有 | 连接 admin 用于心跳/shutdown |
| Go tokenizer | ✅ 已有 | `internal/tokenizer/` CGo 绑定 |
| Go storage | ✅ 已有 | MinIO PUT/GET |
| Go engine | ✅ 已有 | `InsertChunks()` ES/Infinity |
| Go dao | ⚠️ 部分 | `File2Document.GetStorageAddress()` 需确认 |
| Python PDF 解析 | ✅ 已有逻辑 | `naive.py::chunk()` 含全部解析能力 |
| Python PDF HTTP 服务 | ❌ 缺失 | 需要新建 |
| Python → NATS publish | ❌ 缺失 | task_service.py 需新增 NATS publish |

## 子任务 A：Python PDF Parse HTTP 服务

**新建文件**：`api/pdf_parse_server.py`

独立的 Quart 微服务，单一端点 `POST /api/v1/parse`：

- **输入**：multipart `file`（PDF 二进制）+ `config`（JSON 字符串）
- **输出**：`{ success, chunks: [...], chunk_count, duration_ms }`
- 内部复用 `rag.app.naive.FACTORY` → `chunker.chunk()` → 返回 chunk dicts
- 不做 embedding、不生成 chunk ID、不访问 DB/Redis
- 错误返回 `{ success: false, error: "ParserError", message: "..." }`
- 独立端口（如 9384），gunicorn 多 worker

## 子任务 B：Go 共享数据结构

**新建文件**：`internal/common/parse_types.go`

```go
type ParseConfig struct {
    ParserID     string       `json:"parser_id"`
    Language     string       `json:"language"`
    Name         string       `json:"name"`
    FromPage     int          `json:"from_page"`
    ToPage       int          `json:"to_page"`
    DocID        string       `json:"doc_id"`
    KbID         string       `json:"kb_id"`
    TenantID     string       `json:"tenant_id"`
    ParserConfig ParserParams `json:"parser_config"`
}

type ParserParams struct {
    ChunkTokenNum     int    `json:"chunk_token_num"`
    Delimiter         string `json:"delimiter"`
    LayoutRecognizer  string `json:"layout_recognizer"`
    TableContextSize  int    `json:"table_context_size"`
    ImageContextSize  int    `json:"image_context_size"`
    OverlappedPercent int    `json:"overlapped_percent"`
}

type ParseResponse struct {
    Success    bool    `json:"success"`
    Chunks     []Chunk `json:"chunks"`
    ChunkCount int     `json:"chunk_count"`
    DurationMs int64   `json:"duration_ms"`
    Error      string  `json:"error,omitempty"`
    Message    string  `json:"message,omitempty"`
}

type Chunk struct {
    ContentWithWeight string      `json:"content_with_weight"`
    DocID             string      `json:"doc_id"`
    DocnmKwd          string      `json:"docnm_kwd"`
    KbID              []string    `json:"kb_id"`
    PageNumInt        []int       `json:"page_num_int"`
    TopInt            []int       `json:"top_int"`
    PositionInt       []int       `json:"position_int"`
    AvailableInt      int         `json:"available_int"`
    Positions         [][6]float64 `json:"positions"`
    Image             string      `json:"image"`
    ContentLtks       string      `json:"content_ltks"`
    ContentSmLtks     string      `json:"content_sm_ltks"`
    Mom               string      `json:"mom,omitempty"`
    MomWithWeight     string      `json:"mom_with_weight,omitempty"`
}
```

## 子任务 C：Go HTTP 客户端调 Python Parse Service

**新建文件**：`internal/service/parse_client.go`

- `ParseClient` struct，baseURL + `*http.Client`（超时 80 分钟）
- `Parse(ctx, fileBytes []byte, config *ParseConfig) (*ParseResponse, error)`
- multipart/form-data POST
- 网络瞬断重试 2 次

## 子任务 D：NATS 集成 + 消费

### D1. NATS client 封装

**新建或修改**：`internal/cache/nats.go`（或直接在现有 cache 包中加）

```go
// 使用 nats.go JetStream
// go get github.com/nats-io/nats.go

type NATSClient struct {
    conn *nats.Conn
    js   nats.JetStreamContext
}

func InitNATS(url string) error
func (n *NATSClient) PullSubscribe(subject, durable string) (chan *TaskMessage, error)
func (n *NATSClient) Publish(subject string, data []byte) error
```

- 使用 JetStream pull consumer，支持 ack、重试
- subject: `task.ingest`（约定）
- durable consumer name: `ragflow-ingestor-{name}`

### D2. Go ingestor 消费 NATS

**修改**：`internal/ingestion/ingestion_service.go`

新增 `natsLoop()` goroutine：

```
natsLoop:
  for msg := range natsClient.PullSubscribe("task.ingest", durableName):
    taskJSON := parseMessage(msg.Data)
    taskCtx := &TaskContext{...}
    taskChan <- taskCtx
    msg.Ack()
```

- NATS consumer 替代了原来从 admin gRPC 接收 TASK_ASSIGNMENT 的路径
- admin gRPC 连接保持（心跳 + shutdown）
- 与现有 worker pool 无缝对接（taskChan 是同一个）

### D3. Python API server 发布到 NATS

**修改**：`api/db/services/task_service.py`

在 `bulk_insert_into_db` 并推入 Redis 之后（或替代 Redis push），新增 NATS publish：

```python
import nats

async def publish_task_to_nats(task):
    nc = await nats.connect("nats://localhost:4222")
    js = nc.jetstream()
    await js.publish("task.ingest", json.dumps(task).encode())
    await nc.close()
```

注意：Phase 1 可以先**同时保留** Redis push + NATS publish，保证 Python executor 仍能工作，Go ingestor 从 NATS 消费。

## 子任务 E：改造 executeTask — 真实 PDF 处理管线

**修改文件**：`internal/ingestion/ingestion_service.go`

替换模拟的 `executeTask()`：

```
executeTask(taskCtx):
  1. 解析 task JSON → doc_id, kb_id, tenant_id, parser_config...
  2. dao.File2Document.GetStorageAddress(doc_id) → bucket, name
  3. MinioStorage.Get(bucket, name) → file bytes
  4. 构造 ParseConfig → parseClient.Parse(ctx, pdfBytes, config)
  5. 后处理:
     for each chunk:
       - xxhash64(content_with_weight + doc_id) → chunk ID
       - kb_id = [kb_id]; create_time = now
       - if base64 image: decode → MinIO.Put → img_id → 删 image 字段
       - if mom: 构建 mother chunk
  6. engine.InsertChunks(chunks) + InsertChunks(mothers)
  7. sendTaskProgress(30→60→100)
  8. sendTaskResult(COMPLETED)
  9. 更新 document.chunk_num
```

## 子任务 F：初始化 + 启动配置

### F1. `cmd/ingestion_server.go`

新增初始化：
```go
// 初始化 NATS
natsURL := os.Getenv("NATS_URL")
if natsURL == "" { natsURL = "nats://localhost:4222" }
cache.InitNATS(natsURL)

// 初始化 parse client
parseServiceURL := os.Getenv("PDF_PARSE_SERVICE_URL")
if parseServiceURL == "" { parseServiceURL = "http://localhost:9384" }

// 注入到 ingestor
ingestor.ParseClient = service.NewParseClient(parseServiceURL)
ingestor.NATSClient = cache.GetNATS()
```

### F2. 启动 Python parse service

```bash
uv run python -m quart --app api.pdf_parse_server run --host 0.0.0.0 --port 9384
```

---

## 文件清单

| # | 操作 | 文件 | 说明 |
|---|------|------|------|
| A | **新建** | `api/pdf_parse_server.py` | Python parse HTTP 服务 (~150行) |
| B | **新建** | `internal/common/parse_types.go` | Go 共享数据结构 (~80行) |
| C | **新建** | `internal/service/parse_client.go` | HTTP client (~100行) |
| D1 | **新建** | `internal/cache/nats.go` | NATS client 封装 (~100行) |
| D2 | **修改** | `internal/ingestion/ingestion_service.go` | natsLoop + worker 对接 (~100行) |
| D3 | **修改** | `api/db/services/task_service.py` | 新增 NATS publish (~30行) |
| E | **修改** | `internal/ingestion/ingestion_service.go` | executeTask 真实管线 (~350行) |
| F1 | **修改** | `cmd/ingestion_server.go` | 初始化 NATS + ParseClient (~20行) |
| F2 | **修改** | `docker/docker-compose.yml` | 添加 NATS + pdf-parse-service 容器 |
| - | **确认** | `internal/dao/file2document.go` | GetStorageAddress 是否已实现 |
| - | **确认** | `internal/engine/engine.go` | InsertChunks 接口签名 |

## 执行顺序

```
B (数据结构)
 ├─→ A (Python 服务)
 └─→ C (HTTP client)
      └─→ E (executeTask 改造)
            ├─ 依赖 D (NATS 消费)
            └─ 依赖 F (初始化)
```

**B → A + C → D → E → F**

## 风险点

1. **NATS 可用性**：需确认 NATS server 是否已在 docker-compose 中，如没有需新增
2. **NATS JetStream**：pull consumer 需要 JetStream enabled
3. **Go NATS 库**：引入 `github.com/nats-io/nats.go`，需 `go get`
4. **Python NATS 库**：`nats-py` 需加入 `pyproject.toml` 依赖
5. **File2Document.GetStorageAddress**：Go dao 层可能未实现，需要新增
6. **Go engine InsertChunks 参数**：chunk map key 需与 ES mapping 兼容
7. **xxhash**：Go 用 `github.com/cespare/xxhash/v2`，Python 用 `xxhash.xxh64().hexdigest()`
