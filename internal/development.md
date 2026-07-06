# RAGFlow Go implementation - Development Guide

## 1. Prepare dependencies

### 1.1 Install CMake and build RAGFlow on Ubuntu 24.04

```shell
sudo apt update
sudo apt install ca-certificates gpg wget
test -f /usr/share/doc/kitware-archive-keyring/copyright || wget -O - https://apt.kitware.com/keys/kitware-archive-latest.asc 2>/dev/null | gpg --dearmor - | sudo tee /usr/share/keyrings/kitware-archive-keyring.gpg >/dev/null
echo 'deb [signed-by=/usr/share/keyrings/kitware-archive-keyring.gpg] https://apt.kitware.com/ubuntu/ noble main' | sudo tee /etc/apt/sources.list.d/kitware.list >/dev/null
sudo apt update
test -f /usr/share/doc/kitware-archive-keyring/copyright || sudo rm /usr/share/keyrings/kitware-archive-keyring.gpg
sudo apt install -y kitware-archive-keyring
sudo apt update
sudo apt install -y cmake 
```

### 1.2 Install clang-20

```shell
sudo apt install clang-20 lld-20
sudo ln -s /usr/bin/clang++-20 /usr/bin/clang++
sudo ln -s /usr/bin/clang-20 /usr/bin/clang
sudo ln -s /usr/bin/ld.lld-20 /usr/bin/ld.lld
```

### 1.3 Install golang

```shell
wget https://go.dev/dl/go1.25.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 1.4 Install dependent library
```shell
sudo apt install libpcre2-dev
python3 ragflow_deps/download_go_deps.py
```

> **Note**: If you use IDEs like GoLand to run/debug directly (via Run/Debug buttons), or run `go build` / `go run` from command line, set these CGO environment variables:
>
> ```bash
> RAGFLOW_DEPS="${HOME}/ragflow-native-libs"  # created by uv run ragflow_deps/download_deps.py
> PLATFORM="linux_amd64"  # or darwin_amd64, linux_arm64, darwin_arm64
>
> export CGO_CFLAGS="-I${RAGFLOW_DEPS}/office_oxide/include/office_oxide_c"
> export CGO_LDFLAGS="\
>     ${RAGFLOW_DEPS}/office_oxide/lib/liboffice_oxide.a \
>     ${RAGFLOW_DEPS}/pdfium-static/lib/libpdfium.a \
>     ${RAGFLOW_DEPS}/pdfium-static/lib/libc++.a \
>     ${RAGFLOW_DEPS}/pdfium-static/lib/libc++abi.a \
>     ${RAGFLOW_DEPS}/pdf_oxide/lib/${PLATFORM}/libpdf_oxide.a \
>     -fuse-ld=lld \
>     -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"
> ```
>
> All three native libraries are statically linked — no `LD_LIBRARY_PATH` or `-Wl,-rpath` needed.


### 1.5 Build RAGFlow

- Build binary
```bash
./build.sh
```

- Production builds (strip debug symbols for smaller binaries):

```bash
./build.sh --strip --all
# or
./build.sh -s --go
```

## 2. Start RAGFlow

- Start dependencies
```bash
docker compose -f docker/docker-compose-base.yml --profile ragflow-go --profile infinity up -d
```


- Start RAGFlow
Note: admin server must be started first; otherwise, api server will encounter errors when sending heartbeats.

```bash
# Start admin server
./bin/ragflow_server --admin
```

```bash
# Start RAGFlow server
./bin/ragflow_server --api
```

```bash
# Start RAGFlow ingestor
./bin/ragflow_server --ingestor
```

```bash
# Run CLI in API mode
./bin/ragflow-cli
```

```bash
# Run CLI in ADMIN mode
./bin/ragflow-cli --admin
```

## 3. Start Frontend
```bash
cd web && export API_PROXY_SCHEME=hybrid && npm run dev
```

## 4. Service Ports & API Routing
- api server listens on port 9384 by default
- admin server listens on port 9383 by default

After updating or implementing an API, update the frontend development environment routes in web/vite.config.ts under proxySchemes.

### 4.1 Proxy Schemes

| Scheme | Description |
|--------|-------------|
| `python` | All API requests from the frontend are routed to the Python server |
| `hybrid` | API requests are partially routed to the Go server and partially to the Python server |
| `go` | All API requests from the frontend are routed to the Go server |


## 5. RAGFlow commands

You can use the following CLI commands to test the corresponding API implementations.

### 5.1. Run ragflow-cli, register user, login, and logout:

```
$ ./ragflow-cli
Welcome to RAGFlow CLI
Type \? for help, \q to quit

