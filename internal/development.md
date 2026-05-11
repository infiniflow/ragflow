# RAGFlow Go Version - Startup Guide

## 1. Start Dependencies

```bash
docker compose -f docker/docker-compose-base.yml up -d
```

## 2. Build Go Version RAGFlow
- First build (includes C++ dependencies):

```bash
./build.sh --cpp
```

- Subsequent builds (Go only):

```bash
./build.sh --go
```

## 3. Run Go Version RAGFlow
Note: admin_server must be started first; otherwise, ragflow_server will encounter errors when sending heartbeats.

```bash
# Start admin server
./bin/admin_server
```

```bash
# Start RAGFlow server
./bin/ragflow_server
```
```bash
# Run CLI
./bin/ragflow_cli
```

## 4. Start Frontend
```bash
cd web && export API_PROXY_SCHEME=hybrid && npm run dev
```

## 5. Service Ports & API Routing
- ragflow_server listens on port 9384
- admin_server listens on port 9383

After updating or implementing an API, update the frontend development environment routes in web/vite.config.ts under proxySchemes.

### Proxy Schemes

| Scheme | Description |
|--------|-------------|
| `python` | All API requests from the frontend are routed to the Python server |
| `hybrid` | API requests are partially routed to the Go server and partially to the Python server |
| `go` | All API requests from the frontend are routed to the Go server |


## 6. RAGFlow commands

You can use the following CLI commands to test the corresponding API implementations.

### 6.1. Run ragflow_cli, register user, login, and logout:

```
$ ./ragflow_cli
Welcome to RAGFlow CLI
Type \? for help, \q to quit

RAGFlow(user)> REGISTER USER 'aaa@aaa.com' AS 'aaa' PASSWORD 'aaa';
Register successfully
RAGFlow(user)> login user 'aaa@aaa.com';
password for aaa@aaa.com: Password: 
Login user aaa@aaa.com successfully
RAGFlow(user)> logout;
SUCCESS
```

### 6.2. List currently supported providers
```
RAGFlow(user)> list available providers;
```

### 6.3. Add or delete a provider for the current tenant
```
RAGFlow(user)> add provider 'openai';
```
```
RAGFlow(user)> delete provider 'openai';
```
### 6.4. Create a model instance for a specific provider
```
RAGFlow(user)> create provider 'openai' instance 'instance_name' key 'api-key';
```

Note: The api-key is a valid API key that needs to be applied for. You can create multiple instances for the same model provider, each with a different API key.

For locally deployed models (e.g., ollama, vLLM), use the following command to add a model instance:

```
RAGFlow(user)> create provider 'vllm' instance 'instance_name' key '' url 'http://192.168.1.96:8123/v1';
```
### 6.5. List and delete an instance
```
RAGFlow(user)> list instances from 'openai';
```
```
RAGFlow(user)> drop instance 'instance_name' from 'openai';
```
### 6.6. List models supported by a model instance
```
RAGFlow(user)> list models from 'openai' 'instance_name';
```
### 6.7. Chat with LLM
- Chat
```
RAGFlow(user)> chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Answer: A large language model is an AI trained on vast text data to understand, generate, and refine human-like language.
Time: 1.052269
```
- Chat with Thinking (Reasoning)
```
RAGFlow(user)> think chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Thinking: I need to create a concise 20-word introduction to LLMs...
Answer: Large Language Models are AI systems trained on vast datasets, enabling human-like text generation, comprehension, and problem-solving across diverse applications.
Time: 11.592358
```
- Streaming Chat
```
RAGFlow(user)> stream chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Answer: Language Models are advanced AI systems. They process text to learn, generate human-like responses, and perform diverse tasks through machine learning.
Time: 2.615930
```
- Streaming Chat with Thinking
```
RAGFlow(user)> stream think chat with 'glm-4.5-flash@test@zhipu-ai' message '20 words introduce LLM';
Thinking: The user is asking for a very concise introduction to LLMs...
Answer: language models are AI systems trained on vast text datasets to understand and generate human-like text for diverse tasks.
Time: 11.958035
```
- Image Understanding
```
RAGFlow(user)> chat with 'glm-4.6v-flash@test@zhipu-ai' message 'What are the pics talk about?' image 'https://cdn.bigmodel.cn/static/logo/register.png' 'https://cdn.bigmodel.cn/static/logo/api-key.png'
Answer: The first picture shows a login/register modal... The second picture displays the API keys management page...
Time: 31.600545
```
- Video Understanding
```
RAGFlow(user)> chat with 'glm-4.6v-flash@test@zhipu-ai' message 'What are the video talk about?' video 'https://cdn.bigmodel.cn/agent-demos/lark/113123.mov'
Answer: Based on the sequence of frames provided, the video is a demonstration of a web search and navigation process...
Time: 76.582520
```
Note: Both image and video understanding support streaming and thinking modes as well.

### 6.8. Generate Embeddings
```
RAGFlow(user)> embed text 'what is rag' 'who are you' with 'embedding-3@test@zhipu-ai' dimension 16;
```
### 6.9. Document Reranking
```
RAGFlow(user)> rerank query 'what is rag' document 'rag is retrieval augment generation' 'rag need llm' 'famous rag project includes ragflow' with 'rerank@test@zhipu-ai' top 2;
```

### 6.10. Get supported models from provider API

```
RAGFlow(user)> list supported models from 'minimax' 'test';
+------------------------+
| model_name             |
+------------------------+
| MiniMax-M2.7           |
| MiniMax-M2.7-highspeed |
| MiniMax-M2.5           |
| MiniMax-M2.5-highspeed |
| MiniMax-M2.1           |
| MiniMax-M2.1-highspeed |
| MiniMax-M2             |
+------------------------+
```

