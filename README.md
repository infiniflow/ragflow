<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a>
</p>

<p align="center">
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99"></a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.12.0-brightgreen" alt="docker pull infiniflow/ragflow:v0.12.0"></a>
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

<details open>
<summary></b>📕 Table of Contents</b></summary>
  
- 💡 [What is RAGFlow?](#-what-is-ragflow)
- 🎮 [Demo](#-demo)
- 📌 [Latest Updates](#-latest-updates)
- 🌟 [Key Features](#-key-features)
- 🔎 [System Architecture](#-system-architecture)
- 🎬 [Get Started](#-get-started)
- 🔧 [Configurations](#-configurations)
- 🔧 [Build a docker image without embedding models](#-build-a-docker-image-without-embedding-models)
- 🔧 [Build a docker image including embedding models](#-build-a-docker-image-including-embedding-models)
- 🔨 [Launch service from source for development](#-launch-service-from-source-for-development)
- 📚 [Documentation](#-documentation)
- 📜 [Roadmap](#-roadmap)
- 🏄 [Community](#-community)
- 🙌 [Contributing](#-contributing)

</details>

## 💡 What is RAGFlow?

[RAGFlow](https://ragflow.io/) is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It offers a streamlined RAG workflow for businesses of any scale, combining LLM (Large Language Models) to provide truthful question-answering capabilities, backed by well-founded citations from various complex formatted data.

## 🎮 Demo

Try our demo at [https://demo.ragflow.io](https://demo.ragflow.io).
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b083d173-dadc-4ea9-bdeb-180d7df514eb" width="1200"/>
</div>


## 🔥 Latest Updates

- 2024-09-29 Optimizes multi-round conversations.
- 2024-09-13 Adds search mode for knowledge base Q&A.
- 2024-09-09 Adds a medical consultant agent template.
- 2024-08-22 Support text to SQL statements through RAG.
- 2024-08-02 Supports GraphRAG inspired by [graphrag](https://github.com/microsoft/graphrag) and mind map.

## 🎉 Stay Tuned
⭐️ Star our repository to stay up-to-date with exciting new features and improvements! Get instant notifications for new releases! 🌟
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Key Features

### 🍭 **"Quality in, quality out"**

- [Deep document understanding](./deepdoc/README.md)-based knowledge extraction from unstructured data with complicated formats.
- Finds "needle in a data haystack" of literally unlimited tokens.

### 🍱 **Template-based chunking**

- Intelligent and explainable.
- Plenty of template options to choose from.

### 🌱 **Grounded citations with reduced hallucinations**

- Visualization of text chunking to allow human intervention.
- Quick view of the key references and traceable citations to support grounded answers.

### 🍔 **Compatibility with heterogeneous data sources**

- Supports Word, slides, excel, txt, images, scanned copies, structured data, web pages, and more.

### 🛀 **Automated and effortless RAG workflow**

- Streamlined RAG orchestration catered to both personal and large businesses.
- Configurable LLMs as well as embedding models.
- Multiple recall paired with fused re-ranking.
- Intuitive APIs for seamless integration with business.

## 🔎 System Architecture

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 Get Started

### 📝 Prerequisites

- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
  > If you have not installed Docker on your local machine (Windows, Mac, or Linux), see [Install Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Start up the server

1. Ensure `vm.max_map_count` >= 262144:

   > To check the value of `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Reset `vm.max_map_count` to a value at least 262144 if it is not.
   >
   > ```bash
   > # In this case, we set it to 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > This change will be reset after a system reboot. To ensure your change remains permanent, add or update the `vm.max_map_count` value in **/etc/sysctl.conf** accordingly:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. Clone the repo:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. Build the pre-built Docker images and start up the server:

   > The command below downloads the dev version Docker image for RAGFlow slim (`dev-slim`). Note that RAGFlow slim Docker images do not include embedding models or Python libraries and hence are approximately 1GB in size.

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose.yml up -d
   ```

   > - To download a RAGFlow slim Docker image of a specific version, update the `RAGFlow_IMAGE` variable in **docker/.env** to your desired version. For example, `RAGFLOW_IMAGE=infiniflow/ragflow:v0.12.0-slim`. After making this change, rerun the command above to initiate the download.
   > - To download the dev version of RAGFlow Docker image *including* embedding models and Python libraries, update the `RAGFlow_IMAGE` variable in **docker/.env** to `RAGFLOW_IMAGE=infiniflow/ragflow:dev`. After making this change, rerun the command above to initiate the download.
   > - To download a specific version of RAGFlow Docker image *including* embedding models and Python libraries, update the `RAGFlow_IMAGE` variable in **docker/.env** to your desired version. For example, `RAGFLOW_IMAGE=infiniflow/ragflow:v0.12.0`. After making this change, rerun the command above to initiate the download.  
   
   > **NOTE:** A RAGFlow Docker image that includes embedding models and Python libraries is approximately 9GB in size and may take significantly longer time to load.

4. Check the server status after having the server up and running:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _The following output confirms a successful launch of the system:_

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
   > If you skip this confirmation step and directly log in to RAGFlow, your browser may prompt a `network abnormal` error because, at that moment, your RAGFlow may not be fully initialized.  

5. In your web browser, enter the IP address of your server and log in to RAGFlow.
   > With the default settings, you only need to enter `http://IP_OF_YOUR_MACHINE` (**sans** port number) as the default HTTP serving port `80` can be omitted when using the default configurations.
6. In [service_conf.yaml](./docker/service_conf.yaml), select the desired LLM factory in `user_default_llm` and update the `API_KEY` field with the corresponding API key.

   > See [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) for more information.

   _The show is on!_

## 🔧 Configurations

When it comes to system configurations, you will need to manage the following files:

- [.env](./docker/.env): Keeps the fundamental setups for the system, such as `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, and `MINIO_PASSWORD`.
- [service_conf.yaml](./docker/service_conf.yaml): Configures the back-end services.
- [docker-compose.yml](./docker/docker-compose.yml): The system relies on [docker-compose.yml](./docker/docker-compose.yml) to start up.

You must ensure that changes to the [.env](./docker/.env) file are in line with what are in the [service_conf.yaml](./docker/service_conf.yaml) file.

> The [./docker/README](./docker/README.md) file provides a detailed description of the environment settings and service configurations, and you are REQUIRED to ensure that all environment settings listed in the [./docker/README](./docker/README.md) file are aligned with the corresponding configurations in the [service_conf.yaml](./docker/service_conf.yaml) file.

To update the default HTTP serving port (80), go to [docker-compose.yml](./docker/docker-compose.yml) and change `80:80` to `<YOUR_SERVING_PORT>:80`.

Updates to the above configurations require a reboot of all containers to take effect:

> ```bash
> $ docker compose -f docker/docker-compose.yml up -d
> ```

## 🔧 Build a Docker image without embedding models

This image is approximately 1 GB in size and relies on external LLM and embedding services.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```

## 🔧 Build a Docker image including embedding models

This image is approximately 9 GB in size. As it includes embedding models, it relies on external LLM services only.  

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```

## 🔨 Launch service from source for development

1. Install Poetry, or skip this step if it is already installed:  
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. Clone the source code and install Python dependencies:  
   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ~/.local/bin/poetry install --sync --no-root # install RAGFlow dependent python modules
   ```

3. Launch the dependent services (MinIO, Elasticsearch, Redis, and MySQL) using Docker Compose:  
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Add the following line to `/etc/hosts` to resolve all hosts specified in **docker/service_conf.yaml** to `127.0.0.1`:  
   ```
   127.0.0.1       es01 mysql minio redis
   ```  
   In **docker/service_conf.yaml**, update mysql port to `5455` and es port to `1200`, as specified in **docker/.env**.

4. If you cannot access HuggingFace, set the `HF_ENDPOINT` environment variable to use a mirror site:  
 
   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. Launch backend service:  
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

6. Install frontend dependencies:  
   ```bash
   cd web
   npm install --force
   ```  
7. Configure frontend to update `proxy.target` in **.umirc.ts** to `http://127.0.0.1:9380`:
8. Launch frontend service:  
   ```bash
   npm run dev 
   ```  

   _The following output confirms a successful launch of the system:_  

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

## 📚 Documentation

- [Quickstart](https://ragflow.io/docs/dev/)
- [User guide](https://ragflow.io/docs/dev/category/guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

See the [RAGFlow Roadmap 2024](https://github.com/infiniflow/ragflow/issues/162)

## 🏄 Community

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Contributing

RAGFlow flourishes via open-source collaboration. In this spirit, we embrace diverse contributions from the community. If you would like to be a part, review our [Contribution Guidelines](./CONTRIBUTING.md) first.
