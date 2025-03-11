# KGSearch 类的 retrieval 方法详细分析

`KGSearch` 类的 `retrieval` 方法是 RAGFlow 中知识图谱检索的核心实现，它通过一系列复杂的步骤，将自然语言查询转换为知识图谱检索结果。以下是对该方法的详细分析：

## 1. 方法签名与参数

```python
def retrieval(self, question: str,
           tenant_ids: str | list[str],
           kb_ids: list[str],
           emb_mdl,
           llm,
           max_token: int = 8196,
           ent_topn: int = 6,
           rel_topn: int = 6,
           comm_topn: int = 1,
           ent_sim_threshold: float = 0.3,
           rel_sim_threshold: float = 0.3,
           ):
```

**参数说明**：
- `question`: 用户的自然语言查询
- `tenant_ids`: 租户ID，可以是单个ID或ID列表
- `kb_ids`: 知识库ID列表
- `emb_mdl`: 嵌入模型，用于向量化文本
- `llm`: 大语言模型，用于查询分析和重写
- `max_token`: 最大令牌数，限制返回结果的大小
- `ent_topn`: 返回的实体数量上限
- `rel_topn`: 返回的关系数量上限
- `comm_topn`: 返回的社区报告数量上限
- `ent_sim_threshold`: 实体相似度阈值，低于此值的实体将被过滤
- `rel_sim_threshold`: 关系相似度阈值，低于此值的关系将被过滤

## 2. 核心流程

### 2.1 查询分析与重写

```python
ty_kwds, ents = self.query_rewrite(llm, qst, [index_name(tid) for tid in tenant_ids], kb_ids)
```

这一步使用大语言模型分析查询，提取两类关键信息：
1. `ty_kwds`: 答案类型关键词，表示查询期望的答案类型
2. `ents`: 从查询中提取的实体

`query_rewrite` 方法使用了一个特殊的提示模板 `PROMPTS["minirag_query2kwd"]`，该模板引导大语言模型从预定义的类型池中选择适当的答案类型，并从查询中提取实体。

在 `PROMPTS["minirag_query2kwd"]` 模板中，预定义的类型池（Answer type pool）是一个动态传入的参数，而不是在模板中固定的值。这个类型池是通过 `{TYPE_POOL}` 占位符在运行时传入的。

从代码实现来看，这个类型池是在 `KGSearch` 类的 `query_rewrite` 方法中生成并传入的：

```python
def query_rewrite(self, llm, question, idxnms, kb_ids):
    ty2ents = get_entity_type2sampels(idxnms, kb_ids)
    hint_prompt = PROMPTS["minirag_query2kwd"].format(query=question,
                                                      TYPE_POOL=json.dumps(ty2ents, ensure_ascii=False, indent=2))
    # ...
```

这里的 `ty2ents` 是通过 `get_entity_type2sampels` 函数获取的，它返回一个字典，其中：
- 键（key）是实体类型，如 'PERSON'、'ORGANIZATION'、'LOCATION' 等
- 值（value）是该类型的示例实体列表

`get_entity_type2sampels` 函数的实现会从知识图谱中提取不同类型的实体样本，具体来说：

1. 它会查询知识库中已有的实体
2. 按照实体类型（entity_type_kwd）对这些实体进行分组
3. 从每种类型中选取一些代表性样本
4. 返回一个类型到样本的映射字典

这个类型池的目的是为大语言模型提供一个"菜单"，让它知道在当前知识图谱中有哪些实体类型可供选择。模型会根据用户查询的内容，从这个类型池中选择最相关的类型作为 "answer_type_keywords"。

例如，如果用户查询是"谁发明了电灯泡？"，模型可能会从类型池中选择 "PERSON" 作为答案类型，因为答案应该是一个人。

在模板中的示例中，我们可以看到类型池的一些例子：

```
'PERSONAL LIFE': ['FAMILY TIME', 'HOME MAINTENANCE'],
'STRATEGY': ['MARKETING PLAN', 'BUSINESS EXPANSION'],
'SERVICE FACILITATION': ['ONLINE SUPPORT', 'CUSTOMER SERVICE TRAINING'],
'PERSON': ['JANE DOE', 'JOHN SMITH'],
'FOOD': ['PASTA', 'SUSHI'],
...
```

