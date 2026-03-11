# RAGFlow MCP Skill — TODO

## 项目介绍

### RAGFlow 是什么

RAGFlow 是一个基于深度文档理解的开源 RAG（检索增强生成）引擎，支持对 PDF、Word、Excel、图片等非结构化文档进行高精度解析，并提供完整的知识库管理、混合检索（Dense + BM25 + Rerank）和 Agent 工作流能力。

核心组件：

| 组件 | 路径 | 说明 |
|------|------|------|
| API Server | `api/ragflow_server.py` | Flask/Quart 主服务 |
| 检索接口 | `POST /api/v1/retrieval` | Hybrid BM25 + 向量 RRF 融合检索 |
| 文档处理 | `rag/svr/task_executor.py` | Redis Stream 任务消费，OCR/解析/分块 |
| Agent 引擎 | `agent/canvas.py` | DSL 图引擎，支持 ReAct 循环 |
| MCP Server | `mcp/server/server.py` | Model Context Protocol 服务端 |
| 前端 | `web/` | React + TypeScript + UmiJS |

数据存储：MySQL（元数据）、Elasticsearch/Infinity（向量+全文）、MinIO（文件）、Redis（任务队列）。

### MCP Skill 子项目是什么

RAGFlow MCP Server 原本只暴露单一工具 `ragflow_retrieval`，是一个对 LLM 完全透明的黑盒——LLM 不知道有哪些知识库、不知道该如何调参、遇到空结果也不知道如何自愈。

本子项目在不改动任何现有检索逻辑的前提下，通过实现 **MCP Prompts 规范**，向接入的 LLM 动态注入一份"白盒化 SOP"，使 LLM 能够：

1. **精准路由**：根据问题语义选择正确的知识库，而非盲目全局搜索
2. **自适应调参**：根据查询类型（语义 / 精确关键词 / 探索性）调整检索参数
3. **自主自愈**：遇到空结果、认证失败、服务错误时按规则重试或降级
4. **主动探索**：通过新增的 `ragflow_list_datasets` Tool 主动刷新 KB 列表

实现方式是 **MCP Prompts**（`list_prompts` + `get_prompt`），这是 MCP 协议的标准扩展点，与任何兼容 MCP 的客户端（Claude Desktop、Cursor、Continue 等）均可配合使用。

---

## ragas 介绍

### 是什么

