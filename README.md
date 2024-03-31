<div align="center">
<a href="https://demo.ragflow.io/">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/f034fb27-b3bf-401b-b213-e1dfa7448d2a" width="320" alt="ragflow logo">
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

[RAGFlow](http://demo.ragflow.io) is a knowledge management platform built on custom-build document understanding engine and LLM, with reasoned and well-founded answers to your question. Clone this repository, you can deploy your own knowledge management platform to empower your business with AI.

## 🌟 Key Features

- 🍭**Custom-build document understanding engine.** Our deep learning engine is made according to the needs of analyzing and searching various type of documents in different domain.
  - For documents from different domain for different purpose, the engine applies different analyzing and search strategy.
  - Easily intervene and manipulate the data proccessing procedure when things goes beyond expectation.
  - Multi-media document understanding is supported using OCR and multi-modal LLM. 
- 🍭**State-of-the-art table structure and layout recognition.** Precisely extract and understand the document including table content. See [README.](./deepdoc/README.md)
  - For PDF files, layout and table structures including row, column and span of them are recognized.
  - Put the table accrossing the pages together.
  - Reconstruct the table structure components into html table.  
- **Querying database dumped data are supported.** After uploading tables from any database, you can search any data records just by asking.
  - You can now query a database using natural language instead of using SQL.
  - The record number uploaded is not limited. 
- **Reasoned and well-founded answers.** The cited document part in LLM's answer is provided and pointed out in the original document.
  - The answers are based on retrieved result for which we apply vector-keyword hybrids search and re-rank.
  - The part of document cited in the answer is presented in the most expressive way.
  - For PDF file, the cited parts in document can be located in the original PDF.  

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

### Start up the server

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

4. Check the server status after pulling all images and having Docker up and running:
   ```bash
   $ docker logs -f ragflow-server
   ```
   *The following output confirms a successful launch of the system:*

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

5. In your web browser, enter the IP address of your server as prompted.
   
   *The show is on!*


## 🔧 Configurations

When it comes to system configurations, you will need to manage the following files:

- [.env](./docker/.env): Keeps the fundamental setups for the system, such as `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, and `MINIO_PASSWORD`.
- [service_conf.yaml](./docker/service_conf.yaml): Configures the back-end services.
- [docker-compose.yml](./docker-compose.yaml): The system relies on [docker-compose.yml](./docker-compose.yaml) to start up.


You must ensure that changes in [.env](./docker/.env) are in line with what are in the [service_conf.yaml](./docker/service_conf.yaml) file. 

> The [./docker/README](./docker/README.md) file provides a detailed description of the environment settings and service configurations, and it is IMPORTANT to ensure that all environment settings listed in the [./docker/README](./docker/README.md) file should be aligned with the corresponding settings in the [service_conf.yaml](./docker/service_conf.yaml) file.

To change the default serving port (80), go to [docker-compose.yml](./docker-compose.yaml) and change `80:80` to `<YOUR_SERVING_PORT>:80`.

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

RAGFlow flourishes via open-source collaboration. In this spirit, we embrace diverse contributions from the community. If you would like to be a part, review our [Contribution Guidelines](https://github.com/infiniflow/ragflow/blob/main/CONTRIBUTING.md) first. 
