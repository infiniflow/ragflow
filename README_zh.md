<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> 
</p>

<p align="center">
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/RAGFLOW-LLM-white?&labelColor=dd0af7"></a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v1.0-brightgreen"
            alt="docker pull ragflow:v1.0"></a>
      <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
    <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?style=flat-square&labelColor=d4eaf7&color=7d09f1" alt="license">
  </a>
</p>

## 💡 RAGFlow 是什么？

[RAGFlow](https://demo.ragflow.io) 是一款基于深度文档理解构建的开源 RAG（Retrieval-Augmented Generation）引擎。RAGFlow 可以为各种规模的企业及个人提供一套精简的 RAG 工作流程，结合大语言模型（LLM）针对用户各类不同的复杂格式数据提供可靠的问答以及有理有据的引用。

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

- 支持丰富的文件类型，包括 Word 文档、PPT、excel 表格、txt 文件、图片、PDF、影印件、复印件、结构化数据, 网页等。

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

- CPU >= 2 核
- RAM >= 8 GB
- Docker
  > 如果你并没有在本机安装 Docker（Windows、Mac，或者 Linux）, 可以参考文档 [Install Docker Engine](https://docs.docker.com/engine/install/) 自行安装。

### 🚀 启动服务器

1. 确保 `vm.max_map_count` 大于 65535：

   > 如需确认 `vm.max_map_count` 的大小：
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > 如果 `vm.max_map_count` 的值不大于 65535，可以进行重置：
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

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose-CN.yml up -d
   ```

   > 核心镜像文件大约 15 GB，可能需要一定时间拉取。请耐心等待。

4. 服务器启动成功后再次确认服务器状态：

   ```bash
   $ docker logs -f ragflow-server
   ```

   _出现以下界面提示说明服务器启动成功：_

   ```bash
       ____                 ______ __
      / __ \ ____ _ ____ _ / ____// /____  _      __
     / /_/ // __ `// __ `// /_   / // __ \| | /| / /
    / _, _// /_/ // /_/ // __/  / // /_/ /| |/ |/ /
   /_/ |_| \__,_/ \__, //_/    /_/ \____/ |__/|__/
                 /____/

    * Running on all addresses (0.0.0.0)
    * Running on http://127.0.0.1:9380
    * Running on http://172.22.0.5:9380
    INFO:werkzeug:Press CTRL+C to quit
   ```

5. 根据刚才的界面提示在你的浏览器中输入你的服务器对应的 IP 地址并登录 RAGFlow。
   > 上面这个例子中，您只需输入 http://172.22.0.5 即可：未改动过配置则无需输入端口（默认的 HTTP 服务端口 80）。
6. 在 [service_conf.yaml](./docker/service_conf.yaml) 文件的 `user_default_llm` 栏配置 LLM factory，并在 `API_KEY` 栏填写和你选择的大模型相对应的 API key。

   > 详见 [./docs/llm_api_key_setup.md](./docs/llm_api_key_setup.md)。

   _好戏开始，接着奏乐接着舞！_

## 🔧 系统配置

系统配置涉及以下三份文件：

- [.env](./docker/.env)：存放一些基本的系统环境变量，比如 `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` 等。
- [service_conf.yaml](./docker/service_conf.yaml)：配置各类后台服务。
- [docker-compose-CN.yml](./docker/docker-compose-CN.yml): 系统依赖该文件完成启动。

请务必确保 [.env](./docker/.env) 文件中的变量设置与 [service_conf.yaml](./docker/service_conf.yaml) 文件中的配置保持一致！

> [./docker/README](./docker/README.md) 文件提供了环境变量设置和服务配置的详细信息。请**一定要**确保 [./docker/README](./docker/README.md) 文件当中列出来的环境变量的值与 [service_conf.yaml](./docker/service_conf.yaml) 文件当中的系统配置保持一致。

如需更新默认的 HTTP 服务端口(80), 可以在 [docker-compose-CN.yml](./docker/docker-compose-CN.yml) 文件中将配置 `80:80` 改为 `<YOUR_SERVING_PORT>:80`。

> 所有系统配置都需要通过系统重启生效：
>
> ```bash
> $ docker compose up -f docker-compose-CN.yml -d
> ```

## 🛠️ 源码编译、安装 Docker 镜像

如需从源码安装 Docker 镜像：

```bash
$ git clone https://github.com/infiniflow/ragflow.git
$ cd ragflow/
$ docker build -t infiniflow/ragflow:v1.0 .
$ cd ragflow/docker
$ docker compose up -d
```

## 📜 路线图

详见 [RAGFlow Roadmap 2024](https://github.com/infiniflow/ragflow/issues/162)。

## 🏄 开源社区

- [Discord](https://discord.gg/uqQ4YMDf)
- [Twitter](https://twitter.com/infiniflowai)

## 🙌 贡献指南

RAGFlow 只有通过开源协作才能蓬勃发展。秉持这一精神,我们欢迎来自社区的各种贡献。如果您有意参与其中,请查阅我们的[贡献者指南](https://github.com/infiniflow/ragflow/blob/main/docs/CONTRIBUTING.md)。

## 👥 加入社区

扫二维码添加 RAGFlow 小助手，进 RAGFlow 交流群。

<p align="center">
  <img src="https://github.com/infiniflow/ragflow/assets/7248/bccf284f-46f2-4445-9809-8f1030fb7585" width=50% height=50%>
</p>

