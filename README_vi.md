# RAGFlow

<div align="center">

**RAGFlow** là một engine RAG (Retrieval-Augmented Generation) mã nguồn mở hàng đầu, kết hợp công nghệ RAG tiên tiến với khả năng Agent để tạo ra lớp ngữ cảnh vượt trội cho các mô hình ngôn ngữ lớn (LLM). Được phát triển bởi **InfiniFlow**, RAGFlow cung cấp quy trình RAG tinh gọn, phù hợp với doanh nghiệp ở mọi quy mô.

Phiên bản hiện tại: **v0.25.6**

</div>

---

## Mục lục

- [Giới thiệu](#giới-thiệu)
- [Tính năng nổi bật](#tính-năng-nổi-bật)
- [Cập nhật mới nhất](#cập-nhật-mới-nhất)
- [Kiến trúc hệ thống](#kiến-trúc-hệ-thống)
- [Cấu trúc dự án](#cấu-trúc-dự-án)
- [Công nghệ sử dụng](#công-nghệ-sử-dụng)
- [Yêu cầu hệ thống](#yêu-cầu-hệ-thống)
- [Cài đặt và Triển khai](#cài-đặt-và-triển-khai)
  - [Triển khai bằng Docker (khuyến nghị)](#triển-khai-bằng-docker-khuyến-nghị)
  - [Chạy từ mã nguồn để phát triển](#chạy-từ-mã-nguồn-để-phát-triển)
- [Cấu hình](#cấu-hình)
- [SDK Python](#sdk-python)
- [Tài liệu](#tài-liệu)
- [Cộng đồng](#cộng-đồng)
- [Đóng góp](#đóng-góp)
- [Giấy phép](#giấy-phép)

---

## Giới thiệu

[RAGFlow](https://ragflow.io/) là một engine RAG mã nguồn mở dựa trên công nghệ **hiểu tài liệu chuyên sâu (deep document understanding)**. Khi tích hợp với các mô hình ngôn ngữ lớn (LLM), hệ thống có khả năng cung cấp các câu trả lời chính xác, có trích dẫn rõ ràng từ nhiều định dạng dữ liệu phức tạp khác nhau.

Điểm khác biệt cốt lõi của RAGFlow:

- **"Chất lượng đầu vào, chất lượng đầu ra"** — Trích xuất tri thức từ dữ liệu phi cấu trúc với định dạng phức tạp nhờ DeepDoc.
- **"Tìm kim trong đống cỏ"** — Truy xuất chính xác thông tin trong khối lượng token khổng lồ.
- **Công cụ Agent có sẵn** — Cho phép tạo các tác nhân AI với quy trình làm việc phức tạp.

## Tính năng nổi bật

### Trích xuất tri thức chuyên sâu
- Hiểu tài liệu chuyên sâu ([DeepDoc](./deepdoc/README.md)) từ dữ liệu phi cấu trúc với định dạng phức tạp.
- Khả năng tìm kiếm thông tin "kim trong đống cỏ" với lượng token không giới hạn.

### Chunking dựa trên template
- Thông minh và có khả năng giải thích được.
- Nhiều tùy chọn template để lựa chọn theo nhu cầu.

### Trích dẫn có căn cứ, giảm thiểu hallucination
- Trực quan hóa quá trình phân đoạn văn bản, cho phép con người can thiệp.
- Xem nhanh các tham chiếu quan trọng và truy nguyên được nguồn trích dẫn.

### Tương thích đa dạng nguồn dữ liệu
- Hỗ trợ: Word, Slides, Excel, TXT, hình ảnh, bản scan, dữ liệu có cấu trúc, trang web, v.v.
- Đồng bộ dữ liệu từ **Confluence, S3, Notion, Discord, Google Drive**.

### Quy trình RAG tự động
- Quy trình RAG được tinh gọn cho cả cá nhân và doanh nghiệp lớn.
- Cấu hình linh hoạt LLM cũng như mô hình embedding.
- Đa truy xuất kết hợp re-ranking.

### Agent và MCP
- Hỗ trợ **quy trình làm việc có tác nhân (agentic workflow)**.
- Tích hợp **MCP (Model Context Protocol)**.
- Thành phần thực thi mã Python/JavaScript trong Agent.
- Hỗ trợ **Memory** cho AI agent.

## Cập nhật mới nhất

- **2026-04-24** — Hỗ trợ DeepSeek v4.
- **2026-03-24** — RAGFlow Skill trên OpenClaw.
- **2025-12-26** — Hỗ trợ "Memory" cho AI agent.
- **2025-11-19** — Hỗ trợ Gemini 3 Pro.
- **2025-11-12** — Đồng bộ dữ liệu từ Confluence, S3, Notion, Discord, Google Drive.
- **2025-10-23** — Hỗ trợ MinerU và Docling làm phương pháp phân tích tài liệu.
- **2025-10-15** — Hỗ trợ pipeline ingestion có thể điều phối.
- **2025-08-08** — Hỗ trợ dòng model GPT-5 mới nhất của OpenAI.
- **2025-08-01** — Hỗ trợ quy trình agentic và MCP.
- **2025-05-23** — Thêm thành phần thực thi mã Python/JavaScript vào Agent.
- **2025-05-05** — Hỗ trợ truy vấn đa ngôn ngữ.
- **2025-03-19** — Sử dụng mô hình đa phương thức để hiểu hình ảnh trong PDF/DOCX.

## Kiến trúc hệ thống

RAGFlow là hệ thống microservice dựa trên Docker với các thành phần chính:

```
+-----------------+   +------------------+   +-----------------+
|  Frontend (Web) | --|   API Server     | --|  Core RAG Logic |
|  React + UmiJS  |   |  Flask/Quart     |   |  rag/           |
+-----------------+   +------------------+   +-----------------+
                              |
                +-------------+-------------+
                v             v             v
        +-----------+  +-------------+  +-----------+
        |   MySQL   |  | ES/Infinity |  |   MinIO   |
        |           |  |  + Vector   |  |  Storage  |
        +-----------+  +-------------+  +-----------+
                              |
                +-------------+-------------+
                v                           v
        +---------------+         +----------------+
        |   DeepDoc     |         |  Agent/SDK/MCP |
        | (OCR/Parse)   |         |                |
        +---------------+         +----------------+
```

## Cấu trúc dự án

```
ragflow/
├── api/              # Backend API server (Flask/Quart)
│   ├── apps/         # API Blueprints (Knowledge Base, Chat,...)
│   │   ├── auth/         # Xác thực người dùng
│   │   ├── restful_apis/ # Các RESTful API endpoints
│   │   └── services/     # Logic nghiệp vụ
│   ├── db/           # Models và services cho database
│   ├── common/       # Tiện ích dùng chung
│   └── ragflow_server.py  # Server chính
│
├── rag/              # Core RAG logic
│   ├── app/          # Các app RAG (naive, qa, paper, table,...)
│   ├── llm/          # Trừu tượng hóa LLM, Embedding, Rerank
│   ├── flow/         # Quy trình xử lý
│   ├── graphrag/     # Graph-based RAG
│   ├── advanced_rag/ # Các kỹ thuật RAG nâng cao
│   ├── nlp/          # Xử lý ngôn ngữ tự nhiên
│   └── svr/          # Các service con (task executor,...)
│
├── deepdoc/          # Phân tích tài liệu và OCR
│   ├── parser/       # Parser cho PDF, DOCX, Excel, PPT,...
│   └── vision/       # OCR, layout recognition, TSR
│
├── agent/            # Thành phần Agentic
│   ├── component/    # Begin, LLM, Retrieval, Switch, Loop,...
│   ├── plugin/       # Plugin mở rộng
│   ├── sandbox/      # Môi trường sandbox thực thi mã
│   └── tools/        # Công cụ: Google, GitHub, Wikipedia,...
│
├── web/              # Frontend (React + UmiJS + Vite)
│   ├── src/
│   │   ├── pages/        # Trang: home, dataset, chat, agent,...
│   │   ├── components/   # UI components
│   │   ├── services/     # API services
│   │   ├── hooks/        # React hooks (TanStack Query)
│   │   ├── locales/      # Đa ngôn ngữ (i18n)
│   │   └── ...
│   └── package.json
│
├── docker/           # Cấu hình Docker
│   ├── docker-compose.yml
│   ├── docker-compose-base.yml
│   ├── .env
│   └── service_conf.yaml.template
│
├── sdk/              # Python SDK cho RAGFlow
│   └── python/ragflow_sdk/
│       ├── modules/      # Dataset, Document, Chat, Agent, Memory
│       └── ragflow.py    # Lớp RAGFlow chính
│
├── mcp/              # Model Context Protocol
│   ├── client/
│   └── server/
│
├── memory/           # Dịch vụ bộ nhớ cho agent
├── sandbox/          # Môi trường thực thi mã an toàn
├── common/           # Tiện ích dùng chung
├── conf/             # File cấu hình (mapping, models,...)
├── docs/             # Tài liệu (MDX)
├── helm/             # Helm chart cho Kubernetes
├── admin/            # Admin service
├── internal/         # Logic nội bộ
├── tools/            # Tiện ích CLI
├── test/             # Test cho backend
├── examples/         # Ví dụ sử dụng
├── pyproject.toml    # Cấu hình Python project (uv)
├── go.mod            # Một số thành phần phụ trợ viết bằng Go
├── Dockerfile        # Image Docker chính
└── docker-compose*.yml
```

## Công nghệ sử dụng

### Backend (Python 3.13+)
- **Web framework**: Flask, Quart (async)
- **API documentation**: Flasgger (Swagger)
- **Database**: MySQL
- **Search/Vector engine**: Elasticsearch 8.11+ / Infinity
- **Object storage**: MinIO
- **Cache/Session**: Redis
- **LLM SDKs**: OpenAI, Anthropic Claude, Google Gemini, Mistral, Cohere, Groq, DeepSeek, Tongyi Qianwen, Ollama,...
- **OCR/Parse**: PaddleOCR, Docling, MinerU, OpenDataLoader
- **gRPC**: Kết nối giữa các service con
- **Async**: asyncio, aiohttp

### Frontend (TypeScript + React)
- **Framework**: UmiJS / Vite
- **UI Components**: shadcn/ui, Radix UI, Ant Design Icons
- **Styling**: Tailwind CSS, Less
- **State management**: Zustand
- **Data fetching**: TanStack Query (React Query)
- **i18n**: react-i18next
- **Form**: react-hook-form
- **Charts**: AntV G2/G6
- **Editor**: Monaco Editor, Lexical
- **Testing**: Jest

### Hạ tầng
- **Containerization**: Docker, Docker Compose
- **Orchestration**: Kubernetes (Helm chart)
- **MCP**: Model Context Protocol
- **Sandbox**: gVisor (cho code executor)
- **Quản lý phụ thuộc Python**: uv
- **Một số module phụ trợ**: Go

## Yêu cầu hệ thống

| Thành phần   | Yêu cầu tối thiểu                    |
|--------------|---------------------------------------|
| CPU          | >= 4 cores                            |
| RAM          | >= 16 GB                              |
| Ổ đĩa        | >= 50 GB                              |
| Docker       | >= 24.0.0                             |
| Docker Compose | >= v2.26.1                          |
| Python       | >= 3.13                               |
| gVisor       | Chỉ cần nếu dùng tính năng code executor (sandbox) |

> **Lưu ý:** Tất cả Docker image hiện được build cho nền tảng **x86**. Nếu bạn dùng ARM64, vui lòng tự build image từ mã nguồn.

## Cài đặt và Triển khai

### Triển khai bằng Docker (khuyến nghị)

Đây là cách nhanh nhất để trải nghiệm RAGFlow.

**Bước 1:** Đảm bảo `vm.max_map_count` >= 262144 (Linux):

```bash
sudo sysctl -w vm.max_map_count=262144
```

Để thay đổi vĩnh viễn, thêm vào `/etc/sysctl.conf`:
```
vm.max_map_count=262144
```

**Bước 2:** Clone repository:

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow
```

**Bước 3:** Khởi động server bằng Docker image có sẵn:

```bash
cd docker

# Dùng CPU cho DeepDoc:
docker compose -f docker-compose.yml up -d

# Hoặc dùng GPU (NVIDIA) để tăng tốc:
sed -i '1i DEVICE=gpu' .env
docker compose -f docker-compose.yml up -d
```

**Bước 4:** Kiểm tra trạng thái server:

```bash
docker logs -f docker-ragflow-cpu-1
```

Khi server khởi động thành công, bạn sẽ thấy log tương tự:

```
     ____   ___    ______ ______ __
    / __ \ /   |  / ____// ____// /____  _      __
   / /_/ // /| | / / __ / /_   / // __ \| | /| / /
  / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
 /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

 * Running on all addresses (0.0.0.0)
```

**Bước 5:** Mở trình duyệt và truy cập `http://IP_MAY_CUA_BAN` (mặc định port 80).

**Bước 6:** Trong file `docker/service_conf.yaml.template`, chọn LLM factory mong muốn ở `user_default_llm` và cập nhật `API_KEY` tương ứng.

### Chạy từ mã nguồn để phát triển

**Bước 1:** Cài đặt `uv` và `pre-commit`:

```bash
pipx install uv pre-commit
```

**Bước 2:** Clone và cài đặt các phụ thuộc Python:

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow
uv sync --python 3.13
uv run python3 download_deps.py
pre-commit install
```

**Bước 3:** Khởi động các dịch vụ phụ thuộc (MinIO, Elasticsearch, Redis, MySQL):

```bash
docker compose -f docker/docker-compose-base.yml up -d
```

Thêm vào `/etc/hosts`:
```
127.0.0.1   es01 infinity mysql minio redis sandbox-executor-manager
```

**Bước 4:** Nếu không truy cập được HuggingFace, đặt biến môi trường:

```bash
export HF_ENDPOINT=https://hf-mirror.com
```

**Bước 5:** Cài đặt jemalloc:

```bash
# Ubuntu
sudo apt-get install libjemalloc-dev
# CentOS
sudo yum install jemalloc
# macOS
brew install jemalloc
```

**Bước 6:** Khởi động backend:

```bash
source .venv/bin/activate
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh
```

**Bước 7:** Cài đặt và khởi động frontend:

```bash
cd web
npm install
npm run dev
```

Frontend mặc định chạy ở `http://localhost:9222`.

**Bước 8:** Dừng dịch vụ sau khi phát triển xong:

```bash
pkill -f "ragflow_server.py|task_executor.py"
```

## Cấu hình

Có 3 file cấu hình chính cần quản lý:

| File                                              | Mô tả                                                       |
|---------------------------------------------------|-------------------------------------------------------------|
| `docker/.env`                                     | Cấu hình nền tảng: `SVR_HTTP_PORT`, `MYSQL_PASSWORD`,...    |
| `docker/service_conf.yaml.template`               | Cấu hình các dịch vụ backend                                |
| `docker/docker-compose.yml`                       | Cấu hình các container                                      |

Để thay đổi cổng HTTP mặc định (80), sửa trong `docker-compose.yml`: `80:80` → `<YOUR_PORT>:80`.

Sau khi thay đổi cấu hình, cần khởi động lại:

```bash
docker compose -f docker-compose.yml up -d
```

### Chuyển doc engine từ Elasticsearch sang Infinity

RAGFlow mặc định dùng Elasticsearch. Để chuyển sang [Infinity](https://github.com/infiniflow/infinity/):

1. Dừng tất cả container:
   ```bash
   docker compose -f docker/docker-compose.yml down -v
   ```
2. Đặt `DOC_ENGINE=infinity` trong `docker/.env`.
3. Khởi động lại:
   ```bash
   docker compose -f docker-compose.yml up -d
   ```

> **Cảnh báo:** Chuyển sang Infinity trên Linux/arm64 hiện chưa được hỗ trợ chính thức.

## SDK Python

RAGFlow cung cấp Python SDK để tích hợp vào ứng dụng:

```python
from ragflow_sdk import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://localhost:9380")

# Liệt kê các dataset
datasets = rag.list_datasets()

# Tạo dataset mới
dataset = rag.create_dataset(name="my_dataset")

# Upload tài liệu
dataset.upload_file("path/to/file.pdf")

# Tạo chat assistant
chat = rag.create_chat(
    name="my_chat",
    dataset_ids=[dataset.id]
)

# Hỏi đáp
response = chat.ask("Nội dung câu hỏi của bạn", stream=False)
print(response.answer)
```

Các module SDK bao gồm:
- **Dataset** — Quản lý dataset
- **Document** — Quản lý tài liệu
- **Chunk** — Quản lý đoạn văn bản
- **Chat** — Chat assistant
- **Agent** — Tác nhân AI
- **Session** — Phiên làm việc
- **Memory** — Bộ nhớ agent

## Tài liệu

- [Quickstart](https://ragflow.io/docs/dev/)
- [Cấu hình](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [Hướng dẫn sử dụng](https://ragflow.io/docs/category/user-guides)
- [Hướng dẫn phát triển](https://ragflow.io/docs/category/developer-guides)
- [Tài liệu tham khảo API](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

Tài liệu nội bộ trong thư mục `docs/`:
- `docs/administrator/` — Hướng dẫn quản trị
- `docs/basics/` — Khái niệm cơ bản
- `docs/develop/` — Phát triển
- `docs/guides/` — Hướng dẫn sử dụng
- `docs/references/` — Tham khảo

## Cộng đồng

- [Discord](https://discord.gg/NjYzJD3GM3)
- [X (Twitter)](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)
- [Đám mây RAGFlow](https://cloud.ragflow.io)

## Đóng góp

RAGFlow phát triển nhờ sự hợp tác mã nguồn mở. Chúng tôi hoan nghênh mọi đóng góp từ cộng đồng. Nếu bạn muốn tham gia, vui lòng đọc [Hướng dẫn đóng góp](https://ragflow.io/docs/dev/contributing) trước.

### Quy tắc viết code

**Python:**
- Sử dụng `ruff` để lint và format:
  ```bash
  ruff check
  ruff format
  ```

**Frontend (TypeScript/React):**
```bash
cd web
npm run lint
```

**Pre-commit:**
```bash
pre-commit install
pre-commit run --all-files
```

## Giấy phép

Dự án được phát hành theo giấy phép **Apache 2.0**. Xem chi tiết tại [LICENSE](./LICENSE).

---

<div align="center">

**[Về đầu trang](#ragflow)**

Được phát triển bởi [InfiniFlow](https://infiniflow.ai). Star repo này để cập nhật các tính năng mới.

</div>
