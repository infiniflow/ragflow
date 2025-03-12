# DeepSearch (深度搜索) 整体流程分析

DeepSearch，或称为深度研究（Deep Research）功能，是 RAGFlow 中的代理式推理系统，它通过迭代式的思考-搜索-分析循环来解决复杂问题。以下是其整体流程的详细分析：

## 1. 初始化阶段

当用户提出一个问题时，系统首先初始化 DeepResearcher：

```python
reasoner = DeepResearcher(
    chat_mdl,                  # 大语言模型
    prompt_config,             # 配置参数
    partial(retriever.retrieval, ...) # 知识库检索函数
)
```

## 2. 核心推理循环

DeepResearcher 的 `thinking` 方法实现了一个迭代式的推理循环，最多进行 6 次（由 MAX_SEARCH_LIMIT 定义）：

### 2.1 思考阶段（Reasoning）

系统使用大语言模型生成推理和搜索查询：

```python
for ans in self.chat_mdl.chat_streamly(REASON_PROMPT, msg_history, {"temperature": 0.7}):
    query_think = ans
```

- 使用 REASON_PROMPT 提示模板引导模型思考
- 模型生成包含推理过程和搜索查询的文本
- 搜索查询被特殊标记包围：`<|begin_search_query|>查询内容<|end_search_query|>`

### 2.2 搜索阶段（Acting）

系统从多个数据源获取信息：

```python
# 1. 知识库搜索
kbinfos = self._kb_retrieve(question=search_query)

# 2. 网络搜索（如果配置了Tavily API）
if self.prompt_config.get("tavily_api_key"):
    tav = Tavily(self.prompt_config["tavily_api_key"])
    tav_res = tav.retrieve_chunks(search_query)
    kbinfos["chunks"].extend(tav_res["chunks"])
    
# 3. 知识图谱搜索（如果启用）
if self.prompt_config.get("use_kg") and self._kg_retrieve:
    ck = self._kg_retrieve(question=search_query)
    if ck["content_with_weight"]:
        kbinfos["chunks"].insert(0, ck)
```

### 2.3 分析阶段（Analyzing）

系统分析搜索结果，提取相关信息：

```python
for ans in self.chat_mdl.chat_streamly(
        RELEVANT_EXTRACTION_PROMPT.format(...),
        [{"role": "user", "content": f'Now you should analyze...'}],
        {"temperature": 0.7}):
    summary_think = ans
```

- 使用 RELEVANT_EXTRACTION_PROMPT 提示模板引导模型分析搜索结果
- 模型提取与当前查询相关的有用信息
- 分析结果被特殊标记包围：`<|begin_search_result|>分析结果<|end_search_result|>`

### 2.4 更新推理历史

系统将分析结果添加到推理历史和消息历史中：

```python
all_reasoning_steps.append(summary_think)
msg_history.append({"role": "user", "content": f"\n\n{BEGIN_SEARCH_RESULT}{summary_think}{END_SEARCH_RESULT}\n\n"})
```

### 2.5 重复循环或终止

系统根据以下条件决定是否继续循环：
- 如果达到最大搜索次数限制（6次），则终止
- 如果模型不再生成新的搜索查询，则终止
- 如果模型认为已经收集了足够的信息，则终止

## 3. 流程图示

```
用户问题
    ↓
初始化 DeepResearcher
    ↓
┌─→ 思考阶段 ───────┐
│   (生成推理和查询) │
│       ↓          │
│   搜索阶段        │
│   (多源信息检索)   │
│       ↓          │
│   分析阶段        │
│   (提取相关信息)   │
│       ↓          │
│   更新推理历史     │
│       ↓          │
│   检查终止条件 ─────┐
└───┬── 否         │ 是
    ↓              ↓
继续下一轮循环      返回完整推理过程
```

## 4. 数据流动

1. **输入**：用户问题
2. **中间数据**：
   - 推理历史（all_reasoning_steps）
   - 消息历史（msg_history）
   - 搜索查询（queries）
   - 搜索结果（kbinfos）
   - 分析结果（summary_think）
3. **输出**：完整的推理过程，包括每一步的思考、搜索和分析

## 5. 关键技术特点

1. **流式输出**：整个推理过程采用流式输出，用户可以实时看到推理过程
2. **多源信息融合**：结合知识库、网络搜索和知识图谱
3. **自主推理**：模型自主决定需要搜索什么信息
4. **防止重复搜索**：跟踪已执行的搜索查询，避免重复搜索
5. **智能截断**：维护完整的推理历史，但在传递给模型时会进行智能截断

## 6. 集成到对话服务

在 `dialog_service.py` 中，DeepResearcher 被集成到对话服务中：

```python
if prompt_config.get("reasoning", False):
    reasoner = DeepResearcher(chat_mdl,
                              prompt_config,
                              partial(retriever.retrieval, ...))

    for think in reasoner.thinking(kbinfos, " ".join(questions)):
        if isinstance(think, str):
            thought = think
            knowledges = [t for t in think.split("\n") if t]
        elif stream:
            yield think
```

这使得 DeepResearcher 可以作为对话系统的一部分，为用户提供透明的推理过程。

## 总结

DeepSearch 是一个复杂而强大的推理系统，它通过迭代式的思考-搜索-分析循环，结合多源信息，实现了透明且可解释的推理过程。这种设计使 RAGFlow 能够处理需要多步推理和信息收集的复杂问题，如多跳问答、因果分析和比较分析等任务。