RAGFlow(api/default)> REGISTER USER 'aaa@aaa.com' AS 'aaa' PASSWORD 'aaa';
Register successfully
RAGFlow(api/default)> login user 'aaa@aaa.com';
password for aaa@aaa.com: Password:
Login user aaa@aaa.com successfully
RAGFlow(api/default)> logout;
SUCCESS
```

### 5.2. List currently supported providers
```
RAGFlow(api/default)> list available providers;
```

### 5.3. Add or delete a provider for the current tenant
```
RAGFlow(api/default)> add provider 'openai';
```
```
RAGFlow(api/default)> delete provider 'openai';
```
### 5.4. Create a model instance for a specific provider
```
RAGFlow(api/default)> create provider 'openai' instance 'instance_name' key 'api-key';
```

Note: The api-key is a valid API key that needs to be applied for. You can create multiple instances for the same model provider, each with a different API key.

For locally deployed models (e.g., ollama, vLLM), use the following command to add a model instance:

```
RAGFlow(api/default)> create provider 'vllm' instance 'instance_name' key '' url 'http://192.168.1.96:8123/v1';
```
### 5.5. List and delete an instance
```
RAGFlow(api/default)> list instances from 'openai';
```
```
RAGFlow(api/default)> drop instance 'instance_name' from 'openai';
```
### 5.5. List models supported by a model instance
```
RAGFlow(api/default)> list models from 'openai' 'instance_name';
```
### 5.7. Chat with LLM
- Chat
```
RAGFlow(api/default)> chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Answer: A large language model is an AI trained on vast text data to understand, generate, and refine human-like language.
Time: 1.052269
```
- Chat with Thinking (Reasoning)
```
RAGFlow(api/default)> think chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Thinking: I need to create a concise 20-word introduction to LLMs...
Answer: Large Language Models are AI systems trained on vast datasets, enabling human-like text generation, comprehension, and problem-solving across diverse applications.
Time: 11.592358
```
- Streaming Chat
```
RAGFlow(api/default)> stream chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Answer: Language Models are advanced AI systems. They process text to learn, generate human-like responses, and perform diverse tasks through machine learning.
Time: 2.615930
```
- Streaming Chat with Thinking
```
RAGFlow(api/default)> stream think chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Thinking: The user is asking for a very concise introduction to LLMs...
Answer: language models are AI systems trained on vast text datasets to understand and generate human-like text for diverse tasks.
Time: 11.958035
```
- Image Understanding
```
RAGFlow(api/default)> chat with 'glm-4.6v-flash@test@zhipu-ai' message 'What are the pics talk about?' image 'https://cdn.bigmodel.cn/static/logo/register.png' 'https://cdn.bigmodel.cn/static/logo/api-key.png'
Answer: The first picture shows a login/register modal... The second picture displays the API keys management page...
Time: 31.600545
```
- Video Understanding
```
RAGFlow(api/default)> chat with 'glm-4.6v-flash@test@zhipu-ai' message 'What are the video talk about?' video 'https://cdn.bigmodel.cn/agent-demos/lark/113123.mov'
Answer: Based on the sequence of frames provided, the video is a demonstration of a web search and navigation process...
Time: 75.582520
```
Note: Both image and video understanding support streaming and thinking modes as well.

### 5.8. Chat completions

```
RAGFlow(api/default)> chat completion 'hello'
Answer: Hello! How can I assist you today? 😊
Time: 1.591929
```

```
RAGFlow(api/default)> CHAT COMPLETIONS '<question>' chat_id '<chat_id>';
```

```
RAGFlow(api/default)> CHAT COMPLETIONS 'Explain the theory' \
                      chat_id '<chat_id>' \
                      session '<session_id>' llm 'glm-4.5-flash@test@zhipu-ai' stream true;
```

```
RAGFlow(api/default)> CHAT COMPLETIONS 'Continue' \
                      system 'You are a helpful assistant.' \
                      history 'user:What is RAG?;assistant:RAG stands for Retrieval-Augmented Generation...' \
                      history_delimiter ';';
