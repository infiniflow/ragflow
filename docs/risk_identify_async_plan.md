# 风险识别异步化整改方案

## 1. 目标

- 支持大规模风险识别请求，避免前端长时间等待或浏览器超时。
- 提升高并发场景吞吐量，保证模型与检索服务稳定性。
- 让用户可追踪处理进度并在任务完成后下载结果。

## 2. 当前流程回顾

| 阶段 | 说明 |
| --- | --- |
| 上传文件 | `POST /v1/kb/risk_identify` 返回表格行数据（循环、主要风险点、相应的内部控制）。 |
| 单行检索 | 前端调用 `POST /v1/kb/risk_retrieval` 获取命中片段。 |
| AI 识别 | 前端调用 `POST /v1/kb/risk_ai_identify`，返回单条 LLM 输出。 |
| 一键识别 | 前端直接调用 `POST /v1/kb/risk_ai_identify_batch`，同步等待生成 Excel 并触发下载。 |

痛点：批量识别操作完全在同步请求内完成，容易超时；多人并发会大量占用模型资源与 Redis 流。

## 3. 新的异步处理架构

```
前端 -> /kb/risk_ai_identify_task (创建任务)
        <- 返回 task_id
任务轮询 -> /kb/risk_ai_identify_task/status?task_id=xxx
任务完成 -> 返回结果下载地址 (对象存储)

后台 Worker (task_executor) 从 Redis 队列消费 risk_ai_identify 任务：
  - 批量检索 -> in-memory/cache
  - 分批调用 LLM
  - 生成 Excel -> 对象存储
  - 更新任务状态、进度、错误信息
```

### 3.1 数据结构

1. **新增表 `risk_ai_task`**
   - `id` (UUID)
   - `kb_id`
   - `status` (`pending`/`running`/`success`/`failed`)
   - `progress` (0~100)
   - `total_rows`, `processed_rows`, `failed_rows`
   - `result_location` (存储路径)
   - `error_msg`
   - `created_by`, `created_at`, `updated_at`
   - `params`（JSON，保存行数据、阈值、模型温度等）

2. **Redis 队列**：沿用 `rag_flow_svr_queue`，在 `message` 中新增 `task_type = "risk_ai_identify_batch"`。

### 3.2 后端改造任务

1. **任务创建接口** `POST /v1/kb/risk_ai_identify_task`
   - 入参：`kb_id`、`rows`、识别参数（阈值、温度等）
   - 写入 `risk_ai_task`，状态 `pending`，推送队列
   - 返回 `task_id`

2. **任务状态接口** `GET /v1/kb/risk_ai_identify_task/status?task_id=xxx`
   - 返回状态、进度、已处理条数、失败详情、结果下载 URL

3. **Worker 处理逻辑**（`task_executor` 新增 handler）
   - 从任务表读取待处理行（分页/批量）
   - 使用 `risk_retrieval` 逻辑（可缓存）获取检索结果
   - 调用 LLM 生成结构化 JSON，解析并构建 Excel 行
   - 定期 `set_progress`，更新任务表
   - 成功：excel 写入对象存储，更新 `result_location`
   - 失败：记录 error，设置 `status = failed`

4. **复用现有工具**
   - 数据库访问：复用 peewee 模型
   - Excel 处理：沿用当前的 `openpyxl` 逻辑
   - 存储：`STORAGE_IMPL.put`

### 3.3 前端改造

1. **创建任务**
   - “一键AI识别并导出”按钮 -> 调用 `/risk_ai_identify_task`
   - 保存返回的 `task_id`，跳转/展示任务卡片

2. **任务轮询**
   - 每 3~5s 拉取 `status` 接口，更新进度条与行统计
   - 失败行显示错误，可提供“重试失败行”按钮（提交新任务，仅包含失败行）

3. **完成后**
   - 显示“下载结果”按钮（直接访问后端返回的下载 URL）
   - 任务列表支持刷新/查看历史任务

4. **UX**
   - 任务卡片包含：状态、进度百分比、总行数、成功/失败数、创建时间
   - 提供“取消任务”按钮（可选，后端在 Redis 中标记取消）

### 3.4 共享优化点

- 检索缓存：`question + 阈值` 缓存到 Redis/任务表，避免重复调用。
- LLM rate limit：在 worker 中使用 `trio.Semaphore` 控制并发；失败自动重试（指数退避）。
- 超时控制：对单批次设置超时，失败时记录并继续后续行。
- 日志：为每行写入详细日志（成功/失败原因），前端可展示。

## 4. 工期与分工建议

| 阶段 | 工作项 | 预估 |
| --- | --- | --- |
| 1 | DB模型、接口设计 (任务表、API 定义) | 0.5d |
| 2 | 后端开发 (任务接口、task_executor handler) | 1.5d |
| 3 | 前端改造 (提交/轮询/下载/历史) | 1d |
| 4 | 联调与压测 (长任务、多任务、重试) | 1d |

## 5. 验收要点

- 大量行数据时请求不超时，任务可持续运行
- 多用户同时提交时队列可顺序处理，模型服务未被压垮
- 任务状态与下载文件准确，失败行能清晰提示
- 旧有单条识别功能可继续使用

---

> 文档若有更新，会同步此处。下一步可按上述任务拆分实现。

## 6. 下一轮任务拆解

1. **数据行任务化**
   - 新增 `risk_ai_task_row` 表（或先手动建表）记录每一行的 payload、状态、LLM 输出。
   - 创建批量任务时：遍历 rows 写入行记录，状态 `pending`，同时针对每行推送 `risk_ai_identify_row` 消息到 Redis，以便多个 executor 并行消费。

2. **Worker 行级处理**
   - `task_executor.collect` 支持识别 `risk_ai_identify_row`。
   - 新增行处理函数：读取行记录 → 检索 → 调用 LLM → 写回 `result/status`，并更新主任务的 `processed_rows`/`failed_rows`。
   - 当所有行完成时，最后一个 worker 负责汇总 Excel、写入对象存储、更新主任务状态与 `result_location`。

3. **状态与下载接口**
   - `/risk_ai_identify_task/status` 基于行记录统计进度，返回成功/失败行数及错误信息。
   - `/risk_ai_identify_task/download` 读取汇总后的 Excel 并下发，供前端“下载结果”按钮使用。

4. **前端适配**
   - 保留现有创建接口，轮询 `/status` 展示进度与错误明细。
   - 任务成功后使用新的 `/download` 接口下载结果；若失败提供“重试失败行”入口。

5. **测试与验证**
   - 长任务、大批量、多任务并发、失败重试等场景。
   - 校验导出的 Excel 内容顺序与输入一致。
