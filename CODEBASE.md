# RAGFlow Codebase Explain

## 项目简介

RAGFlow 是一个基于深度文档理解的开源 RAG（检索增强生成）引擎。核心理念是 **"Quality in, quality out"**——从复杂的非结构化文档中提取可溯源、有引用的高质量答案。

**版本:** v0.18.0 | **许可证:** Apache 2.0

---

## 目录结构总览

```
ragflow/
├── api/                # Flask 后端 REST API
│   ├── apps/           #   各模块 API 端点
│   ├── db/             #   数据库模型、服务层、迁移
│   ├── utils/          #   工具函数
│   ├── ragflow_server.py  # 主服务入口
│   └── settings.py     #   全局配置
├── rag/                # 核心 RAG 引擎
│   ├── app/            #   按文档类型的解析器（naive, paper, book, resume 等）
│   ├── llm/            #   LLM 接口（chat, embedding, rerank, TTS, ASR）
│   ├── nlp/            #   检索、NLP 工具、分词
│   ├── svr/            #   异步任务执行器
│   └── utils/          #   存储连接（ES, Infinity, S3, MinIO 等）
├── deepdoc/            # 深度文档理解
│   ├── vision/         #   OCR、版面识别、表格结构识别
│   └── parser/         #   格式解析器（PDF, DOCX, Excel, HTML 等）
├── agent/              # Agent / 工作流系统
│   ├── component/      #   可组合的工作流组件
│   ├── canvas.py       #   Canvas 图执行引擎
│   └── templates/      #   预置 Agent 模板
├── graphrag/           # 知识图谱抽取与检索
│   ├── general/        #   完整知识图谱流水线
│   ├── light/          #   轻量级版本
│   └── search.py       #   基于图的检索
├── web/                # React/TypeScript 前端
│   └── src/            #   页面、组件、服务
├── sdk/                # Python SDK
├── mcp/                # Model Context Protocol 服务器
├── docker/             # Docker 部署配置
├── helm/               # Kubernetes Helm Charts
└── conf/               # 配置文件
```

---

## 技术栈

### 后端

| 类别 | 技术 |
|------|------|
| Web 框架 | Flask 3.0 |
| ORM | Peewee（支持 MySQL / PostgreSQL） |
| 向量/全文检索 | Elasticsearch 8.x / Infinity / OpenSearch |
| 缓存与消息队列 | Redis（Valkey 8.0） |
| 对象存储 | MinIO / AWS S3 / Azure Blob / OSS |
| LLM | OpenAI, Anthropic, Ollama, Qianwen 等 100+ 模型 |
| Embedding | BAAI/bge-large-zh-v1.5, BCE, FastEmbed |
| 文档解析 | PDFPlumber, python-docx, openpyxl, Tika |
| OCR/视觉 | PaddleOCR, 表格结构识别模型 |

### 前端

| 类别 | 技术 |
|------|------|
| 框架 | Umi 4.0 (React) |
| UI 组件 | Ant Design 5.x, Radix UI |
| 样式 | Tailwind CSS 3 |
| 状态管理 | Zustand |
| 图可视化 | @antv/g6, @xyflow/react |
| 代码编辑器 | Monaco Editor |

### 部署

| 类别 | 技术 |
|------|------|
| 容器化 | Docker（多阶段构建） |
| 编排 | Docker Compose / Kubernetes (Helm) |
| 反向代理 | Nginx |

---

## 系统架构

### 整体数据流

```
┌─────────────┐     HTTP/WS      ┌──────────────────┐
│  Web 前端    │ ◄──────────────► │  Flask REST API   │
│  (React)    │                   │  (api/apps/)      │
└─────────────┘                   └────────┬─────────┘
                                           │
                    ┌──────────────────────┼──────────────────────┐
                    ▼                      ▼                      ▼
           ┌──────────────┐     ┌──────────────────┐    ┌──────────────┐
           │   MySQL/PG   │     │   Elasticsearch  │    │    MinIO      │
           │  (元数据)     │     │  (向量+全文检索)  │    │  (文件存储)   │
           └──────────────┘     └──────────────────┘    └──────────────┘
                                        ▲
                                        │
                               ┌────────┴────────┐
                               │   RAG Engine     │
                               │   (rag/)         │
                               ├─────────────────┤
                               │ • 文档解析       │
                               │ • 分块 & Embedding│
                               │ • 混合检索       │
                               │ • 重排序         │
                               └────────┬────────┘
                                        │
                    ┌───────────────────┼───────────────────┐
                    ▼                   ▼                    ▼
            ┌─────────────┐    ┌──────────────┐    ┌──────────────┐
            │  DeepDoc    │    │  GraphRAG    │    │  Agent 系统   │
            │  (文档理解)  │    │  (知识图谱)   │    │  (Canvas)    │
            └─────────────┘    └──────────────┘    └──────────────┘
```

