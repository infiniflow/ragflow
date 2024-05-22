---
sidebar_position: 3
slug: /faq
---

# Frequently asked questions

## General

### 1. What sets RAGFlow apart from other RAG products?

The "garbage in garbage out" status quo remains unchanged despite the fact that LLMs have advanced Natural Language Processing (NLP) significantly. In response, RAGFlow introduces two unique features compared to other Retrieval-Augmented Generation (RAG) products.

- Fine-grained document parsing: Document parsing involves images and tables, with the flexibility for you to intervene as needed.
- Traceable answers with reduced hallucinations: You can trust RAGFlow's responses as you can view the citations and references supporting them.

### 2. Which languages does RAGFlow support?

English, simplified Chinese, traditional Chinese for now. 

## Performance

### 1. Why does it take longer for RAGFlow to parse a document than LangChain?

We put painstaking effort into document pre-processing tasks like layout analysis, table structure recognition, and OCR (Optical Character Recognition) using our vision model. This contributes to the additional time required. 

### 2. Why does RAGFlow require more resources than other projects?

RAGFlow has a number of built-in models for document structure parsing, which account for the additional computational resources.

## Feature

### 1. Which architectures or devices does RAGFlow support?

Currently, we only support x86 CPU and Nvidia GPU. 

### 2. Do you offer an API for integration with third-party applications?

The corresponding APIs are now available. See the [RAGFlow API Reference](./api.md) for more information. 

### 3. Do you support stream output?

No, this feature is still in development. Contributions are welcome. 

### 4. Is it possible to share dialogue through URL?

Yes, this feature is now available.

### 5. Do you support multiple rounds of dialogues, i.e., referencing previous dialogues as context for the current dialogue?

This feature and the related APIs are still in development. Contributions are welcome.


## Troubleshooting

### 1. Issues with docker images

#### 1.1 How to build the RAGFlow image from scratch?

```
$ git clone https://github.com/infiniflow/ragflow.git
$ cd ragflow
$ docker build -t infiniflow/ragflow:latest .
$ cd ragflow/docker
$ chmod +x ./entrypoint.sh
$ docker compose up -d
```

#### 1.2 `process "/bin/sh -c cd ./web && npm i && npm run build"` failed

1. Check your network from within Docker, for example:
```bash
curl https://hf-mirror.com
```

2. If your network works fine, the issue lies with the Docker network configuration. Replace the Docker building command:
```bash
docker build -t infiniflow/ragflow:vX.Y.Z.
```
   With this:  
```bash
docker build -t infiniflow/ragflow:vX.Y.Z. --network host
```

### 2. Issues with huggingface models

#### 2.1 Cannot access https://huggingface.co
 