这些示例展示了类型池的结构，但实际运行时，类型池的内容是根据当前知识库中的实际实体动态生成的，因此会随着知识库的变化而变化。这种设计使得系统能够适应不同领域的知识图谱，提供更加精确的查询理解和实体检索。

让我查看 `get_entity_type2sampels` 函数的具体实现，以确定它是如何获取类型和实体字典的。

Ran tool

Read file: graphrag/utils.py

Ran tool

Read file: graphrag/utils.py
根据代码分析，我可以详细解释 `get_entity_type2sampels` 函数及其相关的类型池机制：

## get_entity_type2sampels 函数分析

`get_entity_type2sampels` 函数**不是**获取知识图谱中全量的类型和实体，而是获取预先存储的类型样本字典。这个函数的工作方式如下：

```python
def get_entity_type2sampels(idxnms, kb_ids: list):
    # 从索引中查询标记为 "ty2ents" 的特殊文档
    es_res = settings.retrievaler.search({"knowledge_graph_kwd": "ty2ents", "kb_id": kb_ids,
                                       "size": 10000,
                                       "fields": ["content_with_weight"]},
                                      idxnms, kb_ids)

    res = defaultdict(list)
    for id in es_res.ids:
        smp = es_res.field[id].get("content_with_weight")
        if not smp:
            continue
        try:
            # 解析存储的JSON字符串
            smp = json.loads(smp)
        except Exception as e:
            logging.exception(e)

        # 合并不同文档中的类型样本
        for ty, ents in smp.items():
            res[ty].extend(ents)
    return res
```

这个函数从向量数据库中查询特殊标记为 `"knowledge_graph_kwd": "ty2ents"` 的文档，这些文档包含了预先计算和存储的类型到实体样本的映射。

## 类型池的生成和存储

类型池不是实时生成的，而是在知识图谱构建过程中预先计算并存储的。从代码中可以看到类型池的生成过程：

```python
ty2ents = defaultdict(list)
# 按PageRank值排序实体，选择重要的实体作为样本
for p, r in sorted(pr.items(), key=lambda x: x[1], reverse=True):
    ty = graph.nodes[p].get("entity_type")
    # 如果没有类型或者该类型的样本已经足够，则跳过
    if not ty or len(ty2ents[ty]) > 12:
        continue
    ty2ents[ty].append(p)

# 将类型样本字典存储为特殊文档
chunk = {
    "content_with_weight": json.dumps(ty2ents, ensure_ascii=False),
    "kb_id": kb_id,
    "knowledge_graph_kwd": "ty2ents",
    "available_int": 0
}
```

这段代码的关键点是：

1. 它使用 PageRank 算法计算图中每个实体的重要性
2. 按重要性排序实体，为每种类型选择最多12个重要的实体作为样本
3. 将这个类型到样本的映射存储为一个特殊的文档，标记为 `"knowledge_graph_kwd": "ty2ents"`

## 类型池的特点

基于以上分析，类型池有以下特点：

1. **预计算性**：类型池不是在查询时实时生成的，而是在知识图谱构建时预先计算并存储的
2. **代表性**：类型池中的实体样本是按 PageRank 值选择的，代表了知识图谱中每种类型的重要实体
3. **有限性**：每种类型最多选择12个样本，避免类型池过大
4. **持久性**：类型池作为特殊文档存储在向量数据库中，可以被多次查询使用

## 类型池的用途

在查询处理过程中，类型池的主要用途是帮助大语言模型理解当前知识图谱中存在哪些实体类型，以便它能够：

1. 从用户查询中识别出可能的答案类型
2. 将这些类型与知识图谱中已有的类型对应起来
3. 指导后续的实体检索过程，优先检索与答案类型相关的实体

