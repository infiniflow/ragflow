<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.13.0-brightgreen" alt="docker pull infiniflow/ragflow:v0.13.0">
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
  <a href="https://github.com/infiniflow/ragflow/issues/162">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/4XxujFgUN7">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>

## 💡 RAGFlow 是什么？

[RAGFlow](https://ragflow.io/) 是一款基于深度文档理解构建的开源 RAG（Retrieval-Augmented Generation）引擎。RAGFlow 可以为各种规模的企业及个人提供一套精简的 RAG 工作流程，结合大语言模型（LLM）针对用户各类不同的复杂格式数据提供可靠的问答以及有理有据的引用。

## 🎮 Demo 试用

请登录网址 [https://demo.ragflow.io](https://demo.ragflow.io) 试用 demo。
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/user-attachments/assets/504bbbf1-c9f7-4d83-8cc5-e9cb63c26db6" width="1200"/>
</div>


## 🔥 近期更新

- 2024-11-01 对解析后的chunk加入关键词抽取和相关问题生成以提高召回的准确度。
- 2024-09-29 优化多轮对话.
- 2024-09-13 增加知识库问答搜索模式。
- 2024-09-09 在 Agent 中加入医疗问诊模板。
- 2024-08-22 支持用 RAG 技术实现从自然语言到 SQL 语句的转换。
- 2024-08-02 支持 GraphRAG 启发于 [graphrag](https://github.com/microsoft/graphrag) 和思维导图。

## 🎉 关注项目
⭐️点击右上角的 Star 关注RAGFlow，可以获取最新发布的实时通知 !🌟
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
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 快速开始

### 📝 前提条件

- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
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

   > 运行以下命令会自动下载 dev 版的 RAGFlow slim Docker 镜像（`dev-slim`），该镜像并不包含 embedding 模型以及一些 Python 库，因此镜像大小约 1GB。

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose.yml up -d
   ```

   > - 如果你想下载并运行特定版本的 RAGFlow slim Docker 镜像，请在 **docker/.env** 文件中找到 `RAGFLOW_IMAGE` 变量，将其改为对应版本。例如 `RAGFLOW_IMAGE=infiniflow/ragflow:v0.13.0-slim`，然后再运行上述命令。
   > - 如果您想安装内置 embedding 模型和 Python 库的 dev 版本的 Docker 镜像，需要将 **docker/.env** 文件中的 `RAGFLOW_IMAGE` 变量修改为： `RAGFLOW_IMAGE=infiniflow/ragflow:dev`。
   > - 如果您想安装内置 embedding 模型和 Python 库的指定版本的 RAGFlow Docker 镜像，需要将 **docker/.env** 文件中的 `RAGFLOW_IMAGE` 变量修改为： `RAGFLOW_IMAGE=infiniflow/ragflow:v0.13.0`。修改后，再运行上面的命令。
   > **注意：** 安装内置 embedding 模型和 Python 库的指定版本的 RAGFlow Docker 镜像大小约 9 GB，可能需要更长时间下载，请耐心等待。
   
4. 服务器启动成功后再次确认服务器状态：

   ```bash
   $ docker logs -f ragflow-server
   ```

   _出现以下界面提示说明服务器启动成功：_

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
   > 如果您跳过这一步系统确认步骤就登录 RAGFlow，你的浏览器有可能会提示 `network abnormal` 或 `网络异常`，因为 RAGFlow 可能并未完全启动成功。

5. 在你的浏览器中输入你的服务器对应的 IP 地址并登录 RAGFlow。
   > 上面这个例子中，您只需输入 http://IP_OF_YOUR_MACHINE 即可：未改动过配置则无需输入端口（默认的 HTTP 服务端口 80）。
6. 在 [service_conf.yaml](./docker/service_conf.yaml) 文件的 `user_default_llm` 栏配置 LLM factory，并在 `API_KEY` 栏填写和你选择的大模型相对应的 API key。

   > 详见 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)。

   _好戏开始，接着奏乐接着舞！_

## 🔧 系统配置

系统配置涉及以下三份文件：

- [.env](./docker/.env)：存放一些基本的系统环境变量，比如 `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` 等。
- [service_conf.yaml](./docker/service_conf.yaml)：配置各类后台服务。
- [docker-compose.yml](./docker/docker-compose.yml): 系统依赖该文件完成启动。

请务必确保 [.env](./docker/.env) 文件中的变量设置与 [service_conf.yaml](./docker/service_conf.yaml) 文件中的配置保持一致！

如果不能访问镜像站点hub.docker.com或者模型站点huggingface.co，请按照[.env](./docker/.env)注释修改`RAGFLOW_IMAGE`和`HF_ENDPOINT`。

> [./docker/README](./docker/README.md) 文件提供了环境变量设置和服务配置的详细信息。请**一定要**确保 [./docker/README](./docker/README.md) 文件当中列出来的环境变量的值与 [service_conf.yaml](./docker/service_conf.yaml) 文件当中的系统配置保持一致。

如需更新默认的 HTTP 服务端口(80), 可以在 [docker-compose.yml](./docker/docker-compose.yml) 文件中将配置 `80:80` 改为 `<YOUR_SERVING_PORT>:80`。

> 所有系统配置都需要通过系统重启生效：
>
> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

## 🔧 源码编译 Docker 镜像（不含 embedding 模型）

本 Docker 镜像大小约 1 GB 左右并且依赖外部的大模型和 embedding 服务。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```

## 🔧 源码编译 Docker 镜像（包含 embedding 模型）

本 Docker 大小约 9 GB 左右。由于已包含 embedding 模型，所以只需依赖外部的大模型服务即可。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```

## 🔨 以源代码启动服务

1. 安装 Poetry。如已经安装，可跳过本步骤：  
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. 下载源代码并安装 Python 依赖：  
   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ~/.local/bin/poetry install --sync --no-root # install RAGFlow dependent python modules
   ```

3. 通过 Docker Compose 启动依赖的服务（MinIO, Elasticsearch, Redis, and MySQL）：  
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   在 `/etc/hosts` 中添加以下代码，将 **docker/service_conf.yaml** 文件中的所有 host 地址都解析为 `127.0.0.1`：  
   ```
   127.0.0.1       es01 mysql minio redis
   ```  
   在文件 **docker/service_conf.yaml** 中，对照 **docker/.env** 的配置将 mysql 端口更新为 `5455`，es 端口更新为 `1200`。

4. 如果无法访问 HuggingFace，可以把环境变量 `HF_ENDPOINT` 设成相应的镜像站点：  
 
   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. 启动后端服务：  
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

6. 安装前端依赖：  
   ```bash
   cd web
   npm install --force
   ```  
7. 配置前端，将 **.umirc.ts** 的 `proxy.target` 更新为 `http://127.0.0.1:9380`：  
8. 启动前端服务：  
   ```bash
   npm run dev 
   ```  

   _以下界面说明系统已经成功启动：_  

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

## 📚 技术文档

- [Quickstart](https://ragflow.io/docs/dev/)
- [User guide](https://ragflow.io/docs/dev/category/guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 路线图

详见 [RAGFlow Roadmap 2024](https://github.com/infiniflow/ragflow/issues/162) 。

## 🏄 开源社区

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 贡献指南

RAGFlow 只有通过开源协作才能蓬勃发展。秉持这一精神,我们欢迎来自社区的各种贡献。如果您有意参与其中,请查阅我们的 [贡献者指南](./CONTRIBUTING.md) 。

## 🤝 商务合作

- [预约咨询](https://aao615odquw.feishu.cn/share/base/form/shrcnjw7QleretCLqh1nuPo1xxh)

## 👥 加入社区

扫二维码添加 RAGFlow 小助手，进 RAGFlow 交流群。

<p align="center">
  <img src="https://github.com/infiniflow/ragflow/assets/7248/bccf284f-46f2-4445-9809-8f1030fb7585" width=50% height=50%>
</p>