### 核心模块详解

#### 1. API 层 (`api/`)

Flask 后端，负责所有对外的 RESTful 接口。

- **`api/ragflow_server.py`** — 服务主入口，初始化数据库、加载插件、启动异步任务执行器
- **`api/apps/`** — 按业务域划分的端点：
  - `kb_app.py` — 知识库 CRUD
  - `document_app.py` — 文档上传、解析、管理
  - `dialog_app.py` — 对话应用配置
  - `conversation_app.py` — 会话历史管理
  - `chunk_app.py` — 分块检索与搜索
  - `canvas_app.py` — Agent 工作流管理
- **`api/db/`** — 数据层：
  - `db_models.py` — Peewee ORM 模型定义
  - `services/` — 业务逻辑服务层

#### 2. RAG 引擎 (`rag/`)

系统的核心——检索增强生成流水线。

- **`rag/app/`** — 按文档类型定制的解析器：
  - `naive.py` — 通用文档解析（默认）
  - `paper.py` — 学术论文
  - `book.py` — 书籍
  - `resume.py` — 简历
  - `laws.py` — 法律文书
  - `qa.py` — 问答对
  - `table.py` — 表格数据
  - `audio.py` — 音频转文本
  - `email.py` — 邮件解析
  - `picture.py` — 图片解析
- **`rag/llm/`** — LLM 统一接口抽象：
  - Chat 模型、Embedding 模型、Rerank 模型、TTS/ASR
  - 支持 100+ 模型供应商
- **`rag/nlp/search.py`** — **检索调度器 (`Dealer` 类)**，编排混合搜索：
  1. 向量搜索（语义相似度）
  2. 全文搜索（BM25 关键词匹配）
  3. 得分融合（可配置向量权重，默认 0.3）
  4. 过滤（按知识库、文档、标签）
  5. 重排序（可选 Cross-Encoder）
- **`rag/svr/task_executor.py`** — 异步任务执行器，监听 Redis 队列处理文档

#### 3. 深度文档理解 (`deepdoc/`)

负责从各种格式的文档中提取结构化信息。

- **`deepdoc/vision/`** — 视觉 AI：
  - OCR 文字识别
  - 版面分析（识别标题、段落、表格、图片等区域）
  - 表格结构识别
- **`deepdoc/parser/`** — 格式解析器：
  - PDF、DOCX、Excel、HTML、JSON、Markdown 等
  - 保留文档结构和空间信息
  - 多语言支持

#### 4. 知识图谱 (`graphrag/`)

从文档中抽取实体和关系，构建知识图谱以支持更复杂的推理。

- 实体抽取与链接
- 关系发现
- 社区检测（Leiden 算法）
- 基于图的检索与上下文扩展
- 思维导图可视化

#### 5. Agent 系统 (`agent/`)

可视化工作流编排系统，基于 Canvas（画布）构建复杂 RAG 流水线。

- **`agent/canvas.py`** — 图执行引擎（支持 DAG 和循环图）
- **`agent/component/`** — 可组合组件：
  - 输入：`Begin`, `Input`
  - 处理：`Retrieval`, `Generate`, `Categorize`, `SQL Execute`
  - 集成：Web 爬虫、邮件、API 调用、网页搜索
  - 输出：`Answer`
- **`agent/templates/`** — 预置模板，开箱即用

#### 6. 前端 (`web/`)

基于 React 和 Umi 框架的单页应用。

- 知识库管理界面
- 文档上传与处理状态
- 对话界面（支持流式响应）
- Canvas 可视化工作流编辑器
- 系统设置与模型管理

---

## RAG 完整流水线

```
    ┌──────────────────── 文档入库 ────────────────────┐
    │                                                   │
    │  上传文件 → MinIO 存储 → DeepDoc 分析             │
    │      → 类型检测 → 选择对应解析器                   │
    │      → 文本/表格/图片提取                          │
    │      → 按策略分块                                  │
    │      → 生成 Embedding 向量                         │
    │      → 提取关键词和实体                            │
    │      → 写入 Elasticsearch（分块 + 向量）           │
    │                                                   │
    └───────────────────────────────────────────────────┘

    ┌──────────────────── 查询处理 ────────────────────┐
    │                                                   │
    │  用户提问 → 问题解析与改写                         │
    │      → 生成查询向量                                │
    │      → 混合检索：                                  │
    │          ├─ 向量相似度搜索                         │
    │          ├─ 全文关键词搜索                         │
    │          └─ 实体/标签过滤                          │
    │      → 融合排序                                    │
    │      → Cross-Encoder 重排序（可选）                │
    │      → 选取 Top-K 分块                             │
    │                                                   │
    └───────────────────────────────────────────────────┘

    ┌──────────────────── 生成回答 ────────────────────┐
    │                                                   │
    │  组装 Prompt（上下文 + 对话历史 + 系统指令）       │
    │      → 调用 LLM（支持流式输出）                   │
    │      → 插入引用标注（可追溯到源文档）              │
    │      → 返回带引用的答案                            │
    │                                                   │
    └───────────────────────────────────────────────────┘
```

