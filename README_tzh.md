<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a> |
  <a href="./README_id.md">Bahasa Indonesia</a> |
  <a href="/README_pt_br.md">Português (Brasil)</a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.16.0-brightgreen" alt="docker pull infiniflow/ragflow:v0.16.0">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/4214">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/4XxujFgUN7">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>

## 💡 RAGFlow 是什麼？

[RAGFlow](https://ragflow.io/) 是一款基於深度文件理解所建構的開源 RAG（Retrieval-Augmented Generation）引擎。 RAGFlow 可以為各種規模的企業及個人提供一套精簡的 RAG 工作流程，結合大語言模型（LLM）針對用戶各類不同的複雜格式數據提供可靠的問答以及有理有據的引用。

## 🎮 Demo 試用

請登入網址 [https://demo.ragflow.io](https://demo.ragflow.io) 試用 demo。

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/user-attachments/assets/504bbbf1-c9f7-4d83-8cc5-e9cb63c26db6" width="1200"/>
</div>

## 🔥 近期更新

- 2025-02-05 更新「SILICONFLOW」的型號清單並新增 Deepseek-R1/DeepSeek-V3 的支援。
- 2025-01-26 最佳化知識圖譜的擷取與應用，提供了多種配置選擇。
- 2024-12-18 升級了 Deepdoc 的文檔佈局分析模型。
- 2024-12-04 支援知識庫的 Pagerank 分數。
- 2024-11-22 完善了 Agent 中的變數定義和使用。
- 2024-11-01 對解析後的 chunk 加入關鍵字抽取和相關問題產生以提高回想的準確度。
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

- 支援豐富的文件類型，包括 Word 文件、PPT、excel 表格、txt 檔案、圖片、PDF、影印件、影印件、結構化資料、網頁等。

### 🛀 **全程無憂、自動化的 RAG 工作流程**

- 全面優化的 RAG 工作流程可以支援從個人應用乃至超大型企業的各類生態系統。
- 大語言模型 LLM 以及向量模型皆支援配置。
- 基於多路召回、融合重排序。
- 提供易用的 API，可輕鬆整合到各類企業系統。

## 🔎 系統架構

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 快速開始

### 📝 前提條件

- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
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

2. 克隆倉庫：

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. 進入 **docker** 資料夾，利用事先編譯好的 Docker 映像啟動伺服器：

   > 執行以下指令會自動下載 RAGFlow slim Docker 映像 `v0.16.0-slim`。請參考下表查看不同 Docker 發行版的說明。如需下載不同於 `v0.16.0-slim` 的 Docker 映像，請在執行 `docker compose` 啟動服務之前先更新 **docker/.env** 檔案內的 `RAGFLOW_IMAGE` 變數。例如，你可以透過設定 `RAGFLOW_IMAGE=infiniflow/ragflow:v0.16.0` 來下載 RAGFlow 鏡像的 `v0.16.0` 完整發行版。

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose.yml up -d
   ```

   | RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?                  |
   | ----------------- | --------------- | --------------------- | ------------------------ |
   | v0.16.0           | &approx;9       | :heavy_check_mark:    | Stable release           |
   | v0.16.0-slim      | &approx;2       | ❌                    | Stable release           |
   | nightly           | &approx;9       | :heavy_check_mark:    | _Unstable_ nightly build |
   | nightly-slim      | &approx;2       | ❌                    | _Unstable_ nightly build |

   > [!TIP]
   > 如果你遇到 Docker 映像檔拉不下來的問題，可以在 **docker/.env** 檔案內根據變數 `RAGFLOW_IMAGE` 的註解提示選擇華為雲或阿里雲的對應映像。
   >
   > - 華為雲鏡像名：`swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow`
   > - 阿里雲鏡像名：`registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow`

4. 伺服器啟動成功後再次確認伺服器狀態：

   ```bash
   $ docker logs -f ragflow-server
   ```

   _出現以下介面提示說明伺服器啟動成功：_

   ```bash
        ____   ___    ______ ______ __
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
    * Running on http://127.0.0.1:9380
    * Running on http://x.x.x.x:9380
    INFO:werkzeug:Press CTRL+C to quit
   ```

   > 如果您跳過這一步驟系統確認步驟就登入 RAGFlow，你的瀏覽器有可能會提示 `network anormal` 或 `網路異常`，因為 RAGFlow 可能並未完全啟動成功。

5. 在你的瀏覽器中輸入你的伺服器對應的 IP 位址並登入 RAGFlow。
   > 上面這個範例中，您只需輸入 http://IP_OF_YOUR_MACHINE 即可：未改動過設定則無需輸入連接埠（預設的 HTTP 服務連接埠 80）。
6. 在 [service_conf.yaml.template](./docker/service_conf.yaml.template) 檔案的 `user_default_llm` 欄位設定 LLM factory，並在 `API_KEY` 欄填入和你選擇的大模型相對應的 API key。

   > 詳見 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)。

   _好戲開始，接著奏樂接著舞！ _

## 🔧 系統配置

系統配置涉及以下三份文件：

- [.env](./docker/.env)：存放一些基本的系統環境變量，例如 `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` 等。
- [service_conf.yaml.template](./docker/service_conf.yaml.template)：設定各類別後台服務。
- [docker-compose.yml](./docker/docker-compose.yml): 系統依賴該檔案完成啟動。

請務必確保 [.env](./docker/.env) 檔案中的變數設定與 [service_conf.yaml.template](./docker/service_conf.yaml.template) 檔案中的設定保持一致！

如果無法存取映像網站 hub.docker.com 或模型網站 huggingface.co，請依照 [.env](./docker/.env) 註解修改 `RAGFLOW_IMAGE` 和 `HF_ENDPOINT`。

> [./docker/README](./docker/README.md) 解釋了 [service_conf.yaml.template](./docker/service_conf.yaml.template) 用到的環境變數設定和服務配置。

如需更新預設的 HTTP 服務連接埠(80), 可以在[docker-compose.yml](./docker/docker-compose.yml) 檔案中將配置`80:80` 改為`<YOUR_SERVING_PORT>:80` 。

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

## 🔧 原始碼編譯 Docker 映像（不含 embedding 模型）

本 Docker 映像大小約 2 GB 左右並且依賴外部的大模型和 embedding 服務。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --build-arg LIGHTEN=1 --build-arg NEED_MIRROR=1 -f Dockerfile -t infiniflow/ragflow:nightly-slim .
```

## 🔧 原始碼編譯 Docker 映像（包含 embedding 模型）

本 Docker 大小約 9 GB 左右。由於已包含 embedding 模型，所以只需依賴外部的大模型服務即可。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --build-arg NEED_MIRROR=1 -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 以原始碼啟動服務

1. 安裝 uv。如已安裝，可跳過此步驟：

   ```bash
   pipx install uv
   export UV_INDEX=https://pypi.tuna.tsinghua.edu.cn/simple
   ```

2. 下載原始碼並安裝 Python 依賴：

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.10 --all-extras # install RAGFlow dependent python modules
   ```

3. 透過 Docker Compose 啟動依賴的服務（MinIO, Elasticsearch, Redis, and MySQL）：

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   在 `/etc/hosts` 中加入以下程式碼，將 **conf/service_conf.yaml** 檔案中的所有 host 位址都解析為 `127.0.0.1`：

   ```
   127.0.0.1 es01 infinity mysql minio redis
   ```

4. 如果無法存取 HuggingFace，可以把環境變數 `HF_ENDPOINT` 設為對應的鏡像網站：

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5.啟動後端服務：
『`bash
source .venv/bin/activate
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh

```

6. 安裝前端依賴：
『`bash
cd web
npm install
```

7. 啟動前端服務：
   『`bash
   npm run dev

   ```

   以下界面說明系統已成功啟動：_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
   ```

## 📚 技術文檔

- [Quickstart](https://ragflow.io/docs/dev/)
- [User guide](https://ragflow.io/docs/dev/category/guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 路線圖

詳見 [RAGFlow Roadmap 2025](https://github.com/infiniflow/ragflow/issues/4214) 。

## 🏄 開源社群

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 貢獻指南

RAGFlow 只有透過開源協作才能蓬勃發展。秉持這項精神,我們歡迎來自社區的各種貢獻。如果您有意參與其中,請查閱我們的 [貢獻者指南](./CONTRIBUTING.md) 。

## 🤝 商務合作

- [預約諮詢](https://aao615odquw.feishu.cn/share/base/form/shrcnjw7QleretCLqh1nuPo1xxh)

## 👥 加入社區

掃二維碼加入 RAGFlow 小助手，進 RAGFlow 交流群。

<p align="center">
  <img src="https://github.com/infiniflow/ragflow/assets/7248/bccf284f-46f2-4445-9809-8f1030fb7585" width=50% height=50%>
</p>
