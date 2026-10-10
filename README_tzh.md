<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow/main/web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DBEDFA"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
  <a href="./README_ru.md"><img alt="Русская версия README" src="https://img.shields.io/badge/Русский-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="RAGFlow Docker image downloads">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Cloud</a> |
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow in the GitHub Octoverse" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 目錄</b></summary>

- 💡 [RAGFlow 是什麼？](#-ragflow-是什麼)
- 🎮 [快速開始](#-快速開始)
- 🔥 [近期更新](#-近期更新)
- 🌟 [主要功能](#-主要功能)
- 🔎 [系統架構](#-系統架構)
- 🏠 [本機部署](#-本機部署)
- 📚 [技術文檔](#-技術文檔)
- 📜 [路線圖](#-路線圖)
- 🏄 [開源社群](#-開源社群)
- 🙌 [貢獻指南](#-貢獻指南)

</details>

## 💡 RAGFlow 是什麼？

[RAGFlow](https://ragflow.io/) 是一款領先的開源 [RAG](https://ragflow.io/basics/what-is-rag)（Retrieval-Augmented Generation）引擎，通過融合前沿的 RAG 技術與 Agent 能力，為大型語言模型提供卓越的上下文層。它提供可適配任意規模企業的端到端 RAG 工作流，憑藉融合式[上下文引擎](https://ragflow.io/basics/what-is-agent-context-engine)與預置的 Agent 模板，助力開發者以極致效率與精度將複雜數據轉化為高可信、生產級的人工智能系統。

## 🎮 快速開始

請登入網址 [https://cloud.ragflow.io](https://cloud.ragflow.io) 試用雲服務。

如需本機部署，請參閱[本機部署](#-自行架設)。

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 近期更新

- 2026-09-29 發布 RAGFlow 1.0.0-rc1。

- 2026-09-10 支援透過 Sitemap 接入網站內容。
- 2026-08-19 推出知識編譯，支援在文件級和知識庫級產生 Wiki、Graph、Tree、PageIndex、Mind Map、Timeline 及 Skills。
- 2026-08-19 推出 Agentic RAG，支援 Low、Medium、High、Ultra 四種思考模式。
- 2026-07-02 支援 Google BigQuery 資料來源接入與增量同步。

更多更新請參閱[完整發布記錄](https://ragflow.io/docs/dev/release_notes)。


## 🎉 關注項目

⭐️ 點擊右上角的 Star 追蹤 RAGFlow，可以取得最新發布的即時通知 !🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 主要功能

### 🍭 **"Quality in, quality out"**

- 基於深度文件理解，能夠從各類複雜格式的非結構化資料中提取真知灼見。
- 真正在無限上下文（token）的場景下快速完成大海撈針測試。

### 🍱 **基於模板的文字切片**

- 不只是智能，更重要的是可控可解釋。
- 多種文字範本可供選擇

### 🧩 **知識編譯（Knowledge Compilation）**

- 支援文件級和知識庫級編譯，將原始內容整理為結構化知識產物。
- 透過編譯範本產生 Wiki、Graph、Tree、PageIndex、Mind Map、Timeline 及 Skills，滿足不同的知識組織與重用需求。
- 支援設定編譯模型與處理規則，並檢視、更新和重新產生知識產物。

### 🧠 **Agentic Retrieval**

- 針對複雜問題進行多步檢索：模型會分析問題，並在需要時執行問題拆解、知識檢索和證據核驗。
- 透過多輪檢索與推理取得更完整的上下文，協助產生有依據的回答。
- 支援 Low、Medium、High、Ultra 思考模式，可依問題複雜度控制檢索和推理深度。

### 🌱 **有理有據、最大程度降低幻覺（hallucination）**

- 文字切片過程視覺化，支援手動調整。
- 有理有據：答案提供關鍵引用的快照並支持追根溯源。

### 🍔 **相容各類異質資料來源**

- 支援豐富的文件類型，包括 Word 文件、PPT、excel 表格、txt 檔案、圖片、PDF、影印件、複印件、結構化資料、網頁等。

### 🛀 **全程無憂、自動化的 RAG 工作流程**

- 全面優化的 RAG 工作流程可以支援從個人應用乃至超大型企業的各類生態系統。
- 大語言模型 LLM 以及向量模型皆支援配置。
- 基於多路召回、融合重排序。
- 提供易用的 API，可輕鬆整合到各類企業系統。

## 🔎 系統架構

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow system architecture" width="1000" />
</div>

## 🏠 本機部署

Docker 適合快速體驗、整合測試和正式環境；原始碼啟動適合開發與除錯。Docker 不需要 Go。原始碼啟動需要 `go.mod` 指定的 Go，前端開發還需要 Node.js 和 npm。

### 🐳 Docker 部署

#### 📝 前提條件

- 建議起步設定：4 核心 CPU、16 GB 記憶體和 50 GB 可用磁碟空間。
- Docker ≥ 24.0.0, Docker Compose ≥ v2.26.1.

#### 🚀 啟動伺服器

1. Elasticsearch: `vm.max_map_count` ≥ 262144.

2. `git clone https://github.com/infiniflow/ragflow.git`

3. Docker:

   ```bash
   cd ragflow/docker
   git checkout v1.0.0-rc1
   docker compose -f docker-compose.yml up -d
   ```

4. 檢查服務狀態和 API 是否就緒：

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   docker logs --tail 50 ragflow-cpu
   ```

5. 在瀏覽器開啟 `http://IP_OF_YOUR_MACHINE` 並登入。

6. 在模型供應商頁面新增 LLM、Embedding 和 Reranker。詳情請參閱 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)。

詳情請參閱[快速入門指南](./docs/quickstart.mdx)。

#### ⚙️ Docker 設定與調整

使用 `docker/.env` 和 `docker/docker-compose.yml`。請參閱 [Docker 設定](./docker/README.md)及[映像建置與平台支援](./docs/develop/build_docker_image.mdx)。macOS 目前不受支援。

### 🔨 以原始碼啟動

#### 📝 前提條件

1. 複製倉庫並安裝 `go.mod` 指定的 Go、Clang 20、LLD 20、CMake ≥ 4.0、PCRE2，以及 CGO 所需的原生程式庫。

2. 準備依賴項目：

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   ./build.sh --all
   ```

3. 編譯服務:

   ```bash
   ./build.sh --all
   ```

4. 啟動本機依賴服務:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks
   ```

5. 遷移資料庫:

   ```bash
   ./bin/ragflow_server --migrate
   ```

6. 使用四個終端機啟動四項服務:

   終端機 1：Admin (`9381`)。

   ```bash
   ./bin/ragflow_server --admin
   ```

   終端機 2：Ingestor。

   ```bash
   ./bin/ragflow_server --ingestor
   ```

   終端機 3：Syncer。

   ```bash
   ./bin/ragflow_server --syncer
   ```

   終端機 4：API (`9380`)。

   ```bash
   ./bin/ragflow_server --api
   ```

7. 僅在前端開發時安裝 Node.js 和 npm，並驗證 API:

   ```bash
   cd web
   npm install
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

詳情請參閱[從原始碼啟動服務](./docs/develop/launch_ragflow_from_source.md)。

> **操作說明：** 若要永久設定 Elasticsearch，請在 `/etc/sysctl.conf` 加入 `vm.max_map_count=262144`。預設 MySQL 設定下，Docker 入口會先執行資料庫遷移，再啟動 Admin、Syncer、Ingestor 和 API。開源版 1.0 的 DeepDoc 使用 CPU 執行版面分析、OCR 和表格辨識。修改 Docker 設定後請重新啟動服務，資料保留或清理請依照 [Docker 指南](./docker/README.md)。`download_deps.py` 會準備原生程式庫和模型資源，並需要 `requests` 與 `huggingface-hub`。`--migrate` 執行遷移後退出；其餘參數啟動對應服務。HTTP 200 表示 API 已就緒。

## 📚 技術文檔

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/category/user-guides)
- [Developer guides](https://ragflow.io/docs/category/developer-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 路線圖

詳見 [RAGFlow Roadmap 2026](https://github.com/infiniflow/ragflow/issues/12241) 。

## 🏄 開源社群

- [Discord](https://discord.gg/NjYzJD3GM3)
- [X](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 貢獻指南

RAGFlow 只有透過開源協作才能蓬勃發展。秉持這項精神,我們歡迎來自社區的各種貢獻。如果您有意參與其中,請查閱我們的 [貢獻者指南](https://ragflow.io/docs/dev/contributing) 。
