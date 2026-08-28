# Netstars-KB 对外 API

> 作者：张彦龙

外部系统（访谈助手、其他 Agent、业务系统）只走这三条通道，不直连 MySQL / ES：

1. HTTP API `http://<host>:9380`（`SVR_HTTP_PORT`）
2. Python SDK `ragflow_sdk`
3. 可选 MCP `http://<host>:9382`（需在 compose 里启用）

鉴权：系统设置里生成 API Key，请求头 `Authorization: Bearer <API Key>`。

以下 `{address}` 默认 `localhost:9380`。

## 1. 建知识库（dataset）

```bash
curl -sS -X POST "http://{address}/api/v1/datasets" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"name":"netstars-kb-demo","chunk_method":"naive"}'
```

法规/制度类文档可把 `chunk_method` 换成 `laws` 或 `paper`。

## 2. 上传并解析文档

```bash
curl -sS -X POST "http://{address}/api/v1/datasets/{dataset_id}/documents" \
  -H "Authorization: Bearer <API_KEY>" \
  -F "file=@./demo.pdf"
```

上传后需触发解析（以当前版本文档的 parse 接口为准，在 UI 里点「解析」也可）。解析完成前不要检索。

## 3. 检索（给其他助手用）

```bash
curl -sS -X POST "http://{address}/api/v1/retrieval" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "question": "报送口径怎么定义",
    "dataset_ids": ["DATASET_ID"],
    "page_size": 8,
    "similarity_threshold": 0.2,
    "vector_similarity_weight": 0.3
  }'
```

返回的 chunk 文本和引用即可拼进下游 LLM 的 context。访谈助手应把 `dataset_ids` 配成业务知识库，而不是全库乱搜。

## 4. 对话（带知识库的 Chat Assistant）

先在 UI 里建一个 Chat，绑上 dataset，记下 `chat_id`：

```bash
curl -sS -X POST "http://{address}/api/v1/chats/{chat_id}/completions" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "question": "这份制度里对报送时点怎么规定",
    "stream": false
  }'
```

OpenAI 兼容形态（若当前版本已开）：

```bash
curl -sS -X POST "http://{address}/api/v1/chats_openai/{chat_id}/chat/completions" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "model",
    "messages": [{"role":"user","content":"报送时点是什么"}],
    "stream": false
  }'
```

具体路径以运行中的版本为准：打开 `http://{address}/doc/` 或官方 [HTTP API](https://ragflow.io/docs/http_api_reference)。

## 5. Python SDK

```python
from ragflow_sdk import RAGFlow

rag = RAGFlow(api_key="<API_KEY>", base_url="http://localhost:9380")
ds = rag.list_datasets(name="netstars-kb-demo")[0]
chunks = rag.retrieve(question="报送口径怎么定义", dataset_ids=[ds.id])
for c in chunks:
    print(c)
```

```bash
pip install ragflow-sdk
```

## 6. 给访谈助手的约定

- 只读检索用第 3 节；需要带引用的完整回答用第 4 节。
- 每个监管主题一个 dataset（或一个 Chat 绑一组 dataset），不要所有文件丢进同一个库。
- Embedding 一经选定不要中途更换（见 [MODELS.md](MODELS.md)）。
- API Key 放运行环境，不进 git。
