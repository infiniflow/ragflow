---
sidebar_position: 10
slug: /faq
---

# FAQs

Answers to questions about general features, troubleshooting, usage, and more.

---

## General features

---

### What sets RAGFlow apart from other RAG products?

The "garbage in garbage out" status quo remains unchanged despite the fact that LLMs have advanced Natural Language Processing (NLP) significantly. In response, RAGFlow introduces two unique features compared to other Retrieval-Augmented Generation (RAG) products.

- Fine-grained document parsing: Document parsing involves images and tables, with the flexibility for you to intervene as needed.
- Traceable answers with reduced hallucinations: You can trust RAGFlow's responses as you can view the citations and references supporting them.

---

### Where to find the version of RAGFlow? How to interpret it?

You can find the RAGFlow version number on the **System** page of the UI:

![Image](https://github.com/user-attachments/assets/20cf7213-2537-4e18-a88c-4dadf6228c6b)

If you build RAGFlow from source, the version number is also in the system log:

```
        ____   ___    ______ ______ __               
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ / 
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/                             

2025-02-18 10:10:43,835 INFO     1445658 RAGFlow version: v0.15.0-50-g6daae7f2 full
```

Where:

- `v0.15.0`: The officially published release.
- `50`: The number of git commits since the official release.
- `g6daae7f2`: `g` is the prefix, and `6daae7f2` is the first seven characters of the current commit ID.
- `full`/`slim`: The RAGFlow edition.
  - `full`: The full RAGFlow edition.
  - `slim`: The RAGFlow edition without embedding models and Python packages.

---

### Why does it take longer for RAGFlow to parse a document than LangChain?

We put painstaking effort into document pre-processing tasks like layout analysis, table structure recognition, and OCR (Optical Character Recognition) using our vision models. This contributes to the additional time required.

---

### Why does RAGFlow require more resources than other projects?

RAGFlow has a number of built-in models for document structure parsing, which account for the additional computational resources.

---

### Which architectures or devices does RAGFlow support?

We officially support x86 CPU and nvidia GPU. While we also test RAGFlow on ARM64 platforms, we do not maintain RAGFlow Docker images for ARM. If you are on an ARM platform, follow [this guide](./develop/build_docker_image.mdx) to build a RAGFlow Docker image.

---

### Which embedding models can be deployed locally?

RAGFlow offers two Docker image editions, `v0.17.2-slim` and `v0.17.2`:  
  
- `infiniflow/ragflow:v0.17.2-slim` (default): The RAGFlow Docker image without embedding models.  
- `infiniflow/ragflow:v0.17.2`: The RAGFlow Docker image with embedding models including:
  - Built-in embedding models:
    - `BAAI/bge-large-zh-v1.5`
    - `BAAI/bge-reranker-v2-m3`
    - `maidalun1020/bce-embedding-base_v1`
    - `maidalun1020/bce-reranker-base_v1`
  - Embedding models that will be downloaded once you select them in the RAGFlow UI:
    - `BAAI/bge-base-en-v1.5`
    - `BAAI/bge-large-en-v1.5`
    - `BAAI/bge-small-en-v1.5`
    - `BAAI/bge-small-zh-v1.5`
    - `jinaai/jina-embeddings-v2-base-en`
    - `jinaai/jina-embeddings-v2-small-en`
    - `nomic-ai/nomic-embed-text-v1.5`
    - `sentence-transformers/all-MiniLM-L6-v2`

---

### Do you offer an API for integration with third-party applications?

The corresponding APIs are now available. See the [RAGFlow HTTP API Reference](./references/http_api_reference.md) or the [RAGFlow Python API Reference](./references/python_api_reference.md) for more information.

---

### Do you support stream output?

Yes, we do.

---

### Is it possible to share dialogue through URL?

No, this feature is not supported.

---

### Do you support multiple rounds of dialogues, referencing previous dialogues as context for the current query?

Yes, we support enhancing user queries based on existing context of an ongoing conversation:

1. On the **Chat** page, hover over the desired assistant and select **Edit**.
2. In the **Chat Configuration** popup, click the **Prompt Engine** tab.
3. Switch on **Multi-turn optimization** to enable this feature.

---

## Troubleshooting

---

### Issues with Docker images

---

#### How to build the RAGFlow image from scratch?

See [Build a RAGFlow Docker image](./develop/build_docker_image.mdx).

---

### Issues with huggingface models

---

#### Cannot access https://huggingface.co

A locally deployed RAGflow downloads OCR and embedding modules from [Huggingface website](https://huggingface.co) by default. If your machine is unable to access this site, the following error occurs and PDF parsing fails:

```
FileNotFoundError: [Errno 2] No such file or directory: '/root/.cache/huggingface/hub/models--InfiniFlow--deepdoc/snapshots/be0c1e50eef6047b412d1800aa89aba4d275f997/ocr.res'
```

To fix this issue, use https://hf-mirror.com instead:

1. Stop all containers and remove all related resources:

   ```bash
   cd ragflow/docker/
   docker compose down
   ```

2. Uncomment the following line in **ragflow/docker/.env**:

   ```
   # HF_ENDPOINT=https://hf-mirror.com
   ```

3. Start up the server:

   ```bash
   docker compose up -d 
   ```

---

#### `MaxRetryError: HTTPSConnectionPool(host='hf-mirror.com', port=443)`

This error suggests that you do not have Internet access or are unable to connect to hf-mirror.com. Try the following:

1. Manually download the resource files from [huggingface.co/InfiniFlow/deepdoc](https://huggingface.co/InfiniFlow/deepdoc) to your local folder **~/deepdoc**.
2. Add a volumes to **docker-compose.yml**, for example:

   ```
   - ~/deepdoc:/ragflow/rag/res/deepdoc
   ```

---

### Issues with RAGFlow servers

---

#### `WARNING: can't find /raglof/rag/res/borker.tm`

Ignore this warning and continue. All system warnings can be ignored.

---

#### `network anomaly There is an abnormality in your network and you cannot connect to the server.`

![anomaly](https://github.com/infiniflow/ragflow/assets/93570324/beb7ad10-92e4-4a58-8886-bfb7cbd09e5d)

You will not log in to RAGFlow unless the server is fully initialized. Run `docker logs -f ragflow-server`.

*The server is successfully initialized, if your system displays the following:*

```
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

---

### Issues with RAGFlow backend services

---

#### `Realtime synonym is disabled, since no redis connection`

Ignore this warning and continue. All system warnings can be ignored.

![](https://github.com/infiniflow/ragflow/assets/93570324/ef5a6194-084a-4fe3-bdd5-1c025b40865c)

---

#### Why does my document parsing stall at under one percent?

![stall](https://github.com/infiniflow/ragflow/assets/93570324/3589cc25-c733-47d5-bbfc-fedb74a3da50)

Click the red cross beside the 'parsing status' bar, then restart the parsing process to see if the issue remains. If the issue persists and your RAGFlow is deployed locally, try the following:

1. Check the log of your RAGFlow server to see if it is running properly:

   ```bash
   docker logs -f ragflow-server
   ```

2. Check if the **task_executor.py** process exists.
3. Check if your RAGFlow server can access hf-mirror.com or huggingface.com.

---

#### Why does my pdf parsing stall near completion, while the log does not show any error?

Click the red cross beside the 'parsing status' bar, then restart the parsing process to see if the issue remains. If the issue persists and your RAGFlow is deployed locally, the parsing process is likely killed due to insufficient RAM. Try increasing your memory allocation by increasing the `MEM_LIMIT` value in **docker/.env**.

:::note
Ensure that you restart up your RAGFlow server for your changes to take effect!

```bash
docker compose stop
```

```bash
docker compose up -d
```

:::

![nearcompletion](https://github.com/infiniflow/ragflow/assets/93570324/563974c3-f8bb-4ec8-b241-adcda8929cbb)

---

#### `Index failure`

An index failure usually indicates an unavailable Elasticsearch service.

---

#### How to check the log of RAGFlow?

```bash
tail -f ragflow/docker/ragflow-logs/*.log
```

---

#### How to check the status of each component in RAGFlow?

1. Check the status of the Elasticsearch Docker container:

   ```bash
   $ docker ps
   ```

   *The following is an example result:*

   ```bash
   5bc45806b680   infiniflow/ragflow:latest     "./entrypoint.sh"        11 hours ago   Up 11 hours               0.0.0.0:80->80/tcp, :::80->80/tcp, 0.0.0.0:443->443/tcp, :::443->443/tcp, 0.0.0.0:9380->9380/tcp, :::9380->9380/tcp   ragflow-server
   91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
   d8c86f06c56b   mysql:5.7.18        "docker-entrypoint.s…"   7 days ago     Up 16 seconds (healthy)   0.0.0.0:3306->3306/tcp, :::3306->3306/tcp     ragflow-mysql
   cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
   ```

2. Follow [this document](./guides/run_health_check.md) to check the health status of the Elasticsearch service.

:::danger IMPORTANT
The status of a Docker container status does not necessarily reflect the status of the service. You may find that your services are unhealthy even when the corresponding Docker containers are up running. Possible reasons for this include network failures, incorrect port numbers, or DNS issues.
:::

---

#### `Exception: Can't connect to ES cluster`

1. Check the status of the Elasticsearch Docker container:

   ```bash
   $ docker ps
   ```

   *The status of a healthy Elasticsearch component should look as follows:*  

   ```
   91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
   ```

2. Follow [this document](./guides/run_health_check.md) to check the health status of the Elasticsearch service.

:::danger IMPORTANT
The status of a Docker container status does not necessarily reflect the status of the service. You may find that your services are unhealthy even when the corresponding Docker containers are up running. Possible reasons for this include network failures, incorrect port numbers, or DNS issues.
:::

3. If your container keeps restarting, ensure `vm.max_map_count` >= 262144 as per [this README](https://github.com/infiniflow/ragflow?tab=readme-ov-file#-start-up-the-server). Updating the `vm.max_map_count` value in **/etc/sysctl.conf** is required, if you wish to keep your change permanent. Note that this configuration works only for Linux.

---

#### Can't start ES container and get `Elasticsearch did not exit normally`

This is because you forgot to update the `vm.max_map_count` value in **/etc/sysctl.conf** and your change to this value was reset after a system reboot.

---

#### `{"data":null,"code":100,"message":"<NotFound '404: Not Found'>"}`

Your IP address or port number may be incorrect. If you are using the default configurations, enter `http://<IP_OF_YOUR_MACHINE>` (**NOT 9380, AND NO PORT NUMBER REQUIRED!**) in your browser. This should work.

---

#### `Ollama - Mistral instance running at 127.0.0.1:11434 but cannot add Ollama as model in RagFlow`

A correct Ollama IP address and port is crucial to adding models to Ollama:

- If you are on demo.ragflow.io, ensure that the server hosting Ollama has a publicly accessible IP address. Note that 127.0.0.1 is not a publicly accessible IP address.
- If you deploy RAGFlow locally, ensure that Ollama and RAGFlow are in the same LAN and can communicate with each other.

See [Deploy a local LLM](./guides/models/deploy_local_llm.mdx) for more information.

---

#### Do you offer examples of using DeepDoc to parse PDF or other files?

Yes, we do. See the Python files under the **rag/app** folder.

---

#### Why did I fail to upload a 128MB+ file to my locally deployed RAGFlow?

Ensure that you update the **MAX_CONTENT_LENGTH** environment variable:

1. In **ragflow/docker/.env**, uncomment environment variable `MAX_CONTENT_LENGTH`:

   ```
   MAX_CONTENT_LENGTH=176160768 # 168MB
   ```

2. Update **ragflow/docker/nginx/nginx.conf**:

   ```
   client_max_body_size 168M
   ```

3. Restart the RAGFlow server:

   ```
   docker compose up ragflow -d
   ```

---

#### `FileNotFoundError: [Errno 2] No such file or directory`

1. Check the status of the MinIO Docker container:

   ```bash
   $ docker ps
   ```

   *The status of a healthy Elasticsearch component should look as follows:*  

   ```bash
   cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
   ```

2. Follow [this document](./guides/run_health_check.md) to check the health status of the Elasticsearch service.

:::danger IMPORTANT
The status of a Docker container status does not necessarily reflect the status of the service. You may find that your services are unhealthy even when the corresponding Docker containers are up running. Possible reasons for this include network failures, incorrect port numbers, or DNS issues.
:::

---

## Usage

---

### How to increase the length of RAGFlow responses?

1. Right-click the desired dialog to display the **Chat Configuration** window.
2. Switch to the **Model Setting** tab and adjust the **Max Tokens** slider to get the desired length.
3. Click **OK** to confirm your change.

---

### How to run RAGFlow with a locally deployed LLM?

You can use Ollama or Xinference to deploy local LLM. See [here](./guides/models/deploy_local_llm.mdx) for more information.

---

### Is it possible to add an LLM that is not supported?

If your model is not currently supported but has APIs compatible with those of OpenAI, click **OpenAI-API-Compatible** on the **Model providers** page to configure your model:

![openai-api-compatible](https://github.com/user-attachments/assets/b1e964f2-b86e-41af-8528-fd8a96dc5f6f)

---

### How to interconnect RAGFlow with Ollama?

- If RAGFlow is locally deployed, ensure that your RAGFlow and Ollama are in the same LAN.
- If you are using our online demo, ensure that the IP address of your Ollama server is public and accessible.

See [here](./guides/models/deploy_local_llm.mdx) for more information.

---

### `Error: Range of input length should be [1, 30000]`

This error occurs because there are too many chunks matching your search criteria. Try reducing the **TopN** and increasing **Similarity threshold** to fix this issue:

1. Click **Chat** in the middle top of the page.
2. Right-click the desired conversation > **Edit** > **Prompt Engine**
3. Reduce the **TopN** and/or raise **Similarity threshold**.
4. Click **OK** to confirm your changes.

![topn](https://github.com/infiniflow/ragflow/assets/93570324/7ec72ab3-0dd2-4cff-af44-e2663b67b2fc)

---

### How to get an API key for integration with third-party applications?

See [Acquire a RAGFlow API key](./develop/acquire_ragflow_api_key.md).

---

### How to upgrade RAGFlow?

See [Upgrade RAGFlow](./guides/upgrade_ragflow.mdx) for more information.

---