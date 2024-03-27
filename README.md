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

[RagFlow](http://demo.ragflow.io) is a knowledge management platform built on custom-build document understanding engine and LLM, 
with reasoned and well-founded answers to your question. Clone this repository, you can deploy your own knowledge management 
platform to empower your business with AI.
    
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b24a7a5f-4d1d-4a30-90b1-7b0ec558b79d" width="1000"/>
</div>

# Key Features
- **Custom-build document understanding engine.** Our deep learning engine is made according to the needs of analyzing and searching various type of documents in different domain.
  - For documents from different domain for different purpose, the engine applys different analyzing and search strategy.
  - Easily intervene and manipulate the data proccessing procedure when things goes beyond expectation.
  - Multi-media document understanding is supported using OCR and multi-modal LLM. 
- **State-of-the-art table structure and layout recognition.** Precisely extract and understand the document including table content. See [README.](./deepdoc/README.md)
  - For PDF files, layout and table structures including row, column and span of them are recognized.
  - Put the table accrossing the pages together.
  - Reconstruct the table structure components into html table.  
- **Querying database dumped data are supported.** After uploading tables from any database, you can search any data records just by asking.
  - Instead of using SQL to query a database, every one cat get the wanted data just by asking using natrual language.
  - The record number uploaded is not limited.
  - Some extra description of column headers should be provided.  
- **Reasoned and well-founded answers.** The cited document part in LLM's answer is provided and pointed out in the original document.
  - The answers are based on retrieved result for which we apply vector-keyword hybrids search and rerank.
  - The part of document cited in the answer is presented in the most expressive way.
  - For PDF file, the cited parts in document can be located in the original PDF.  
    

# Release Notification
**Star us on GitHub, and be notified for a new releases instantly!**
![star-us](https://github.com/infiniflow/ragflow/assets/12318111/2c2fbb5e-c403-496f-a1fd-64ba0fdbf74f)

# Installation
## System Requirements
Be aware of the system minimum requirements before starting installation.
- CPU >= 2 cores
- RAM >= 8GB

Then, you need to check the following command:
```bash
121:/ragflow# sysctl vm.max_map_count
vm.max_map_count = 262144
```
If **vm.max_map_count** is not larger  than 65535, please run the following commands:
```bash
121:/ragflow# sudo sysctl -w vm.max_map_count=262144
```
However, this change is not persistent and will be reset after a system reboot. 
To make the change permanent, you need to update the **/etc/sysctl.conf**.
Add or update the following line in the file:
```bash
vm.max_map_count=262144
```

## Install docker

If your machine doesn't have *Docker* installed, please refer to [Install Docker Engine](https://docs.docker.com/engine/install/)

## Quick Start

> - In [service_conf.yaml](./docker/service_conf.yaml), configuration of *LLM* in **user_default_llm** is strongly recommended. 
> In **user_default_llm** of [service_conf.yaml](./docker/service_conf.yaml), you need to specify LLM factory and your own _API_KEY_.
> It's O.K if you don't have _API_KEY_ at the moment, you can specify it later at the setting part after starting and logging in the system.
> - We have supported the flowing LLM factory, and the others is coming soon: 
> [OpenAI](https://platform.openai.com/login?launch), [Tongyi-Qianwen](https://dashscope.console.aliyun.com/model), 
> [ZHIPU-AI](https://open.bigmodel.cn/), [Moonshot](https://platform.moonshot.cn/docs/docs)
```bash
$ git clone https://github.com/infiniflow/ragflow.git
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
> The core image is about 15GB, please be patient for the first time

After pulling all the images and running up, use the following command to check the server status. If you can have the following outputs, 
_**Hallelujah!**_ You have successfully launched the system.
```bash
$ docker logs -f  ragflow-server

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
Open your browser, enter the IP address of your server, _**Hallelujah**_ again!
> The default serving port is 80, if you want to change that, please refer to [docker-compose.yml](./docker-compose.yaml), 
> and change the left part of *'80:80'*'.

# System Architecture Diagram

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/39c8e546-51ca-4b50-a1da-83731b540cd0" width="1000"/>
</div>

# Configuration
If you need to change the default setting of the system when you deploy it. There several ways to configure it. 
Please refer to [README](./docker/README.md) and manually set the configuration. 
After changing something, please run *docker-compose up -d* again. 

> If you want to change the basic setups, like port, password .etc., please refer to [.env](./docker/.env) before starting the system.

> If you change anything in [.env](./docker/.env), please check [service_conf.yaml](./docker/service_conf.yaml) which is a 
> configuration of the back-end service and should be consistent with [.env](./docker/.env).

# RoadMap

- [ ] File manager.
- [ ] Support URLs. Crawl web and extract the main content.


# Contributing

For those who'd like to contribute code, see our [Contribution Guide](https://github.com/infiniflow/ragflow/blob/main/CONTRIBUTING.md). 

# License

This repository is available under the [Ragflow Open Source License](LICENSE), which is essentially Apache 2.0 with a few additional restrictions.
