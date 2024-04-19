# Frequently Asked Questions

## General

### What sets RAGFlow apart from other RAG products?

The "garbage in garbage out" status quo remains unchanged despite the fact that LLMs have advanced Natural Language Processing (NLP) significantly. In response, RAGFlow introduces two unique features compared to other Retrieval-Augmented Generation (RAG) products.

- Fine-grained document parsing: Document parsing involves images and tables, with the flexibility for you to intervene as needed.
- Traceable answers with reduced hallucinations: You can trust RAGFlow's responses as you can view the citations and references supporting them.

### Which languages does RAGFlow support?

English, simplified Chinese, traditional Chinese for now. 

## Performance

### Why does it take longer for RAGFlow to parse a document than LangChain?

We put painstaking effort into document pre-processing tasks like layout analysis, table structure recognition, and OCR (Optical Character Recognition) using our vision model. This contributes to the additional time required. 

## Feature

### Which architectures or devices does RAGFlow support?

ARM64 and Ascend GPU are not supported.

### Do you offer an API for integration with third-party applications?

These APIs are still in development. Contributions are welcome.

### Do you support stream output?

No, this feature is still in development. Contributions are welcome. 

### Is it possible to share dialogue through URL?

This feature and the related APIs are still in development. Contributions are welcome.

### Do you support multiple rounds of dialogues, i.e., referencing previous dialogues as context for the current dialogue?

This feature and the related APIs are still in development. Contributions are welcome.

## Configurations

### How to increase the length of RAGFlow responses?

1. Right click the desired dialog to display the **Chat Configuration** window.
2. Switch to the **Model Setting** tab and adjust the **Max Tokens** slider to get the desired length.
3. Click **OK** to confirm your change. 


### What does Empty response mean? How to set it?

You limit what the system responds to what you specify in **Empty response** if nothing is retrieved from your knowledge base. If you do not specify anything in **Empty response**, you let your LLM improvise, giving it a chance to hallucinate.

### Can I set the base URL for OpenAI somewhere?

![](https://github.com/infiniflow/ragflow/assets/93570324/8cfb6fa4-8a97-415d-b9fa-b6f405a055f3)


### How to run RAGFlow with a locally deployed LLM?

You can use Ollama to deploy local LLM. See [here](https://github.com/infiniflow/ragflow/blob/main/docs/ollama.md) for more information. 

### How to link up ragflow and ollama servers?

- If RAGFlow is locally deployed, ensure that your RAGFlow and Ollama are in the same LAN. 
- If you are using our online demo, ensure that the IP address of your Ollama server is public and accessible.

### How to configure RAGFlow to respond with 100% matched results, rather than utilizing LLM?

1. Click the **Knowledge Base** tab in the middle top of the page.
2. Right click the desired knowledge base to display the **Configuration** dialogue. 
3. Choose **Q&A** as the chunk method and click **Save** to confirm your change. 

## Debugging

### `WARNING: can't find /raglof/rag/res/borker.tm`

Ignore this warning and continue. All system warnings can be ignored.

### `dependency failed to start: container ragflow-mysql is unhealthy`

`dependency failed to start: container ragflow-mysql is unhealthy` means that your MySQL container failed to start. If you are using a Mac with an M1/M2 chip, replace `mysql:5.7.18` with `mariadb:10.5.8` in **docker-compose-base.yml**.

### `Realtime synonym is disabled, since no redis connection`

Ignore this warning and continue. All system warnings can be ignored.

![](https://github.com/infiniflow/ragflow/assets/93570324/ef5a6194-084a-4fe3-bdd5-1c025b40865c)

### Why does it take so long to parse a 2MB document?

Parsing requests have to wait in queue due to limited server resources. We are currently enhancing our algorithms and increasing computing power.

### Why does my document parsing stall at under one percent?

![stall](https://github.com/infiniflow/ragflow/assets/93570324/3589cc25-c733-47d5-bbfc-fedb74a3da50)

If your RAGFlow is deployed *locally*, try the following: 

1. Check the log of your RAGFlow server to see if it is running properly:
```bash
docker logs -f ragflow-server
```
2. Check if the **tast_executor.py** process exist.
3. Check if your RAGFlow server can access hf-mirror.com or huggingface.com.

### `MaxRetryError: HTTPSConnectionPool(host='hf-mirror.com', port=443)`

This error suggests that you do not have Internet access or are unable to connect to hf-mirror.com. Try the following: 

1. Manually download the resource files from [huggingface.co/InfiniFlow/deepdoc](https://huggingface.co/InfiniFlow/deepdoc) to your local folder **~/deepdoc**. 
2. Add a volumes to **docker-compose.yml**, for example:
```
- ~/deepdoc:/ragflow/rag/res/deepdoc
```

### `Index failure`

An index failure usually indicates an unavailable Elasticsearch service.

### How to check the log of RAGFlow?

```bash
tail -f path_to_ragflow/docker/ragflow-logs/rag/*.log
```

### How to check the status of each component in RAGFlow?

```bash
$ docker ps
```
*The system displays the following if all your RAGFlow components are running properly:* 

```
5bc45806b680   infiniflow/ragflow:v0.3.0     "./entrypoint.sh"        11 hours ago   Up 11 hours               0.0.0.0:80->80/tcp, :::80->80/tcp, 0.0.0.0:443->443/tcp, :::443->443/tcp, 0.0.0.0:9380->9380/tcp, :::9380->9380/tcp   ragflow-server
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
d8c86f06c56b   mysql:5.7.18        "docker-entrypoint.s…"   7 days ago     Up 16 seconds (healthy)   0.0.0.0:3306->3306/tcp, :::3306->3306/tcp     ragflow-mysql
cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
```

### `Exception: Can't connect to ES cluster`

1. Check the status of your Elasticsearch component:

```bash
$ docker ps
```
   *The status of a 'healthy' Elasticsearch component in your RAGFlow should look as follows:*
```
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
```

2. If your container keeps restarting, ensure `vm.max_map_count` >= 262144 as per [this README](https://github.com/infiniflow/ragflow?tab=readme-ov-file#-start-up-the-server).


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


### `{"data":null,"retcode":100,"retmsg":"<NotFound '404: Not Found'>"}`

Your IP address or port number may be incorrect. If you are using the default configurations, enter http://<IP_OF_YOUR_MACHINE> (**NOT `localhost`, NOT 9380, AND NO PORT NUMBER REQUIRED!**) in your browser. This should work.

### `Ollama - Mistral instance running at 127.0.0.1:11434 but cannot add Ollama as model in RagFlow`

A correct Ollama IP address and port is crucial to adding models to Ollama:

- If you are on demo.ragflow.io, ensure that the server hosting Ollama has a publicly accessible IP address.Note that 127.0.0.1 is not a publicly accessible IP address.
- If you deploy RAGFlow locally, ensure that Ollama and RAGFlow are in the same LAN and can comunicate with each other.

### Do you offer examples of using deepdoc to parse PDF or other files?

Yes, we do. See the Python files under the **rag/app** folder. 

### Why did I fail to upload a 10MB+ file to my locally deployed RAGFlow?

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

### `Table 'rag_flow.document' doesn't exist`

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

### `hint : 102  Fail to access model  Connection error`

![hint102](https://github.com/infiniflow/ragflow/assets/93570324/6633d892-b4f8-49b5-9a0a-37a0a8fba3d2)

1. Ensure that the RAGFlow server can access the base URL.
2. Do not forget to append **/v1/** to **http://IP:port**: 
   **http://IP:port/v1/**