---

## 数据模型

### 核心实体关系

```
User（用户）
 └─ Tenant（租户，多租户架构）
     ├─ Knowledgebase（知识库）
     │   └─ Document（文档）
     │       ├─ Task（处理任务）
     │       └─ Chunk（分块，存储在 ES 中）
     ├─ Dialog（对话应用配置）
     │   └─ Conversation（会话）
     │       └─ Message（消息记录）
     └─ UserCanvas（Agent 工作流）
         └─ CanvasVersion（版本记录）
```

### 关键模型说明

| 模型 | 说明 | 核心字段 |
|------|------|----------|
| `Knowledgebase` | 文档集合 | name, parser_id, embd_id, similarity_threshold |
| `Document` | 文件记录 | name, location, kb_id, parser_id, status, progress, token_num, chunk_num |
| `Task` | 异步任务 | doc_id, from_page, to_page, priority, status, progress |
| `Dialog` | 对话应用 | name, llm_id, kb_ids, prompt_config, top_n, rerank_id |
| `Conversation` | 会话 | dialog_id, messages, references |
| `UserCanvas` | 工作流定义 | dsl (DAG), title, permission |

### Chunk 存储（Elasticsearch）

分块数据不在 MySQL 中，而是存储在 Elasticsearch：

```json
{
  "docnm_kwd": "文件名",
  "content_ltks": "文本内容",
  "kb_id": "知识库ID",
  "doc_id": "文档ID",
  "page_num_int": 1,
  "q_{dim}_vec": [0.1, 0.2, ...],
  "important_kwd": ["关键词"],
  "question_kwd": ["生成的问题"]
}
```

---

## API 接口一览

### 知识库管理

```
POST   /kb/create        创建知识库
GET    /kb/list           列出知识库
DELETE /kb/delete         删除知识库
POST   /kb/update         更新知识库配置
GET    /kb/detail         获取知识库详情
```

### 文档管理

```
POST   /doc/upload        上传文档
POST   /doc/web_crawl     爬取网页为文档
GET    /doc/list           列出文档
DELETE /doc/delete         删除文档
GET    /doc/detail         获取文档详情
```

### 检索与搜索

```
POST   /chunk/search      搜索分块
POST   /chunk/retrieval   带排序的检索
GET    /chunk/list         列出文档的分块
```

### 对话管理

```
POST   /dialog/set        创建/更新对话应用
GET    /dialog/list        列出对话应用
POST   /conversation/set  创建/更新会话
POST   /conversation/message    发送消息
POST   /conversation/stream     流式消息
```

### Agent 工作流

```
POST   /canvas/create     创建工作流
GET    /canvas/get         获取工作流
POST   /canvas/run         执行工作流
```

### SDK API

```
POST   /datasets               创建数据集
GET    /datasets               列出数据集
POST   /documents/:id/chunks   获取文档分块
POST   /conversations/:id/msg  发送会话消息
```

---

## 本地开发

### 前置条件

- Python 3.10 - 3.12
- Node.js 18.20.4+
- Docker & Docker Compose

### 启动步骤

```bash
# 1. 安装 Python 依赖
uv sync --python 3.10 --all-extras
uv run download_deps.py

# 2. 启动基础设施（MySQL, ES, Redis, MinIO）
docker compose -f docker/docker-compose-base.yml up -d

# 3. 启动后端
bash docker/launch_backend_service.sh

# 4. 启动前端（另一个终端）
cd web && npm install && npm run dev
```

### Docker 一键部署

```bash
docker compose -f docker/docker-compose.yml up -d
```

访问 `http://localhost` 即可使用。

---

## 项目亮点

1. **深度文档理解** — DeepDoc 结合视觉 AI，处理扫描件、复杂表格、多栏布局
2. **混合检索** — 向量 + 全文 + 重排序，确保检索质量
3. **知识图谱** — 实体关系抽取，支持多跳推理
4. **可视化 Agent** — Canvas 画布式工作流编排，灵活组合 RAG 流水线
5. **多租户 SaaS** — 完整的认证、授权、租户隔离体系
6. **广泛 LLM 支持** — 100+ 模型，多供应商适配
7. **生产级部署** — Docker / Kubernetes 支持，Nginx 反向代理
8. **开放生态** — Python SDK、MCP Server、插件系统