总结来说，`get_entity_type2sampels` 函数不是获取知识图谱中全量的类型和实体，而是获取一个预先计算和存储的、包含每种类型代表性实体样本的字典。这个类型池是知识图谱查询理解和重写过程中的重要组成部分，帮助系统更准确地理解用户意图并在知识图谱中找到相关信息。


### 2.2 多维度实体检索

```python
# 通过关键词检索实体
ents_from_query = self.get_relevant_ents_by_keywords(ents, filters, idxnms, kb_ids, emb_mdl, ent_sim_threshold)

# 通过类型检索实体
ents_from_types = self.get_relevant_ents_by_types(ty_kwds, filters, idxnms, kb_ids, 10000)

# 通过文本检索关系
rels_from_txt = self.get_relevant_relations_by_txt(qst, filters, idxnms, kb_ids, emb_mdl, rel_sim_threshold)
```

这三步分别从不同维度检索知识图谱中的信息：
1. 从查询中提取的实体关键词检索相关实体
2. 从答案类型关键词检索相关实体
3. 从整个查询文本检索相关关系

### 2.3 N跳路径分析

```python
nhop_pathes = defaultdict(dict)
for _, ent in ents_from_query.items():
    nhops = ent.get("n_hop_ents", [])
    if not isinstance(nhops, list):
        logging.warning(f"Abnormal n_hop_ents: {nhops}")
        continue
    for nbr in nhops:
        path = nbr["path"]
        wts = nbr["weights"]
        for i in range(len(path) - 1):
            f, t = path[i], path[i + 1]
            if (f, t) in nhop_pathes:
                nhop_pathes[(f, t)]["sim"] += ent["sim"] / (2 + i)
            else:
                nhop_pathes[(f, t)]["sim"] = ent["sim"] / (2 + i)
            nhop_pathes[(f, t)]["pagerank"] = wts[i]
```

这一步分析从检索到的实体出发，探索图中的N跳路径：
1. 对于每个检索到的实体，获取其N跳邻居信息
2. 对于路径中的每一对相邻实体，计算其相似度和PageRank值
3. 相似度随着跳数增加而衰减，体现了"距离越远，相关性越低"的原则

### 2.4 实体和关系排序优化

```python
# 增强同时出现在查询实体和类型实体中的实体权重
for ent in ents_from_types.keys():
    if ent not in ents_from_query:
        continue
    ents_from_query[ent]["sim"] *= 2

# 增强与类型实体相关的关系权重
for (f, t) in rels_from_txt.keys():
    pair = tuple(sorted([f, t]))
    s = 0
    if pair in nhop_pathes:
        s += nhop_pathes[pair]["sim"]
        del nhop_pathes[pair]
    if f in ents_from_types:
        s += 1
    if t in ents_from_types:
        s += 1
    rels_from_txt[(f, t)]["sim"] *= s + 1
```

这一步对检索到的实体和关系进行排序优化：
1. 如果一个实体同时出现在查询实体和类型实体中，其相似度权重翻倍
2. 对于关系，根据其端点实体是否出现在类型实体中，以及是否出现在N跳路径中，增强其相似度权重

### 2.5 结果排序与截断

```python
# 按相似度*PageRank排序并截断
ents_from_query = sorted(ents_from_query.items(), key=lambda x: x[1]["sim"] * x[1]["pagerank"], reverse=True)[:ent_topn]
rels_from_txt = sorted(rels_from_txt.items(), key=lambda x: x[1]["sim"] * x[1]["pagerank"], reverse=True)[:rel_topn]
```

这一步对实体和关系按照相似度与PageRank的乘积进行排序，并截取前N个结果：
1. 相似度表示与查询的相关性
2. PageRank表示在图中的重要性
3. 两者的乘积综合考虑了相关性和重要性

### 2.6 结果格式化