```

### 5.9. Chat with OpenAI compatible API

```
RAGFlow(api/default)> openai_chat '<chat_id>' 'Hello, how are you?';
Answer: Hello! I'm just a virtual assistant, so I don't have feelings, but I'm here and ready to help you with anything you need. How can I assist you today? 😊
Time: 8.487349
```

```
RAGFlow(api/default)> openai_chat '<chat_id>' 'Great, now what about x^3?' \
                      system 'You are a math tutor. Always explain step by step.' \
                      history 'user:What is the derivative of x^2?;assistant:The derivative of x^2 is 2x.';
```

```
RAGFlow(api/default)> openai_chat '<chat_id>' 'Hello, how are you?' temperature 0.7 max_tokens 100;
```

```
RAGFlow(api/default)> openai_chat '<chat_id>' "what's in the doc?" stream true \
                      extra_body '{"reference":true,"reference_metadata":{"include":true,"fields":["author","title"]}}';
```

```
RAGFlow(api/default)> openai_chat '7b1d58f263ca11f18121ab54cc8673a7' 'Hello' \
                      extra_body '{"metadata_condition":{"logic":"and","conditions":[{"key":"doc_type","operator":"is","value":"faq"}]}}';
```

```
RAGFlow(api/default)> openai_chat '<chat_id>' 'Hello, how are you?' temp 100;
CLI error: OPENAI_CHAT: unknown option "temp" (valid: model, system, history, delimiter, temperature, max_tokens, stream, top_p, frequency_penalty, presence_penalty, extra_body)
```

```
RAGFlow(api/default)> openai_chat '<chat_id>' 'Hello, how are you?' extra_body '{"ref":true}';
CLI error: OPENAI_CHAT extra_body: unknown field "ref" (valid: reference, reference_metadata, metadata_condition)
```

### 5.10. Generate Embeddings
```
RAGFlow(api/default)> embed text 'what is rag' 'who are you' with 'embedding-3@test@zhipu-ai' dimension 16;
```

### 5.11. Document Reranking
```
RAGFlow(api/default)> rerank query 'what is rag' document 'rag is retrieval augment generation' 'rag need llm' 'famous rag project includes ragflow' with 'rerank@test@zhipu-ai' top 2;
```

### 5.12. Get supported models from provider API

```
RAGFlow(api/default)> list supported models from 'gitee' 'test';
+-----------+---------------------------+---------------+------------+-----------------------------------------------------------------+----------------------------------------------------------+---------------------------------------------+
| dimension | dimensions                | max_dimension | max_tokens | model_types                                                     | name                                                     | thinking                                    |
+-----------+---------------------------+---------------+------------+-----------------------------------------------------------------+----------------------------------------------------------+---------------------------------------------+
|           |                           |               |            |                                                                 | bce-embedding-base_v1@maidalun1020                       |                                             |
|           |                           |               |            |                                                                 | bce-embedding-base_v1@maidalun1020                       |                                             |
|           |                           |               | 8192       | [rerank]                                                        | jina-reranker-m0@jinaai                                  |                                             |
|           |                           |               | 8192       | [rerank]                                                        | jina-reranker-m0@jinaai                                  |                                             |
|           | [64 128 256 512 768]      |               | 8192       | [embedding vision]                                              | jina-clip-v1@jinaai                                      |                                             |
|           | [64 128 256 512 768]      |               | 8192       | [embedding vision]                                              | jina-clip-v1@jinaai                                      |                                             |
|           |                           |               | 32768      | [chat]                                                          | Qwen2.5-Coder-14B-Instruct@Qwen                          |                                             |
|           |                           |               | 32768      | [chat]                                                          | Qwen2.5-Coder-14B-Instruct@Qwen                          |                                             |
|           | [64 128 256 512 768 1024] |               | 8192       | [embedding vision]                                              | jina-clip-v2@jinaai                                      |                                             |
|           |                           |               | 262144     | [chat image2text vision video_understanding]                    | Qwen3.6-27B@Qwen                                         | map[clear_thinking:true default_value:true] |
|           |                           |               | 262144     | [chat image2text vision video_understanding]                    | Qwen3.6-27B@Qwen                                         | map[clear_thinking:true default_value:true] |
|           |                           |               | 32768      | [rerank]                                                        | Qwen3-Reranker-0.6B@Qwen                                 |                                             |
+-----------+---------------------------+---------------+------------+-----------------------------------------------------------------+----------------------------------------------------------+---------------------------------------------+
```

### 5.13. Get preset models of a provider

```
RAGFlow(api/default)> list models from 'minimax';
+------------+-------------+------------------------+
| max_tokens | model_types | name                   |
+------------+-------------+------------------------+
| 204800     | [chat]      | minimax-m2.7           |
| 204800     | [chat]      | minimax-m2.7-highspeed |
| 204800     | [chat]      | minimax-m2.5           |
| 204800     | [chat]      | minimax-m2.5-highspeed |
| 204800     | [chat]      | minimax-m2.1           |
| 204800     | [chat]      | minimax-m2.1-highspeed |
| 204800     | [chat]      | minimax-m2             |
| 65536      | [chat]      | minimax-m2-her         |
+------------+-------------+------------------------+
```

### 5.14. List instances of a provider

```
RAGFlow(api/default)> list instances from 'zhipu-ai';
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
| apiKey  | extra                | id                               | instanceName | providerID                       | status |
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
| api-key | {"region":"default"} | 19f620e73c7a11f1a51138a74640adcc | test         | d21a3758398f11f1ab4838a74640adcc | enable |
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
```

### 5.15. Show instance of a provider
```
RAGFlow(api/default)> show instance 'test' from 'zhipu-ai';
+----------------------------------+--------------+----------------------------------+---------+--------+
| id                               | instanceName | providerID                       | region  | status |
+----------------------------------+--------------+----------------------------------+---------+--------+
| 19f620e73c7a11f1a51138a74640adcc | test         | d21a3758398f11f1ab4838a74640adcc | default | enable |
+----------------------------------+--------------+----------------------------------+---------+--------+
```

### 5.15. List models of a specific instance

```
RAGFlow(api/default)> list models from 'minimax' 'test';
+------------+-------------+------------------------+--------+
| max_tokens | model_types | name                   | status |
+------------+-------------+------------------------+--------+
| 204800     | [chat]      | minimax-m2.7           | active |
| 204800     | [chat]      | minimax-m2.7-highspeed | active |
| 204800     | [chat]      | minimax-m2.5           | active |
| 204800     | [chat]      | minimax-m2.5-highspeed | active |
| 204800     | [chat]      | minimax-m2.1           | active |
| 204800     | [chat]      | minimax-m2.1-highspeed | active |
| 204800     | [chat]      | minimax-m2             | active |
| 65536      | [chat]      | minimax-m2-her         | active |
+------------+-------------+------------------------+--------+
```

### 5.17. List added providers
```
RAGFlow(api/default)> list providers;
+--------------------------------------------------------------------------+-------------+--------------+
| base_url                                                                 | name        | total_models |
+--------------------------------------------------------------------------+-------------+--------------+
| map[default:https://ark.cn-beijing.volces.com/api/v3]                    | VolcEngine  | 2            |
| map[default:https://api.minimaxi.com/ global:https://api.minimax.io/]    | MiniMax     | 8            |
| map[default:https://api.moark.com/v1]                                    | Gitee       | 5            |
+--------------------------------------------------------------------------+-------------+--------------+
```

### 5.18. Deactivate / activate a model

```
RAGFlow(api/default)> disable model 'deepseek-v4-pro' from 'deepseek' 'test';
SUCCESS
RAGFlow(api/default)> list models from 'deepseek' 'test';
+------------+-------------+-------------------+----------+
| max_tokens | model_types | name              | status   |
+------------+-------------+-------------------+----------+
| 1048576    | [chat]      | deepseek-v4-flash | active   |
| 1048576    | [chat]      | deepseek-v4-pro   | inactive |
+------------+-------------+-------------------+----------+
RAGFlow(api/default)> enable model 'deepseek-v4-pro' from 'deepseek' 'test';
SUCCESS
```

### 5.19. Set current model
```
RAGFlow(api/default)> use model 'glm-4.5-flash@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> chat message '20 words introduce LLM';
Answer: Large language models are advanced AI systems. They process text to understand, generate, and refine human-like language for countless tasks.
Time: 1.680416
```

### 5.20. Set, reset, and list default models
```
RAGFlow(api/default)> set default chat model 'glm-4.5-flash@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default vision model 'glm-4.5v@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default embedding model 'embedding-2@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default rerank model 'rerank@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default ocr model 'glm-ocr@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default tts model 'tts@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> set default asr model 'glm-asr-2512@test@zhipu-ai';
SUCCESS
RAGFlow(api/default)> list default models;
+--------+----------------+---------------+----------------+------------+
| enable | model_instance | model_name    | model_provider | model_type |
+--------+----------------+---------------+----------------+------------+
| true   | test           | glm-4.5-flash | zhipu-ai       | chat       |
| true   | test           | embedding-2   | zhipu-ai       | embedding  |
| true   | test           | rerank        | zhipu-ai       | rerank     |
| true   | test           | glm-asr-2512  | zhipu-ai       | asr        |
| true   | test           | glm-4.5v      | zhipu-ai       | vision     |
| true   | test           | glm-ocr       | zhipu-ai       | ocr        |
| true   | test           | glm-tts       | zhipu-ai       | tts        |
+--------+----------------+---------------+----------------+------------+
RAGFlow(api/default)> reset default embedding model;
SUCCESS
RAGFlow(api/default)> reset default chat model;
SUCCESS
RAGFlow(api/default)> list default models;
+--------+----------------+--------------+----------------+------------+
| enable | model_instance | model_name   | model_provider | model_type |
+--------+----------------+--------------+----------------+------------+
| true   | test           | rerank       | zhipu-ai       | rerank     |
| true   | test           | glm-asr-2512 | zhipu-ai       | asr        |
| true   | test           | glm-4.5v     | zhipu-ai       | vision     |
| true   | test           | glm-ocr      | zhipu-ai       | ocr        |
| true   | test           | glm-tts      | zhipu-ai       | tts        |
+--------+----------------+--------------+----------------+------------+
```

### 5.21. Show current balance of a provider instance
```
RAGFlow(api/default)> show balance from 'gitee' 'test';
+-------------+----------+
| balance     | currency |
+-------------+----------+
| 82.49835029 | CNY      |
+-------------+----------+
```

### 5.22. Check provider instance availability
```
RAGFlow(api/default)> check instance 'test' from 'zhipu-ai';
SUCCESS
```

### 5.23. Add local model to RAGFlow, only for local deployed inference server, such as ollama
```
RAGFlow(api/default)> add model 'Qwen/Qwen2.5-0.5B' to provider 'vllm' instance 'test' with tokens 131072 chat;
SUCCESS
RAGFlow(api/default)> list models from 'vllm' 'test';
+-------------------+--------+
| name              | status |
+-------------------+--------+
| Qwen/Qwen2.5-0.5B | active |
+-------------------+--------+
RAGFlow(api/default)> drop model 'Qwen/Qwen2.5-0.5B' from 'vllm' 'test';
SUCCESS
```

### 5.24. List datasets
```
RAGFlow(api/default)> list datasets;
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
| chunk_count | chunk_method | document_count | embedding_model      | id                               | language | name | nickname | permission | tenant_id                        | token_num | update_time   |
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
| 492         | naive        | 1              | embedding-2@ZHIPU-AI | e93ab2c04ad111f1b17438a74640adcc | English  | aaa  | aaa      | me         | 2ba4881420fa11f19e9c38a74640adcc | 74278     | 1778245825722 |
| 0           | naive        | 1              | embedding-2@ZHIPU-AI | 0abe79f9423311f1ad8d38a74640adcc | English  | ccc  | aaa      | me         | 2ba4881420fa11f19e9c38a74640adcc | 0         | 1777375201933 |
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
```

### 5.25. Text to Speech
```
RAGFlow(api/default)> tts with 'speech-2.8-hd@test@minimax' text 'He who desires but acts not, breeds pestilence.' play format 'wav' save './internal' param '{"voice_setting": {"voice_id": "English_radiant_girl", "speed": 1, "vol": 1, "pitch": 0}, "audio_setting": {"sample_rate": 32000, "bitrate": 128000, "format": "wav", "channel": 1}, "output_format": "hex"}'
Saved to directory: /home/infiniflow/Documents/development/ragflow/internal/speech-2.8-hd_output.wav
SUCCESS
```

### 5.25. Audio to Speech
```
RAGFlow(api/default)> asr with 'FunAudioLLM/SenseVoiceSmall@test@siliconflow' audio './internal/test.wav' param ''
+----------------------------------------------------------------------------------------------------------------------+
| text                                                                                                                 |
+----------------------------------------------------------------------------------------------------------------------+
| The examination and testimony of the experts enabled the commission to conclude that five shots may have been fired. |
+----------------------------------------------------------------------------------------------------------------------+
```

### 5.27. Optical Character Recognition
```
RAGFlow(api/default)> ocr with 'paddleocr-vl-0.9b@test@baidu' file './internal/text.jpg'
+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| text                                                                                                                                                                                                                                                             |
+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| Parallel to these organizational innovations there were significant complementary technical innovations (e.g., improved methods of manufacturing cast-iron pipe and of coating interiors for pressure maintenance, and newer paving and construction material... |
+------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
```

### 5.28. Chunk Management Commands

- Create a chunk store with vector size
```
RAGFlow(api/default)> CREATE CHUNK STORE FOR DATASET 'test' VECTOR SIZE 384
```

- Insert data from JSON files
```
RAGFlow(api/default)> INSERT CHUNKS FROM FILE 'insert_kb.json'
```

- Update a chunk's content
```
RAGFlow(api/default)> UPDATE CHUNK 'deb165dc6a732a64' OF DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' IN DATASET 'test' SET '{"content": "Updated chunk content here", "important_keywords": ["keyword1", "keyword2"], "questions": ["What is this about?", "Why is it important?"], "available": true, "tag_kwd": ["tag5", "tag2"]}'
```

- Remove tags from a dataset
```
RAGFlow(api/default)> REMOVE TAGS 'tag1', 'tag2' FROM DATASET 'test'
```

- Remove specific chunks from a document
```
RAGFlow(api/default)> REMOVE CHUNKS '29cc4f6d7a5c6e7c' '0360e3d8519eab12' FROM DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' IN DATASET 'test'
```

- Remove all chunks from a document
```
RAGFlow(api/default)> REMOVE ALL CHUNKS FROM DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' IN DATASET 'test'
```

- Drop chunk store
```
RAGFlow(api/default)> DROP CHUNK STORE FOR DATASET 'test'
```

- Search chunks
```
RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test'
```

- Get chunks
```
RAGFlow(api/default)> GET CHUNK '29cc4f6d7a5c6e7c' OF DATASET 'test' DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' IN DATASET 'test'
```

### 5.29. Metadata Management Commands

- Create metadata store
```
RAGFlow(api/default)> CREATE METADATA STORE
```

- Insert metadata from JSON files
```
RAGFlow(api/default)> INSERT METADATA FROM FILE 'insert_metadata.json'
```
- Set metadata for a document
```
RAGFlow(api/default)> SET METADATA OF DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' TO '{"author": ["John", "Tom"], "category": "tech"}';
```

- Delete metadata of a document
```
RAGFlow(api/default)> DELETE METADATA OF DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3'
```

- Delete metadata keys of a document
```
RAGFlow(api/default)> DELETE METADATA OF DOCUMENT 'bbe55942535e11f1bc5184ba59049aa3' KEYS '["key1", "key2"]'
```

- Drop metadata store
```
RAGFlow(api/default)> DROP METADATA STORE
```

- Get metadata
```
RAGFlow(api/default)> GET METADATA OF DATASET 'test' 'test2'
```

### 5.30. Search datasets

- Search datasets using SQL-like dataset search syntax:
```
RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test1, test2';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH top_k 1;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH page 2 page_size 20;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH similarity_threshold 0.5;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH vector_similarity_weight 0.0;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH keyword true;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH use_kg true;

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH rerank_id 'BAAI/bge-reranker-v2-m3@CI@SILICONFLOW';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH search_id 'abc123';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH cross_languages ['Chinese'];

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH doc_ids ['doc_a', 'doc_b'];

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH meta_data_filter '{"method":"auto"}';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH meta_data_filter '{"method":"manual","conditions":[{"key":"author","op":"eq","value":"Luo"}]}';

RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH top_k 50 similarity_threshold 0.5 vector_similarity_weight 0.5 use_kg true;
```

- Search datasets using filesystem-style search syntax:
```
RAGFlow(api/default)> search "AI"                             # search all datasets

RAGFlow(api/default)> search "AI" datasets/test               # search only dataset 'test'

RAGFlow(api/default)> search "AI" datasets/test -n 20         # return top 20 results

RAGFlow(api/default)> search "AI" datasets 'test1' 'test2'    # search in datasets
```

> [!Note]
>  - `search` is the simple filesystem search command and only accepts `query [path] [-n number]`.
>  - `RETRIEVE` / `SEARCH ... ON DATASETS ...` is the SQL-like search command and supports full `WITH` option expansion.
>  - `WITH` options include: `top_k`, `page_size`, `page`, `similarity_threshold`, `vector_similarity_weight`, `keyword`, `use_kg`, `rerank_id`, `search_id`, `cross_languages`, `doc_ids`, and `meta_data_filter`.
  - Example with multiple options:
```
RAGFlow(api/default)> RETRIEVE 'AI' ON DATASETS 'test' WITH top_k 50 similarity_threshold 0.5 vector_similarity_weight 0.5 use_kg true;
```
