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
## 💡 What is RagFlow?

[RagFlow](http://demo.ragflow.io) is a knowledge management platform built on custom-build document understanding engine and LLM, with reasoned and well-founded answers to your question. Clone this repository, you can deploy your own knowledge management platform to empower your business with AI.

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b24a7a5f-4d1d-4a30-90b1-7b0ec558b79d" width="1000"/>
</div>

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

## 🤺RagFlow vs. other RAG applications

| Feature | RagFlow | Langchain-Chatchat | Assistants API | QAnythig | LangChain |
|---------|:---------:|:----------------:|:-----------:|:-----------:|:-----------:|
| **Well-Founded Answer** | :white_check_mark: | :x: | :x: | :x: | :x: |
| **Trackable Chunking** | :white_check_mark: | :x: | :x: | :x: | :x: |
| **Chunking Method** | Rich Variety | Naive | Naive | | Naive | Naive |
| **Table Structure Recognition** | :white_check_mark: | :x: | | :x: | :x: | :x: |
| **Structured Data Lookup** | :white_check_mark: | :x: | :x: | :x: | :x: | :x: |
| **Programming Approach** | API-oriented | API-oriented | API-oriented | API-oriented | Python Code-oriented |
| **RAG Engine** | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: | :x: |
| **Prompt IDE** | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: | :x: |
| **Supported LLMs** | Rich Variety | Rich Variety | OpenAI-only | QwenLLM | Rich Variety |
| **Local Deployment** | :white_check_mark: | :white_check_mark: | :x: | :x: | :x: |
| **Ecosystem Strategy** | Open Source | Open Source | Close Source | Open Source | Open Source |

## 🔎 System Architecture

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 Get Started

### 📝 Prerequisites

- CPU >= 2 cores
- RAM >= 8 GB
- Docker
- `vm.max_map_count` > 65535

> To check the value of `vm.max_map_count`:
>
> ```bash 
> $ sysctl vm.max_map_count
> ```
>
> Reset `vm.max_map_count` to a value greater than 65535 if it is not. In this case, we set it to 262144:
>
> ```bash
> $ sudo sysctl -w vm.max_map_count=262144
> ```
>
> This change will be reset after a system reboot. To ensure your change remains permanent, add or update the following line in **/etc/sysctl.conf** accordingly:
>
> ```bash
> vm.max_map_count=262144
> ```



### Start up the RagFlow server

1. Clone the repo

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

   

2. 

> - In [service_conf.yaml](./docker/service_conf.yaml), configuration of *LLM* in **user_default_llm** is strongly recommended. 
> In **user_default_llm** of [service_conf.yaml](./docker/service_conf.yaml), you need to specify LLM factory and your own _API_KEY_.
> If you do not have _API_KEY_ at the moment, you can specify it in 
Settings the next time you log in to the system.
> - RagFlow supports the flowing LLM factory, with more coming in the pipeline: 
> [OpenAI](https://platform.openai.com/login?launch), [Tongyi-Qianwen](https://dashscope.console.aliyun.com/model), 
> [ZHIPU-AI](https://open.bigmodel.cn/), [Moonshot](https://platform.moonshot.cn/docs/docs)
```bash

$ cd ragflow/docker
$ docker compose up -d
```
### OR

```bash
$ git clone https://github.com/infiniflow/ragflow.git
$ cd ragflow/
$ docker build  -t infiniflow/ragflow:v1.0 .
$ cd ragflow/docker
$ docker compose up -d
```
> The core image is about 15 GB in size and may take a while to load.

Check the server status after pulling all images and having Docker up and running:
```bash
$ docker logs -f  ragflow-server
```
*The following output confirms the successful launch of the system:*

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
In your browser, enter the IP address of your server.



## 🔧 Configurations

> The default serving port is 80, if you want to change that, refer to the [docker-compose.yml](./docker-compose.yaml) and change the left part of `80:80`, say `66:80`.

If you need to change the default setting of the system when you deploy it. There several ways to configure it. 
Please refer to this [README](./docker/README.md) to manually update the configuration. 
Updates to system configurations require a system reboot to take effect *docker-compose up -d* again. 

> If you want to change the basic setups, like port, password .etc., please refer to [.env](./docker/.env) before starting up the system.

> If you change anything in [.env](./docker/.env), please check [service_conf.yaml](./docker/service_conf.yaml) which is a configuration of the back-end service and should be consistent with [.env](./docker/.env).

## 📜 Roadmap

See the [RagFlow Roadmap 2024](https://github.com/infiniflow/ragflow/issues/162)

## 🏄 Community

- [Discord](https://discord.gg/uqQ4YMDf)
- [Twitter](https://twitter.com/infiniflowai)
- GitHub Discussions

## 🙌 Contributing

RagFlow flourishes via open-source collaboration. In this spirit, we embrace diverse contributions from the community. If you would like to be a part, review our [Contribution Guidelines](https://github.com/infiniflow/ragflow/blob/main/CONTRIBUTING.md) first. 