[ragas](https://docs.ragas.io) 是专门为 RAG 系统设计的评测框架，使用 LLM-as-Judge 范式对检索和生成质量进行自动化评分，无需人工标注大量数据。

### 核心指标

本项目使用以下两个检索侧指标：

| 指标 | 含义 | 计算方式 |
|------|------|----------|
| `context_precision` | 检索到的 chunks 中有多大比例是真正相关的 | LLM 判断每个 chunk 是否有助于回答问题，取加权平均 |
| `context_recall` | 回答问题所需的信息有多大比例被检索到了 | LLM 将参考答案拆解为若干陈述，判断每条是否能从 chunks 中归因 |

两者都高才说明检索质量好：precision 高表示"噪音少"，recall 高表示"不漏信息"。

### 为什么用 Kimi 作为 Judge

ragas 的 LLM-as-Judge 需要一个能力较强的语言模型。本项目使用 Kimi（Moonshot AI）：
- API 兼容 OpenAI 格式，集成简单
- 对中英文混合内容理解良好
- 成本相对 GPT-4 更低，适合批量评测

集成方式：通过 `langchain-openai` 的 `ChatOpenAI` 配置 Kimi 的 `base_url`，再用 `ragas.llms.LangchainLLMWrapper` 包装后传给 `evaluate()`。

---

## Benchmark 介绍

### 设计目标

量化"有 MCP Skill（精准路由）vs 无 MCP Skill（全局搜索）"对检索质量的影响，为 upstream PR 提供数据支撑。

### A/B 对比设计

```
A 组（Baseline）：模拟无 Skill 时 LLM 不知路由
  → 将问题发给所有知识库（dataset_ids = ALL）

B 组（With Routing）：模拟有 Skill 时 LLM 选对知识库
  → 将问题只发给正确的知识库（dataset_ids = [expected_id]）
```

对照的核心假设：如果 MCP Skill 能让 LLM 正确路由，则 B 组的 `context_precision` 应显著高于 A 组（噪音 chunks 被过滤掉），同时延迟更低（搜索范围缩小）。

### 测试集设计

自动创建3个独立知识库，上传各自领域的内联文档：

| 知识库 | 文档内容 | 问题数 |
|--------|----------|--------|
| `eval-python` | Python 语法、函数、类、标准库 | 7 |
| `eval-ml` | 监督/无监督学习、神经网络、正则化、评估指标 | 7 |
| `eval-cloud` | IaaS/PaaS/SaaS、Kubernetes、Docker、Serverless | 7 |

每个问题包含：
- `question`：自然语言问题
- `expected_dataset_name`：应路由到的 KB 名称
- `ground_truth`：参考答案（供 ragas recall 计算使用）

### 输出报告示例

```
| Metric            | Baseline (All KBs) | With Routing | Delta   |
|-------------------|--------------------|--------------|---------|
| Context Precision | 0.52               | 0.81         | +55.8%  |
| Context Recall    | 0.61               | 0.78         | +27.9%  |
| Avg Latency (ms)  | 312.0              | 187.0        | -40.1%  |
| Errors            | 0                  | 0            | —       |
```

---

## 已完成

- [x] **扩展 `list_datasets()` 返回字段**
  - 从 `{id, description}` 扩展为 `{id, name, description, document_count, chunk_count, embedding_model, language}`
  - 抽取 `_normalize_dataset_item()` 静态方法消除重复

- [x] **新增 `_fetch_datasets_raw()` 内部方法**
  - 统一调 `/datasets` API 并规范化字段
  - 顺带填充 `_dataset_metadata_cache`（缓存复用）
  - `list_datasets()` 和 `_build_kb_snapshot()` 均委托此方法

- [x] **新增 Tool：`ragflow_list_datasets`**
  - 让大模型主动刷新 KB 列表，返回完整元数据 JSON 数组

- [x] **实现 `@app.list_prompts()`**
  - 暴露 `ragflow_retrieval_skill` prompt，带 `intent` 可选参数

- [x] **实现 `@app.get_prompt()`**
  - 动态组装 4 模块 SOP Prompt：
    1. 当前 KB 全貌（实时调 API）
    2. 参数调优说明（hybrid 检索机制）
    3. 路由决策规则
    4. 自愈规则（0 chunk / 401 / 500 处理）
  - `intent=precise/broad/auto` 影响参数推荐值

- [x] **新增 ragas 测试依赖**（`pyproject.toml`）
  - `ragas>=0.2.0`, `openai>=1.0.0`

- [x] **新建 `test/benchmark/mcp_skill_eval.py`**
  - 自动创建3个测试 KB（Python / ML / Cloud），上传内联文档并等待解析
  - 21个预置问题，每题含 `expected_dataset` 和 `ground_truth`
  - A 组（Baseline）：全局搜索所有 KB
  - B 组（Routing）：精准路由到正确 KB
  - ragas + Kimi 计算 `context_precision` / `context_recall`
  - 输出 Markdown 对比报告

---

## 待完成

### P1 — 核心验证

- [ ] **MCP Prompts 联调**
  - 启动 MCP Server，用 MCP Inspector 或 Claude Desktop 调用
  - 验证 `list_prompts` 返回 `ragflow_retrieval_skill`
  - 验证 `get_prompt` 返回包含当前 KB 列表的动态 Prompt
  - 验证 `intent=precise/broad` 时 Prompt 内容差异

- [ ] **`ragflow_list_datasets` Tool 验证**
  - 调用后确认返回包含 `name/chunk_count/document_count` 的完整 KB 信息
  - 确认返回格式为 JSON 数组

- [ ] **运行 ragas benchmark**
  ```bash
  MOONSHOT_API_KEY=xxx uv run python test/benchmark/mcp_skill_eval.py \
      --base-url http://127.0.0.1:9380 \
      --api-key ragflow-xxx \
      --teardown
  ```
  - 预期：B 组（精准路由）`context_precision` 高于 A 组（全局搜索）
  - 将报告截图存入 `docs/` 供 PR 附图使用

### P2 — 质量提升

- [ ] **benchmark 文档内容质量**
  - 当前使用内联纯文本文档，可替换为真实 PDF/MD 文件以更接近生产场景
  - 建议：将3份文档放入 `test/benchmark/fixtures/`

- [ ] **ragas `context_precision` 需要 `reference_contexts`**
  - 当前实现只传了 `reference`（ground_truth），未传 `reference_contexts`
  - 需确认 ragas 0.2.x API 是否支持无 `reference_contexts` 的 precision 计算
  - 若不支持，需人工标注或从 B 组检索结果中选取参考 chunks

- [ ] **多 intent 对比**
  - benchmark 目前只跑 `auto` intent
  - 扩展为对比 `precise` / `broad` / `auto` 三组

### P3 — 工程完善

- [ ] **单元测试**
  ```bash
  uv run pytest test/ -k "mcp_skill" -v
  ```
  - 为 `_normalize_dataset_item`、`_assemble_sop_prompt` 编写单元测试

- [ ] **upstream PR 准备**
  - 整理 commit（`feat: add MCP Prompts for RAGFlow retrieval skill`）
  - 附 ragas benchmark 数据作为效果证明
  - 在 PR 描述中说明 `intent` 参数用法和自愈规则

---

## 关键文件

| 文件 | 说明 |
|------|------|
| `mcp/server/server.py` | MCP Server 主文件（Tool + Prompts） |
| `test/benchmark/mcp_skill_eval.py` | A/B 评测脚本 |
| `pyproject.toml` | 新增 ragas/openai 测试依赖 |

## 验证命令速查

```bash
# 启动 MCP Server（self-host 模式）
uv run mcp/server/server.py \
    --base-url http://127.0.0.1:9380 \
    --mode=self-host \
    --api-key=ragflow-xxx

# 运行 A/B benchmark（需 MOONSHOT_API_KEY）
MOONSHOT_API_KEY=xxx uv run python test/benchmark/mcp_skill_eval.py \
    --base-url http://127.0.0.1:9380 \
    --api-key ragflow-xxx \
    --teardown

# 仅跑延迟对比（跳过 ragas）
uv run python test/benchmark/mcp_skill_eval.py \
    --base-url http://127.0.0.1:9380 \
    --api-key ragflow-xxx \
    --skip-ragas

# 单元测试
uv run pytest test/ -k "mcp_skill" -v
```
