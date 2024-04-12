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

Adjust the **Max Tokens** slider in **Model Setting**:

![](https://github.com/infiniflow/ragflow/assets/93570324/6a9c3577-6f5c-496a-9b8d-bee7f98a9c3c)


### What does Empty response mean? How to set it?

You limit what the system responds to what you specify in Empty response if nothing is retrieved from your knowledge base. If you do not specify anything in Empty response, you let your LLM improvise, giving it a chance to hallucinate.

### Can I set the base URL for OpenAI somewhere?

![](https://github.com/infiniflow/ragflow/assets/93570324/8cfb6fa4-8a97-415d-b9fa-b6f405a055f3)


### How to run RAGFlow with a locally deployed LLM?

You can use Ollama to deploy local LLM. See [here](https://github.com/infiniflow/ragflow/blob/main/docs/ollama.md) for more information. 

### How to link up ragflow and ollama servers?

- If RAGFlow is locally deployed, ensure that your RAGFlow and Ollama are in the same LAN. 
- If you are using our online demo, ensure that the IP address of your Ollama server is public and accessible.

### How to configure RAGFlow to respond with 100% matched results, rather than utilizing LLM?

In Configuration, choose **Q&A** as the chunk method:

![](https://github.com/infiniflow/ragflow/assets/93570324/b119f201-ddc2-425f-ab6d-e82fa7b7ce8c)

## Debugging

### How to handle `WARNING: can't find /raglof/rag/res/borker.tm`?

Ignore this warning and continue. All system warnings can be ignored.

### How to handle `Realtime synonym is disabled, since no redis connection`?

Ignore this warning and continue. All system warnings can be ignored.

![](https://github.com/infiniflow/ragflow/assets/93570324/ef5a6194-084a-4fe3-bdd5-1c025b40865c)

### Why does it take so long to parse a 2MB document?

Parsing requests have to wait in queue due to limited server resources. We are currently enhancing our algorithms and increasing computing power.

### How to handle `Index failure`?

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
5bc45806b680   infiniflow/ragflow:v1.0     "./entrypoint.sh"        11 hours ago   Up 11 hours               0.0.0.0:80->80/tcp, :::80->80/tcp, 0.0.0.0:443->443/tcp, :::443->443/tcp, 0.0.0.0:9380->9380/tcp, :::9380->9380/tcp   ragflow-server
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
d8c86f06c56b   mysql:5.7.18        "docker-entrypoint.s…"   7 days ago     Up 16 seconds (healthy)   0.0.0.0:3306->3306/tcp, :::3306->3306/tcp     ragflow-mysql
cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
```

### How to handle `Exception: Can't connect to ES cluster`?

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
    - - If you run RAGFlow outside of Docker, verify the ES host setting in **conf/service_conf.yml** using: 
    ```bash
    curl http://<IP_OF_ES>:<PORT_OF_ES>
    ```


### How to handle `{"data":null,"retcode":100,"retmsg":"<NotFound '404: Not Found'>"}`?

Your IP address or port number may be incorrect. If you are using the default configurations, enter http://<IP_OF_YOUR_MACHINE> (NOT `localhost`, NOT 9380, AND NO PORT NUMBER REQUIRED!) in your browser. This should work.






