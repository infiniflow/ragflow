# Netstars-KB 模型接入

> 作者：张彦龙

RAGFlow / Netstars-KB 的大模型和向量模型**不写在 docker-compose 密钥里**，而是在启动后的管理界面「模型提供商」里配置。`overlay/.env.example` 里的 `KB_*` 变量只作部署记录，compose 不会读取它们。

最低硬件见 [docs/SECONDARY.md](../docs/SECONDARY.md)：4 核 / 16 GB / 50 GB。镜像基线 `infiniflow/ragflow:v0.26.4`。

## 1. 先把服务拉起来

```bash
cp overlay/.env.example docker/.env
# 按需改密码，不要用默认 infini_rag_flow 上生产
cd docker
docker compose up -d
```

浏览器打开 `http://localhost`（`SVR_WEB_HTTP_PORT`，默认 80）。注册管理员账号后进入 **系统设置 → 模型提供商**。

国内拉 Hugging Face 失败时，在 `docker/.env` 取消注释：

```bash
HF_ENDPOINT=https://hf-mirror.com
```

国内拉镜像可把 `RAGFLOW_IMAGE` 改成：

```bash
RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:v0.26.4
```

## 2. 通义千问（DashScope）

适合没有本机 GPU、想先跑通中文问答的场景。

1. 模型提供商里选 **Tongyi-Qianwen**。
2. 填入 DashScope API Key（只放在界面里，不要提交进 git）。
3. 添加对话模型，例如 `qwen-plus` 或 `qwen-max`。
4. 添加 Embedding，例如 `text-embedding-v3`（或控制台里当前可用的中文向量模型）。
5. 设为系统默认：对话模型 + Embedding。

## 3. Ollama（本地）

适合内网、不想把文档打到公网的场景。宿主机先装好 Ollama 并拉好模型，例如 `qwen2.5:7b`、`bge-m3`。

1. 模型提供商里选 **Ollama**。
2. Base URL 填 `http://host.docker.internal:11434`（Linux 若不通，改成宿主机局域网 IP）。
3. 对话模型名与 `ollama list` 一致，例如 `qwen2.5:7b`。
4. Embedding 用 `bge-m3` 或 `bge-large-zh-v1.5`（需已 pull）。
5. 设为默认。

## 4. OpenAI 兼容网关

自建 vLLM、OneAPI、硅基流动等，只要提供 `/v1/chat/completions`。

1. 选 **OpenAI-API-Compatible**。
2. Base URL 填网关地址（含 `/v1`）。
3. API Key、模型名按网关文档填写。
4. Embedding 若网关也兼容 OpenAI embeddings，一并加上；否则 Embedding 仍用 DashScope 或 Ollama。

## 5. 默认 Embedding 建议

中文知识库优先：

- 云端：DashScope `text-embedding-v3`
- 本地：`BAAI/bge-large-zh-v1.5` 或 Ollama `bge-m3`

一个知识库只能用一种 Embedding。改模型等于重解析，先定再灌文档。

## 6. 验证

系统设置里能看到刚加的模型为默认。建一个测试知识库，上传一份短 PDF 或 Markdown，解析完成后在聊天助手里问一句能从文档里找到的话。API 走 `http://localhost:9380`，Header `Authorization: Bearer <API Key>`（API Key 在系统设置里生成）。