A *locally* deployed RAGflow downloads OCR and embedding modules from [Huggingface website](https://huggingface.co) by default. If your machine is unable to access this site, the following error occurs and PDF parsing fails: 

```
FileNotFoundError: [Errno 2] No such file or directory: '/root/.cache/huggingface/hub/models--InfiniFlow--deepdoc/snapshots/be0c1e50eef6047b412d1800aa89aba4d275f997/ocr.res'
```
 To fix this issue, use https://hf-mirror.com instead:

 1. Stop all containers and remove all related resources:

 ```bash
 cd ragflow/docker/
 docker compose down
 ```

 2. Replace `https://huggingface.co` with `https://hf-mirror.com` in **ragflow/docker/docker-compose.yml**.
 
 3. Start up the server: 

 ```bash
 docker compose up -d 
 ```

#### 2.2. `MaxRetryError: HTTPSConnectionPool(host='hf-mirror.com', port=443)`

This error suggests that you do not have Internet access or are unable to connect to hf-mirror.com. Try the following: 

1. Manually download the resource files from [huggingface.co/InfiniFlow/deepdoc](https://huggingface.co/InfiniFlow/deepdoc) to your local folder **~/deepdoc**. 
2. Add a volumes to **docker-compose.yml**, for example:
```
- ~/deepdoc:/ragflow/rag/res/deepdoc
```

#### 2.3 `FileNotFoundError: [Errno 2] No such file or directory: '/root/.cache/huggingface/hub/models--InfiniFlow--deepdoc/snapshots/FileNotFoundError: [Errno 2] No such file or directory: '/ragflow/rag/res/deepdoc/ocr.res'be0c1e50eef6047b412d1800aa89aba4d275f997/ocr.res'`

1. Check your network from within Docker, for example: 
```bash
curl https://hf-mirror.com
```
2. Run `ifconfig` to check the `mtu` value. If the server's `mtu` is `1450` while the NIC's `mtu` in the container is `1500`, this mismatch may cause network instability. Adjust the `mtu` policy as follows:

```
vim docker-compose-base.yml
# Original configuration：
networks:
  ragflow:
    driver: bridge
# Modified configuration：
networks:
  ragflow:
    driver: bridge
    driver_opts:
      com.docker.network.driver.mtu: 1450
```

### 3. Issues with RAGFlow servers

#### 3.1 `WARNING: can't find /raglof/rag/res/borker.tm`

Ignore this warning and continue. All system warnings can be ignored.

#### 3.2 `network anomaly There is an abnormality in your network and you cannot connect to the server.`

![anomaly](https://github.com/infiniflow/ragflow/assets/93570324/beb7ad10-92e4-4a58-8886-bfb7cbd09e5d)

You will not log in to RAGFlow unless the server is fully initialized. Run `docker logs -f ragflow-server`.

*The server is successfully initialized, if your system displays the following:*

```
    ____                 ______ __
   / __ \ ____ _ ____ _ / ____// /____  _      __
  / /_/ // __ `// __ `// /_   / // __ \| | /| / /
 / _, _// /_/ // /_/ // __/  / // /_/ /| |/ |/ /
/_/ |_| \__,_/ \__, //_/    /_/ \____/ |__/|__/
              /____/

 * Running on all addresses (0.0.0.0)
 * Running on http://127.0.0.1:9380
 * Running on http://x.x.x.x:9380
 INFO:werkzeug:Press CTRL+C to quit
```


### 4. Issues with RAGFlow backend services

#### 4.1 `dependency failed to start: container ragflow-mysql is unhealthy`

`dependency failed to start: container ragflow-mysql is unhealthy` means that your MySQL container failed to start. Try replacing `mysql:5.7.18` with `mariadb:10.5.8` in **docker-compose-base.yml**.

#### 4.2 `Realtime synonym is disabled, since no redis connection`

Ignore this warning and continue. All system warnings can be ignored.

![](https://github.com/infiniflow/ragflow/assets/93570324/ef5a6194-084a-4fe3-bdd5-1c025b40865c)

#### 4.3 Why does it take so long to parse a 2MB document?

Parsing requests have to wait in queue due to limited server resources. We are currently enhancing our algorithms and increasing computing power.

#### 4.4 Why does my document parsing stall at under one percent?

![stall](https://github.com/infiniflow/ragflow/assets/93570324/3589cc25-c733-47d5-bbfc-fedb74a3da50)

If your RAGFlow is deployed *locally*, try the following: 

1. Check the log of your RAGFlow server to see if it is running properly:
```bash
docker logs -f ragflow-server
```
2. Check if the **task_executor.py** process exists.
3. Check if your RAGFlow server can access hf-mirror.com or huggingface.com.

#### 4.5 Why does my pdf parsing stall near completion, while the log does not show any error?

If your RAGFlow is deployed *locally*, the parsing process is likely killed due to insufficient RAM. Try increasing your memory allocation by increasing the `MEM_LIMIT` value in **docker/.env**.

> Ensure that you restart up your RAGFlow server for your changes to take effect!
> ```bash
> docker compose stop
> ```
> ```bash
> docker compose up -d
> ```

![nearcompletion](https://github.com/infiniflow/ragflow/assets/93570324/563974c3-f8bb-4ec8-b241-adcda8929cbb)

#### 4.6 `Index failure`

An index failure usually indicates an unavailable Elasticsearch service.

#### 4.7 How to check the log of RAGFlow?

```bash
tail -f path_to_ragflow/docker/ragflow-logs/rag/*.log
```

#### 4.8 How to check the status of each component in RAGFlow?

```bash
$ docker ps
```
*The system displays the following if all your RAGFlow components are running properly:* 

```
5bc45806b680   infiniflow/ragflow:latest     "./entrypoint.sh"        11 hours ago   Up 11 hours               0.0.0.0:80->80/tcp, :::80->80/tcp, 0.0.0.0:443->443/tcp, :::443->443/tcp, 0.0.0.0:9380->9380/tcp, :::9380->9380/tcp   ragflow-server
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
d8c86f06c56b   mysql:5.7.18        "docker-entrypoint.s…"   7 days ago     Up 16 seconds (healthy)   0.0.0.0:3306->3306/tcp, :::3306->3306/tcp     ragflow-mysql
cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
```

#### 4.9 `Exception: Can't connect to ES cluster`

1. Check the status of your Elasticsearch component:

```bash
$ docker ps
```
   *The status of a 'healthy' Elasticsearch component in your RAGFlow should look as follows:*
```
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
```

2. If your container keeps restarting, ensure `vm.max_map_count` >= 262144 as per [this README](https://github.com/infiniflow/ragflow?tab=readme-ov-file#-start-up-the-server). Updating the `vm.max_map_count` value in **/etc/sysctl.conf** is required, if you wish to keep your change permanent. This configuration works only for Linux.


3. If your issue persists, ensure that the ES host setting is correct:

    - If you are running RAGFlow with Docker, it is in **docker/service_conf.yml**. Set it as follows:
    ```
    es:
      hosts: 'http://es01:9200'
    ```
    - If you run RAGFlow outside of Docker, verify the ES host setting in **conf/service_conf.yml** using: 
    ```bash
    curl http://<IP_OF_ES>:<PORT_OF_ES>
    ```

#### 4.10 Can't start ES container and get `Elasticsearch did not exit normally`

This is because you forgot to update the `vm.max_map_count` value in **/etc/sysctl.conf** and your change to this value was reset after a system reboot. 

#### 4.11 `{"data":null,"retcode":100,"retmsg":"<NotFound '404: Not Found'>"}`

Your IP address or port number may be incorrect. If you are using the default configurations, enter `http://<IP_OF_YOUR_MACHINE>` (**NOT 9380, AND NO PORT NUMBER REQUIRED!**) in your browser. This should work.

#### 4.12 `Ollama - Mistral instance running at 127.0.0.1:11434 but cannot add Ollama as model in RagFlow`

A correct Ollama IP address and port is crucial to adding models to Ollama:

- If you are on demo.ragflow.io, ensure that the server hosting Ollama has a publicly accessible IP address.Note that 127.0.0.1 is not a publicly accessible IP address.
- If you deploy RAGFlow locally, ensure that Ollama and RAGFlow are in the same LAN and can comunicate with each other.

#### 4.13 Do you offer examples of using deepdoc to parse PDF or other files?

Yes, we do. See the Python files under the **rag/app** folder. 

#### 4.14 Why did I fail to upload a 10MB+ file to my locally deployed RAGFlow?

You probably forgot to update the **MAX_CONTENT_LENGTH** environment variable:

1. Add environment variable `MAX_CONTENT_LENGTH` to **ragflow/docker/.env**:
```
MAX_CONTENT_LENGTH=100000000
```
2. Update **docker-compose.yml**:
```
environment:
  - MAX_CONTENT_LENGTH=${MAX_CONTENT_LENGTH}
```
3. Restart the RAGFlow server:
```
docker compose up ragflow -d
```
   *Now you should be able to upload files of sizes less than 100MB.*

#### 4.15 `Table 'rag_flow.document' doesn't exist`

This exception occurs when starting up the RAGFlow server. Try the following: 

  1. Prolong the sleep time: Go to **docker/entrypoint.sh**, locate line 26, and replace `sleep 60` with `sleep 280`.
  2. If using Windows, ensure that the **entrypoint.sh** has LF end-lines.
  3. Go to **docker/docker-compose.yml**, add the following:
  ```
  ./entrypoint.sh:/ragflow/entrypoint.sh
  ```
  4. Change directory:
  ```bash
  cd docker
  ```
  5. Stop the RAGFlow server:
  ```bash
  docker compose stop
  ```
  6. Restart up the RAGFlow server:
  ```bash
  docker compose up
  ```

#### 4.16 `hint : 102  Fail to access model  Connection error`

![hint102](https://github.com/infiniflow/ragflow/assets/93570324/6633d892-b4f8-49b5-9a0a-37a0a8fba3d2)

1. Ensure that the RAGFlow server can access the base URL.
2. Do not forget to append **/v1/** to **http://IP:port**: 
   **http://IP:port/v1/**

#### 4.17 `FileNotFoundError: [Errno 2] No such file or directory`

1. Check if the status of your minio container is healthy:
   ```bash
   docker ps
   ```
2. Ensure that the username and password settings of MySQL and MinIO in **docker/.env** are in line with those in **docker/service_conf.yml**.

## Usage

### 1. How to increase the length of RAGFlow responses?

1. Right click the desired dialog to display the **Chat Configuration** window.
2. Switch to the **Model Setting** tab and adjust the **Max Tokens** slider to get the desired length.
3. Click **OK** to confirm your change. 


### 2. What does Empty response mean? How to set it?

You limit what the system responds to what you specify in **Empty response** if nothing is retrieved from your knowledge base. If you do not specify anything in **Empty response**, you let your LLM improvise, giving it a chance to hallucinate.

### 3. Can I set the base URL for OpenAI somewhere?

![](https://github.com/infiniflow/ragflow/assets/93570324/8cfb6fa4-8a97-415d-b9fa-b6f405a055f3)

### 4. How to run RAGFlow with a locally deployed LLM?

You can use Ollama to deploy local LLM. See [here](https://github.com/infiniflow/ragflow/blob/main/docs/guides/deploy_local_llm.md) for more information. 

### 5. How to link up ragflow and ollama servers?

- If RAGFlow is locally deployed, ensure that your RAGFlow and Ollama are in the same LAN. 
- If you are using our online demo, ensure that the IP address of your Ollama server is public and accessible.

### 6. How to configure RAGFlow to respond with 100% matched results, rather than utilizing LLM?

1. Click **Knowledge Base** in the middle top of the page.
2. Right click the desired knowledge base to display the **Configuration** dialogue. 
3. Choose **Q&A** as the chunk method and click **Save** to confirm your change. 

### 7. Do I need to connect to Redis?

No, connecting to Redis is not required. 

### 8. `Error: Range of input length should be [1, 30000]`

This error occurs because there are too many chunks matching your search criteria. Try reducing the **TopN** and increasing **Similarity threshold** to fix this issue: 

1. Click **Chat** in the middle top of the page. 
2. Right click the desired conversation > **Edit** > **Prompt Engine**
3. Reduce the **TopN** and/or raise **Silimarity threshold**.
4. Click **OK** to confirm your changes.

![topn](https://github.com/infiniflow/ragflow/assets/93570324/7ec72ab3-0dd2-4cff-af44-e2663b67b2fc)

### 9. How to upgrade RAGFlow?
   
You can upgrade RAGFlow to either the dev version or the latest version:

- Dev versions are for developers and contributors. They are published on a nightly basis and may crash because they are not fully tested. We cannot guarantee their validity and you are at your own risk trying out latest, untested features.
- The latest version refers to the most recent, officially published release. It is stable and works best with regular users.


To upgrade RAGFlow to the dev version:

1. Pull the latest source code
   ```bash
   cd ragflow
   git pull
   ```
2. If you used `docker compose up -d` to start up RAGFlow server:
   ```bash
   docker pull infiniflow/ragflow:dev
   ```
   ```bash
   docker compose up ragflow -d
   ```
3. If you used `docker compose -f docker-compose-CN.yml up -d` to start up RAGFlow server:
   ```bash
   docker pull swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:dev
   ```
   ```bash
   docker compose -f docker-compose-CN.yml up -d
   ```
   
To upgrade RAGFlow to the latest version:

1. Update **ragflow/docker/.env** as follows:
   ```bash
   RAGFLOW_VERSION=latest
   ```
2. Pull the latest source code:
   ```bash
   cd ragflow
   git pull
   ```   

3. If you used `docker compose up -d` to start up RAGFlow server:
   ```bash
   docker pull infiniflow/ragflow:latest
   ```
   ```bash
   docker compose up ragflow -d
   ```
4. If you used `docker compose -f docker-compose-CN.yml up -d` to start up RAGFlow server:
   ```bash
   docker pull swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:latest
   ```
   ```bash
   docker compose -f docker-compose-CN.yml up -d
   ```
