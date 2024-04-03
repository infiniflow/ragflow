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

## 💡 What is RAGFlow?

[RAGFlow](https://demo.ragflow.io) is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It offers a streamlined RAG workflow for businesses of any scale, combining LLM (Large Language Models) to provide truthful question-answering capabilities, backed by well-founded citations from various complex formatted data.

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

- CPU >= 2 cores
- RAM >= 8 GB
- Docker
  > If you have not installed Docker on your local machine (Windows, Mac, or Linux), see [Install Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Start up the server

1. Ensure `vm.max_map_count` > 65535:

   > To check the value of `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Reset `vm.max_map_count` to a value greater than 65535 if it is not.
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

   ```bash
   $ cd ragflow/docker
   $ docker compose up -d
   ```

   > The core image is about 15 GB in size and may take a while to load.

4. Check the server status after having the server up and running:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _The following output confirms a successful launch of the system:_

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

5. In your web browser, enter the IP address of your server as prompted and log in to RAGFlow.
   > In the given scenario, you only need to enter `http://172.22.0.5` (sans port number) as the default HTTP serving port `80` can be omitted when using the default configurations.
6. In [service_conf.yaml](./docker/service_conf.yaml), select the desired LLM factory in `user_default_llm` and update the `API_KEY` field with the corresponding API key.

   > See [./docs/llm_api_key_setup.md](./docs/llm_api_key_setup.md) for more information.

   _The show is now on!_

## 🔧 Configurations

When it comes to system configurations, you will need to manage the following files:

- [.env](./docker/.env): Keeps the fundamental setups for the system, such as `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, and `MINIO_PASSWORD`.
- [service_conf.yaml](./docker/service_conf.yaml): Configures the back-end services.
- [docker-compose.yml](./docker/docker-compose.yml): The system relies on [docker-compose.yml](./docker/docker-compose.yml) to start up.

You must ensure that changes to the [.env](./docker/.env) file are in line with what are in the [service_conf.yaml](./docker/service_conf.yaml) file.

> The [./docker/README](./docker/README.md) file provides a detailed description of the environment settings and service configurations, and you are REQUIRED to ensure that all environment settings listed in the [./docker/README](./docker/README.md) file are aligned with the corresponding configurations in the [service_conf.yaml](./docker/service_conf.yaml) file.

To update the default HTTP serving port (80), go to [docker-compose.yml](./docker/docker-compose.yml) and change `80:80` to `<YOUR_SERVING_PORT>:80`.

> Updates to all system configurations require a system reboot to take effect:
>
> ```bash
> $ docker-compose up -d
> ```

## 🛠️ Build from source

To build the Docker images from source:

```bash
$ git clone https://github.com/infiniflow/ragflow.git
$ cd ragflow/
$ docker build -t infiniflow/ragflow:v1.0 .
$ cd ragflow/docker
$ docker compose up -d
```

## 📜 Roadmap

See the [RAGFlow Roadmap 2024](https://github.com/infiniflow/ragflow/issues/162)

## 🏄 Community

- [Discord](https://discord.gg/uqQ4YMDf)
- [Twitter](https://twitter.com/infiniflowai)

## 🙌 Contributing

RAGFlow flourishes via open-source collaboration. In this spirit, we embrace diverse contributions from the community. If you would like to be a part, review our [Contribution Guidelines](https://github.com/infiniflow/ragflow/blob/main/docs/CONTRIBUTING.md) first.
