<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="web/src/assets/logo-with-text.svg" width="350" alt="ragflow logo">
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
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.25.4">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Cloud</a> |
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 目錄</b></summary>

- 💡 [RAGFlow 是什麼？](#-RAGFlow-是什麼)
- 🎮 [快速開始](#-快速開始)
- 📌 [近期更新](#-近期更新)
- 🌟 [主要功能](#-主要功能)
- 🔎 [系統架構](#-系統架構)
- 🎬 [自行架設](#-自行架設)
- 🔧 [系統配置](#-系統配置)
- 🔨 [以原始碼啟動服務](#-以原始碼啟動服務)
- 📚 [技術文檔](#-技術文檔)
- 📜 [路線圖](#-路線圖)
- 🏄 [貢獻指南](#-貢獻指南)
- 🙌 [加入社區](#-加入社區)
- 🤝 [商務合作](#-商務合作)

</details>

## 💡 RAGFlow 是什麼？

[RAGFlow](https://ragflow.io/) 是一款領先的開源 [RAG](https://ragflow.io/basics/what-is-rag)（Retrieval-Augmented Generation）引擎，通過融合前沿的 RAG 技術與 Agent 能力，為大型語言模型提供卓越的上下文層。它提供可適配任意規模企業的端到端 RAG 工作流，憑藉融合式[上下文引擎](https://ragflow.io/basics/what-is-agent-context-engine)與預置的 Agent 模板，助力開發者以極致效率與精度將複雜數據轉化為高可信、生產級的人工智能系統。

## 🎮 快速開始

請登入網址 [https://cloud.ragflow.io](https://cloud.ragflow.io) 試用雲服務。

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 近期更新

- 2026-04-24 支援 DeepSeek v4 版本。
- 2026-03-24 發布 [RAGFlow 官方 Skill](https://clawhub.ai/yingfeng/ragflow-skill) — 提供官方 Skill 以透過 OpenClaw 訪問 RAGFlow 數據集。
- 2025-12-26 支援AI代理的「記憶」功能。
- 2025-11-19 支援 Gemini 3 Pro。
- 2025-11-12 支援從 Confluence、S3、Notion、Discord、Google Drive 進行資料同步。
- 2025-10-23 支援 MinerU 和 Docling 作為文件解析方法。
- 2025-10-15 支援可編排的資料管道。
- 2025-08-08 支援 OpenAI 最新的 GPT-5 系列模型。
- 2025-08-01 支援 agentic workflow 和 MCP。
- 2025-05-23 為 Agent 新增 Python/JS 程式碼執行器元件。
- 2025-05-05 支援跨語言查詢。
- 2025-03-19 PDF和DOCX中的圖支持用多模態大模型去解析得到描述。
- 2024-12-18 升級了 DeepDoc 的文檔佈局分析模型。
- 2024-08-22 支援用 RAG 技術實現從自然語言到 SQL 語句的轉換。

## 🎉 關注項目

⭐️ 點擊右上角的 Star 追蹤 RAGFlow，可以取得最新發布的即時通知 !🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 主要功能

### 🍭 **"Quality in, quality out"**

- 基於[深度文件理解](./deepdoc/README.md)，能夠從各類複雜格式的非結構化資料中提取真知灼見。
- 真正在無限上下文（token）的場景下快速完成大海撈針測試。

### 🍱 **基於模板的文字切片**

- 不只是智能，更重要的是可控可解釋。
- 多種文字範本可供選擇

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
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 自行架設

### 📝 前提條件

- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): 僅在您打算使用 RAGFlow 的代碼執行器（沙箱）功能時才需要安裝。

> [!TIP]
> 如果你並沒有在本機安裝 Docker（Windows、Mac，或 Linux）, 可以參考文件 [Install Docker Engine](https://docs.docker.com/engine/install/) 自行安裝。

### 🚀 啟動伺服器

1. 確保 `vm.max_map_count` 不小於 262144：

   > 如需確認 `vm.max_map_count` 的大小：
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > 如果 `vm.max_map_count` 的值小於 262144，可以進行重設：
   >
   > ```bash
   > # 這裡我們設為 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > 你的改動會在下次系統重新啟動時被重置。如果希望做永久改動，還需要在 **/etc/sysctl.conf** 檔案裡把 `vm.max_map_count` 的值再相應更新一遍：
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. 克隆倉庫：

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```
3. 進入 **docker** 資料夾，利用事先編譯好的 Docker 映像啟動伺服器：

> [!CAUTION]
> 所有 Docker 映像檔都是為 x86 平台建置的。目前，我們不提供 ARM64 平台的 Docker 映像檔。
> 如果您使用的是 ARM64 平台，請使用 [這份指南](https://ragflow.io/docs/dev/build_docker_image) 來建置適合您系統的 Docker 映像檔。

> 執行以下指令會自動下載 RAGFlow Docker 映像 `v0.25.4`。請參考下表查看不同 Docker 發行版的說明。如需下載不同於 `v0.25.4` 的 Docker 映像，請在執行 `docker compose` 啟動服務之前先更新 **docker/.env** 檔案內的 `RAGFLOW_IMAGE` 變數。

```bash
   $ cd ragflow/docker

   # git checkout v0.25.4
   # 可選：使用穩定版標籤（查看發佈：https://github.com/infiniflow/ragflow/releases）
   # 此步驟確保程式碼中的 entrypoint.sh 檔案與 Docker 映像版本一致。

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> 注意：在 `v0.22.0` 之前的版本，我們會同時提供包含 embedding 模型的映像和不含 embedding 模型的 slim 映像。具體如下：

| RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?        |
|-------------------|-----------------|-----------------------|----------------|
| v0.21.1           | &approx;9       | ✔️                    | Stable release |
| v0.21.1-slim      | &approx;2       | ❌                     | Stable release |

> 從 `v0.22.0` 開始，我們只發佈 slim 版本，並且不再在映像標籤後附加 **-slim** 後綴。

> [!TIP]
> 如果你遇到 Docker 映像檔拉不下來的問題，可以在 **docker/.env** 檔案內根據變數 `RAGFLOW_IMAGE` 的註解提示選擇華為雲或阿里雲的對應映像。
>
> - 華為雲鏡像名：`swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow`
> - 阿里雲鏡像名：`registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow`

4. 伺服器啟動成功後再次確認伺服器狀態：

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _出現以下介面提示說明伺服器啟動成功：_

   ```bash
        ____   ___    ______ ______ __
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > 如果您跳過這一步驟系統確認步驟就登入 RAGFlow，你的瀏覽器有可能會提示 `network abnormal` 或 `網路異常`，因為 RAGFlow 可能並未完全啟動成功。
   >
5. 在你的瀏覽器中輸入你的伺服器對應的 IP 位址並登入 RAGFlow。

   > 上面這個範例中，您只需輸入 http://IP_OF_YOUR_MACHINE 即可：未改動過設定則無需輸入連接埠（預設的 HTTP 服務連接埠 80）。
   >
6. 在 [service_conf.yaml.template](./docker/service_conf.yaml.template) 檔案的 `user_default_llm` 欄位設定 LLM factory，並在 `API_KEY` 欄填入和你選擇的大模型相對應的 API key。

   > 詳見 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)。
   >

   _好戲開始，接著奏樂接著舞！ _

## 🔧 系統配置

系統配置涉及以下三份文件：

- [.env](./docker/.env)：存放一些系統環境變量，例如 `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` 等。
- [service_conf.yaml.template](./docker/service_conf.yaml.template)：設定各類別後台服務。
- [docker-compose.yml](./docker/docker-compose.yml): 系統依賴該檔案完成啟動。

請務必確保 [.env](./docker/.env) 檔案中的變數設定與 [service_conf.yaml.template](./docker/service_conf.yaml.template) 檔案中的設定保持一致！

如果無法存取映像網站 hub.docker.com 或模型網站 huggingface.co，請依照 [.env](./docker/.env) 註解修改 `RAGFLOW_IMAGE` 和 `HF_ENDPOINT`。

> [./docker/README](./docker/README.md) 解釋了 [service_conf.yaml.template](./docker/service_conf.yaml.template) 用到的環境變數設定和服務配置。

如需更新預設的 HTTP 服務連接埠(80), 可以在[docker-compose.yml](./docker/docker-compose.yml) 檔案中將配置 `80:80` 改為 `<YOUR_SERVING_PORT>:80` 。

> 所有系統配置都需要透過系統重新啟動生效：
>
> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

###把文檔引擎從 Elasticsearch 切換成為 Infinity

RAGFlow 預設使用 Elasticsearch 儲存文字和向量資料. 如果要切換為 [Infinity](https://github.com/infiniflow/infinity/), 可以按照下面步驟進行:

1. 停止所有容器運作:

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```

   Note: `-v` 將會刪除 docker 容器的 volumes，已有的資料會被清空。
2. 設定 **docker/.env** 目錄中的 `DOC_ENGINE` 為 `infinity`.
3. 啟動容器:

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```

> [!WARNING]
> Infinity 目前官方並未正式支援在 Linux/arm64 架構下的機器上運行.

## 🔧 原始碼編譯 Docker 映像

本 Docker 映像大小約 2 GB 左右並且依賴外部的大模型和 embedding 服務。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

若您位於代理環境，可傳遞代理參數：

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 以原始碼啟動服務

1. 安裝 `uv` 和 `pre-commit`。如已安裝，可跳過此步驟：

   ```bash
   pipx install uv pre-commit
   export UV_INDEX=https://mirrors.aliyun.com/pypi/simple
   ```
2. 下載原始碼並安裝 Python 依賴：

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.12 # install RAGFlow dependent python modules
   uv run python3 download_deps.py
   pre-commit install
   ```
3. 透過 Docker Compose 啟動依賴的服務（MinIO, Elasticsearch, Redis, and MySQL）：

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   在 `/etc/hosts` 中加入以下程式碼，將 **conf/service_conf.yaml** 檔案中的所有 host 位址都解析為 `127.0.0.1`：

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. 如果無法存取 HuggingFace，可以把環境變數 `HF_ENDPOINT` 設為對應的鏡像網站：

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```
5. 如果你的操作系统没有 jemalloc，请按照如下方式安装：

   ```bash
   # ubuntu
   sudo apt-get install libjemalloc-dev
   # centos
   sudo yum install jemalloc
   # mac
   sudo brew install jemalloc
   ```
6. 啟動後端服務：

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```
7. 安裝前端依賴：

   ```bash
   cd web
   npm install
   ```
8. 啟動前端服務：

   ```bash
   npm run dev
   ```

   以下界面說明系統已成功啟動：_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

   ```

   ```
9. 開發完成後停止 RAGFlow 前端和後端服務：

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

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

## 🤝 商務合作

- [預約諮詢](https://aao615odquw.feishu.cn/share/base/form/shrcnjw7QleretCLqh1nuPo1xxh)

## 👥 加入社區

掃二維碼加入 RAGFlow 小助手，進 RAGFlow 交流群。

<p align="center">
  <img src="https://github.com/infiniflow/ragflow/assets/7248/bccf284f-46f2-4445-9809-8f1030fb7585" width=50% height=50%>
</p>
