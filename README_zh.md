<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="web/src/assets/logo-with-text.svg" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DBEDFA"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.24.0">
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
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a> |
  <a href="https://cloud.ragflow.io">Demo</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 目录</b></summary>

- 💡 [RAGFlow 是什么？](#-RAGFlow-是什么)
- 🎮 [Demo](#-demo)
- 📌 [近期更新](#-近期更新)
- 🌟 [主要功能](#-主要功能)
- 🔎 [系统架构](#-系统架构)
- 🎬 [快速开始](#-快速开始)
- 🔧 [系统配置](#-系统配置)
- 🔨 [以源代码启动服务](#-以源代码启动服务)
- 📚 [技术文档](#-技术文档)
- 📜 [路线图](#-路线图)
- 🏄 [贡献指南](#-贡献指南)
- 🙌 [加入社区](#-加入社区)
- 🤝 [商务合作](#-商务合作)

</details>

## 💡 RAGFlow 是什么？

[RAGFlow](https://ragflow.io/) 是一款领先的开源检索增强生成（[RAG](https://ragflow.io/basics/what-is-rag)）引擎，通过融合前沿的 RAG 技术与 Agent 能力，为大型语言模型提供卓越的上下文层。它提供可适配任意规模企业的端到端 RAG 工作流，凭借融合式[上下文引擎](https://ragflow.io/basics/what-is-agent-context-engine)与预置的 Agent 模板，助力开发者以极致效率与精度将复杂数据转化为高可信、生产级的人工智能系统。

## 🎮 Demo 试用

请登录网址 [https://cloud.ragflow.io](https://cloud.ragflow.io) 试用 demo。

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 近期更新

- 2025-12-26 支持AI代理的“记忆”功能。
- 2025-11-19 支持 Gemini 3 Pro。
- 2025-11-12 支持从 Confluence、S3、Notion、Discord、Google Drive 进行数据同步。
- 2025-10-23 支持 MinerU 和 Docling 作为文档解析方法。
- 2025-10-15 支持可编排的数据管道。
- 2025-08-08 支持 OpenAI 最新的 GPT-5 系列模型。
- 2025-08-01 支持 agentic workflow 和 MCP。
- 2025-05-23 Agent 新增 Python/JS 代码执行器组件。
- 2025-05-05 支持跨语言查询。
- 2025-03-19 PDF 和 DOCX 中的图支持用多模态大模型去解析得到描述。
- 2024-12-18 升级了 DeepDoc 的文档布局分析模型。
- 2024-08-22 支持用 RAG 技术实现从自然语言到 SQL 语句的转换。

## 🎉 关注项目

⭐️ 点击右上角的 Star 关注 RAGFlow，可以获取最新发布的实时通知 !🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 主要功能

### 🍭 **"Quality in, quality out"**

- 基于[深度文档理解](./deepdoc/README.md)，能够从各类复杂格式的非结构化数据中提取真知灼见。
- 真正在无限上下文（token）的场景下快速完成大海捞针测试。

### 🍱 **基于模板的文本切片**

- 不仅仅是智能，更重要的是可控可解释。
- 多种文本模板可供选择

### 🌱 **有理有据、最大程度降低幻觉（hallucination）**

- 文本切片过程可视化，支持手动调整。
- 有理有据：答案提供关键引用的快照并支持追根溯源。

### 🍔 **兼容各类异构数据源**

- 支持丰富的文件类型，包括 Word 文档、PPT、excel 表格、txt 文件、图片、PDF、影印件、复印件、结构化数据、网页等。

### 🛀 **全程无忧、自动化的 RAG 工作流**

- 全面优化的 RAG 工作流可以支持从个人应用乃至超大型企业的各类生态系统。
- 大语言模型 LLM 以及向量模型均支持配置。
- 基于多路召回、融合重排序。
- 提供易用的 API，可以轻松集成到各类企业系统。

## 🔎 系统架构

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 快速开始

### 📝 前提条件

- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): 仅在你打算使用 RAGFlow 的代码执行器（沙箱）功能时才需要安装。

> [!TIP]
> 如果你并没有在本机安装 Docker（Windows、Mac，或者 Linux）, 可以参考文档 [Install Docker Engine](https://docs.docker.com/engine/install/) 自行安装。

### 🚀 启动服务器

1. 确保 `vm.max_map_count` 不小于 262144：

   > 如需确认 `vm.max_map_count` 的大小：
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > 如果 `vm.max_map_count` 的值小于 262144，可以进行重置：
   >
   > ```bash
   > # 这里我们设为 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > 你的改动会在下次系统重启时被重置。如果希望做永久改动，还需要在 **/etc/sysctl.conf** 文件里把 `vm.max_map_count` 的值再相应更新一遍：
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. 克隆仓库：

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. 进入 **docker** 文件夹，利用提前编译好的 Docker 镜像启动服务器：

> [!CAUTION]
> 请注意，目前官方提供的所有 Docker 镜像均基于 x86 架构构建，并不提供基于 ARM64 的 Docker 镜像。
> 如果你的操作系统是 ARM64 架构，请参考[这篇文档](https://ragflow.io/docs/dev/build_docker_image)自行构建 Docker 镜像。

   > 运行以下命令会自动下载 RAGFlow Docker 镜像 `v0.24.0`。请参考下表查看不同 Docker 发行版的描述。如需下载不同于 `v0.24.0` 的 Docker 镜像，请在运行 `docker compose` 启动服务之前先更新 **docker/.env** 文件内的 `RAGFLOW_IMAGE` 变量。

   ```bash
   $ cd ragflow/docker

   # git checkout v0.24.0
   # 可选：使用稳定版本标签（查看发布：https://github.com/infiniflow/ragflow/releases）
   # 这一步确保代码中的 entrypoint.sh 文件与 Docker 镜像的版本保持一致。

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
   ```

   > 注意：在 `v0.22.0` 之前的版本，我们会同时提供包含 embedding 模型的镜像和不含 embedding 模型的 slim 镜像。具体如下：

   | RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?        |
   |-------------------|-----------------|-----------------------|----------------|
   | v0.21.1           | &approx;9       | ✔️                    | Stable release |
   | v0.21.1-slim      | &approx;2       | ❌                     | Stable release |

   > 从 `v0.22.0` 开始，我们只发布 slim 版本，并且不再在镜像标签后附加 **-slim** 后缀。

   > [!TIP]
   > 如果你遇到 Docker 镜像拉不下来的问题，可以在 **docker/.env** 文件内根据变量 `RAGFLOW_IMAGE` 的注释提示选择华为云或者阿里云的相应镜像。
   >
   > - 华为云镜像名：`swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow`
   > - 阿里云镜像名：`registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow`

4. 服务器启动成功后再次确认服务器状态：

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _出现以下界面提示说明服务器启动成功：_

   ```bash
        ____   ___    ______ ______ __
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > 如果您在没有看到上面的提示信息出来之前，就尝试登录 RAGFlow，你的浏览器有可能会提示 `network abnormal` 或 `网络异常`。

5. 在你的浏览器中输入你的服务器对应的 IP 地址并登录 RAGFlow。
   > 上面这个例子中，您只需输入 http://IP_OF_YOUR_MACHINE 即可：未改动过配置则无需输入端口（默认的 HTTP 服务端口 80）。
6. 在 [service_conf.yaml.template](./docker/service_conf.yaml.template) 文件的 `user_default_llm` 栏配置 LLM factory，并在 `API_KEY` 栏填写和你选择的大模型相对应的 API key。

   > 详见 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)。

   _好戏开始，接着奏乐接着舞！_

## 🔧 系统配置

系统配置涉及以下三份文件：

- [.env](./docker/.env)：存放一些基本的系统环境变量，比如 `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` 等。
- [service_conf.yaml.template](./docker/service_conf.yaml.template)：配置各类后台服务。
- [docker-compose.yml](./docker/docker-compose.yml): 系统依赖该文件完成启动。

请务必确保 [.env](./docker/.env) 文件中的变量设置与 [service_conf.yaml.template](./docker/service_conf.yaml.template) 文件中的配置保持一致！

如果不能访问镜像站点 hub.docker.com 或者模型站点 huggingface.co，请按照 [.env](./docker/.env) 注释修改 `RAGFLOW_IMAGE` 和 `HF_ENDPOINT`。

> [./docker/README](./docker/README.md) 解释了 [service_conf.yaml.template](./docker/service_conf.yaml.template) 用到的环境变量设置和服务配置。

如需更新默认的 HTTP 服务端口(80), 可以在 [docker-compose.yml](./docker/docker-compose.yml) 文件中将配置 `80:80` 改为 `<YOUR_SERVING_PORT>:80`。

> 所有系统配置都需要通过系统重启生效：
>
> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

### 把文档引擎从 Elasticsearch 切换成为 Infinity

RAGFlow 默认使用 Elasticsearch 存储文本和向量数据. 如果要切换为 [Infinity](https://github.com/infiniflow/infinity/), 可以按照下面步骤进行:

1. 停止所有容器运行:

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```
   Note: `-v` 将会删除 docker 容器的 volumes，已有的数据会被清空。

2. 设置 **docker/.env** 目录中的 `DOC_ENGINE` 为 `infinity`.

3. 启动容器:

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```

> [!WARNING]
> Infinity 目前官方并未正式支持在 Linux/arm64 架构下的机器上运行.

## 🔧 源码编译 Docker 镜像

本 Docker 镜像大小约 2 GB 左右并且依赖外部的大模型和 embedding 服务。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

如果您处在代理环境下，可以传递代理参数：

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 以源代码启动服务

1. 安装 `uv` 和 `pre-commit`。如已经安装，可跳过本步骤：

   ```bash
   pipx install uv pre-commit
   export UV_INDEX=https://mirrors.aliyun.com/pypi/simple
   ```

2. 下载源代码并安装 Python 依赖：

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.12 # install RAGFlow dependent python modules
   uv run download_deps.py
   pre-commit install
   ```

3. 通过 Docker Compose 启动依赖的服务（MinIO, Elasticsearch, Redis, and MySQL）：

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   在 `/etc/hosts` 中添加以下代码，目的是将 **conf/service_conf.yaml** 文件中的所有 host 地址都解析为 `127.0.0.1`：

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. 如果无法访问 HuggingFace，可以把环境变量 `HF_ENDPOINT` 设成相应的镜像站点：

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

6. 启动后端服务：

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

7. 安装前端依赖：

   ```bash
   cd web
   npm install
   ```

8. 启动前端服务：

   ```bash
   npm run dev
   ```

   _以下界面说明系统已经成功启动：_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

9. 开发完成后停止 RAGFlow 前端和后端服务：

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```


## 📚 技术文档

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/category/user-guides)
- [Developer guides](https://ragflow.io/docs/category/developer-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 路线图

详见 [RAGFlow Roadmap 2026](https://github.com/infiniflow/ragflow/issues/12241) 。

## 🏄 开源社区

- [Discord](https://discord.gg/zd4qPW6t)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 贡献指南

RAGFlow 只有通过开源协作才能蓬勃发展。秉持这一精神,我们欢迎来自社区的各种贡献。如果您有意参与其中,请查阅我们的 [贡献者指南](https://ragflow.io/docs/dev/contributing) 。

## 🤝 商务合作

- [预约咨询](https://aao615odquw.feishu.cn/share/base/form/shrcnjw7QleretCLqh1nuPo1xxh)

## 👥 加入社区

扫二维码添加 RAGFlow 小助手，进 RAGFlow 交流群。

<p align="center">
  <img src="https://github.com/infiniflow/ragflow/assets/7248/bccf284f-46f2-4445-9809-8f1030fb7585" width=50% height=50%>
</p>