### 6.11. Get preset models of a provider

```
RAGFlow(user)> list models from 'minimax';
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

### 6.12. List instances of a provider

```
RAGFlow(user)> list instances from 'zhipu-ai';
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
| apiKey  | extra                | id                               | instanceName | providerID                       | status |
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
| api-key | {"region":"default"} | 19f620e73c7a11f1a51138a74640adcc | test         | d21a3758398f11f1ab4838a74640adcc | enable |
+---------+----------------------+----------------------------------+--------------+----------------------------------+--------+
```

### 6.13. Show instance of a provider
```
RAGFlow(user)> show instance 'test' from 'zhipu-ai';
+----------------------------------+--------------+----------------------------------+---------+--------+
| id                               | instanceName | providerID                       | region  | status |
+----------------------------------+--------------+----------------------------------+---------+--------+
| 19f620e73c7a11f1a51138a74640adcc | test         | d21a3758398f11f1ab4838a74640adcc | default | enable |
+----------------------------------+--------------+----------------------------------+---------+--------+
```

### 6.14. List models of a specific instance

```
RAGFlow(user)> list models from 'minimax' 'test';
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

### 6.15. List added providers
```
RAGFlow(user)> list providers;
+--------------------------------------------------------------------------+-------------+--------------+
| base_url                                                                 | name        | total_models |
+--------------------------------------------------------------------------+-------------+--------------+
| map[default:https://ark.cn-beijing.volces.com/api/v3]                    | VolcEngine  | 2            |
| map[default:https://api.minimaxi.com/ global:https://api.minimax.io/]    | MiniMax     | 8            |
| map[default:https://api.moark.com/v1]                                    | Gitee       | 5            |
+--------------------------------------------------------------------------+-------------+--------------+
```

### 6.16. Deactivate / activate a model

```
RAGFlow(user)> disable model 'deepseek-v4-pro' from 'deepseek' 'test';
SUCCESS
RAGFlow(user)> list models from 'deepseek' 'test';
+------------+-------------+-------------------+----------+
| max_tokens | model_types | name              | status   |
+------------+-------------+-------------------+----------+
| 1048576    | [chat]      | deepseek-v4-flash | active   |
| 1048576    | [chat]      | deepseek-v4-pro   | inactive |
+------------+-------------+-------------------+----------+
RAGFlow(user)> enable model 'deepseek-v4-pro' from 'deepseek' 'test';
SUCCESS
```

### 6.17. Set current model
```
RAGFlow(user)> use model 'glm-4.5-flash@test@zhipu-ai';
SUCCESS
RAGFlow(user)> chat message '20 words introduce LLM';
Answer: Large language models are advanced AI systems. They process text to understand, generate, and refine human-like language for countless tasks.
Time: 1.680416
```

### 6.18. Set, reset, and list default models
```
RAGFlow(user)> set default chat model 'zhipu-ai/test/glm-4.5-flash';
SUCCESS
RAGFlow(user)> set default vision model 'zhipu-ai/test/glm-4.5v';
SUCCESS
RAGFlow(user)> set default embedding model 'zhipu-ai/test/embedding-2';
SUCCESS
RAGFlow(user)> set default rerank model 'zhipu-ai/test/rerank';
SUCCESS
RAGFlow(user)> set default ocr model 'zhipu-ai/test/glm-ocr';
SUCCESS
RAGFlow(user)> set default tts model 'zhipu-ai/test/glm-tts';
SUCCESS
RAGFlow(user)> set default asr model 'zhipu-ai/test/glm-asr-2512';
SUCCESS
RAGFlow(user)> list default models;
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
RAGFlow(user)> reset default embedding model;
SUCCESS
RAGFlow(user)> reset default chat model
SUCCESS
RAGFlow(user)> list default models;
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

### 6.19. Show current balance of a provider instance
```
RAGFlow(user)> show balance from 'gitee' 'test';
+-------------+----------+
| balance     | currency |
+-------------+----------+
| 82.49835029 | CNY      |
+-------------+----------+
```

### 6.20. Check provider instance availability
```
RAGFlow(user)> check instance 'test' from 'zhipu-ai';
SUCCESS
```

### 6.21. Add local model to RAGFlow, only for local deployed inference server, such as ollama
```
RAGFlow(user)> add model 'Qwen/Qwen2.5-0.5B' to provider 'vllm' instance 'test' with tokens 131072 chat;
SUCCESS
RAGFlow(user)> list models from 'vllm' 'test';
+-------------------+--------+
| name              | status |
+-------------------+--------+
| Qwen/Qwen2.5-0.5B | active |
+-------------------+--------+
RAGFlow(user)> drop model 'Qwen/Qwen2.5-0.5B' from 'vllm' 'test';
SUCCESS
```

### 6.22. List datasets
```
RAGFlow(user)> list datasets;
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
| chunk_count | chunk_method | document_count | embedding_model      | id                               | language | name | nickname | permission | tenant_id                        | token_num | update_time   |
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
| 492         | naive        | 1              | embedding-2@ZHIPU-AI | e93ab2c04ad111f1b17438a74640adcc | English  | aaa  | aaa      | me         | 2ba4881420fa11f19e9c38a74640adcc | 74278     | 1778245825722 |
| 0           | naive        | 1              | embedding-2@ZHIPU-AI | 0abe79f9423311f1ad8d38a74640adcc | English  | ccc  | aaa      | me         | 2ba4881420fa11f19e9c38a74640adcc | 0         | 1777375201933 |
+-------------+--------------+----------------+----------------------+----------------------------------+----------+------+----------+------------+----------------------------------+-----------+---------------+
```