```python
ents = []
relas = []

# 格式化实体信息
for n, ent in ents_from_query:
    ents.append({
        "Entity": n,
        "Score": "%.2f" % (ent["sim"] * ent["pagerank"]),
        "Description": json.loads(ent["description"]).get("description", "") if ent["description"] else ""
    })
    # 控制令牌数
    max_token -= num_tokens_from_string(str(ents[-1]))
    if max_token <= 0:
        ents = ents[:-1]
        break

# 格式化关系信息
for (f, t), rel in rels_from_txt:
    # 获取关系描述
    if not rel.get("description"):
        for tid in tenant_ids:
            rela = get_relation(tid, kb_ids, f, t)
            if rela:
                break
        else:
            continue
        rel["description"] = rela["description"]
    
    # 格式化描述
    desc = rel["description"]
    try:
        desc = json.loads(desc).get("description", "")
    except Exception:
        pass
    
    relas.append({
        "From Entity": f,
        "To Entity": t,
        "Score": "%.2f" % (rel["sim"] * rel["pagerank"]),
        "Description": desc
    })
    
    # 控制令牌数
    max_token -= num_tokens_from_string(str(relas[-1]))
    if max_token <= 0:
        relas = relas[:-1]
        break
```

这一步将排序后的实体和关系转换为结构化格式，并控制总令牌数不超过限制：
1. 对于每个实体，提取其名称、得分和描述
2. 对于每个关系，提取其源实体、目标实体、得分和描述
3. 实时计算令牌数，确保不超过限制

### 2.7 社区报告检索

```python
# 调用社区报告检索方法
community_reports = self._community_retrival_([n for n, _ in ents_from_query], filters, kb_ids, idxnms, comm_topn, max_token)
```

这一步检索与检索到的实体相关的社区报告：
1. 社区是知识图谱中紧密连接的实体集合
2. 社区报告是对这些实体集合的摘要描述
3. 这为用户提供了更高层次的知识图谱理解

### 2.8 返回最终结果

```python
return {
        "chunk_id": get_uuid(),
        "content_ltks": "",
        "content_with_weight": ents + relas + community_reports,
        "doc_id": "",
        "docnm_kwd": "Related content in Knowledge Graph",
        "kb_id": kb_ids,
        "important_kwd": [],
        "image_id": "",
        "similarity": 1.,
        "vector_similarity": 1.,
        "term_similarity": 0,
        "vector": [],
        "positions": [],
    }
```

最后，方法将所有检索到的信息整合为一个统一的结果对象，包含：
1. 实体信息（CSV格式）
2. 关系信息（CSV格式）
3. 社区报告（Markdown格式）

这个结果对象与普通的文档块具有相同的结构，使得它可以无缝集成到RAGFlow的检索流程中。

## 3. 技术亮点

### 3.1 多维度检索

`retrieval` 方法不仅仅依赖于简单的关键词匹配，而是从多个维度进行检索：
- 实体关键词检索
- 答案类型检索
- 文本相似度检索
- N跳路径检索
- 社区报告检索

这种多维度检索策略大大提高了检索的召回率和准确性。

### 3.2 查询理解与重写

使用大语言模型进行查询理解和重写是该方法的一大亮点：
- 从预定义的类型池中选择适当的答案类型
- 从查询中提取关键实体
- 这种方式比传统的关键词提取更加智能和准确

### 3.3 图结构感知排序

该方法不仅考虑了实体和关系与查询的相似度，还考虑了它们在图中的结构重要性：
- 使用PageRank值衡量实体和关系在图中的重要性
- 考虑N跳路径，捕捉实体间的间接关系
- 对同时出现在不同检索结果中的实体和关系进行加权

### 3.4 令牌控制

方法实时计算结果的令牌数，确保不超过限制，这对于与大语言模型的集成至关重要：
- 按重要性顺序添加实体和关系
- 当接近令牌限制时，及时截断结果
- 确保最重要的信息能够被包含在结果中

## 4. 总结

`KGSearch` 类的 `retrieval` 方法是一个复杂而强大的知识图谱检索实现，它通过多维度检索、查询理解与重写、图结构感知排序和令牌控制等技术，将自然语言查询转换为结构化的知识图谱检索结果。这种实现使得RAGFlow能够有效地利用知识图谱中的结构化信息，增强其处理复杂问题的能力，特别是对于需要理解实体间关系的多跳问题。
