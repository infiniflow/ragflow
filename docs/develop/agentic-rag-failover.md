# Agentic RAG 对话级 Failover 设计

## 背景

agentic RAG 在解析 chat model 时，只使用对话显式选定的模型；未选定则使用租户的默认 chat model。这个策略避免了把用途不同、能力不同、成本不同的模型混在一起：例如一个租户同时配置了通用聊天模型、视觉模型和实验模型时，任意一个模型故障都可能切换到不适合当前对话的模型。

本设计让**对话作者**（而非租户管理员）为自己的对话配置一个有序的 chat model 列表作为 failover chain。agentic RAG 按列表顺序尝试：当前成员出现 provider 级故障（配额墙、限流、 outages、鉴权失败）时切到下一个成员。

范围是**单个对话**，存在对话自己的 `llm_setting` 中。这样 failover 语义跟随使用它的那个对话，而不是一个需要跨实体 join 才能解析的租户级对象。

## 目标

- 支持在 Chat Settings 中为一个对话配置多个 chat model，顺序即优先级。
- 对话自选模型（`llm_id`）永远是 chain 的首位，用户显式选择不被配置覆盖。
- 不引入新表：failover 列表是对话的一个配置字段，随对话创建、复制和删除。
- 后端校验每个成员属于该租户且是 chat 模型；越权或失效成员不拖垮整轮对话。
- 明确不做的校验：不校验成员的 tool-calling 能力。`is_tools` 是 provider 声明的标志而非实测能力，对它做强制校验会同时挡掉可用配置并放过不可用配置。已有的能力守卫（harness 路径的 `chatConfigSupportsTools`）覆盖了真正重要的场景：primary 模型不支持工具调用时。

## 非目标

- 不在租户级别共享 failover 配置：两个对话想用同一组模型，需要各自配置。
- 不做加权路由、负载均衡或健康度评分：第一阶段只有有序优先级。
- 不改变普通聊天、embedding、rerank、OCR、TTS 的模型选择流程。普通聊天路径（`AsyncChat` 主链）不使用本chain。
- 不在流式响应已经开始输出 delta 之后切换模型。切换只发生在首个 delta 之前。

## 数据模型

不新增表。failover 列表是对话的一个 JSON 字段：

| 位置 | 字段 | 说明 |
| --- | --- | --- |
| `chat.llm_setting` | `failover_llm_ids` | 有序的模型 id 数组（`tenant_model.id`），可缺省 |

存JSON 字段而非独立表的原因：

- 列表随对话生命周期存在，删除对话即失效，没有独立的悬挂状态；
- 不需要 join `tenant_model` 就能读出原始 id，解析在 service 层一次完成；
- 复制对话时自动跟随，不需要额外的复制逻辑。

解析容错：非数组、空数组、非字符串元素、空字符串一律丢弃。若列表里混入无法解析的 id（模型已删除或已停用），跳过该成员并记 warning，而不是让整轮对话失败——primary 仍然可用。

## 请求与解析规则

agentic RAG 的模型解析（`agenticModelChain`）：

1. 首位始终是对话自选模型（`llm_id`），未选定时用租户默认 chat model。
2. 之后按 `failover_llm_ids` 的顺序追加成员。
3. 跳过与 `llm_id` 重复的条目。
4. 任一成员解析失败（删除、停用、改类型）时跳过并记 warning，chain 继续。
5. chain 长度为 1 时即普通单模型对话，行为与引入本设计前一致。

chain 交给 `NewFailoverEinoChatModel`：它按顺序尝试成员，命中终端错误后切到下一个；sticky cursor 让已服务的成员保持在首位，一个已经故障的 primary 不会在长对话的每次调用都被重试；全链失败进入 30 秒 cooldown，避免一轮 agentic 提问在每个 ReAct 步都重扫整条链。

## 前端交互

在对话的 **Chat Settings** 中，紧邻既有的 LLM 选择器，增加一个 failover model 列表：

1. 逐项添加成员，顺序即优先级，上移/下移调整顺序。
2. 只列出当前租户的 chat 模型；已被其他对话占用不影响添加。
3. 至少 2 个成员才构成 failover，1 个成员与不配置等价，UI 提示但不强制。
4. 成员失效（模型被删除/停用）时保留该行并标记失效，让作者看到并移除，而不是静默改写列表。
5. 列表随对话保存，复制对话时一并复制。

不复用既有的 "Multiple models" 按钮：那个是多面板**人工对比**（每个面板一个独立对话，各自选模型，见 PR #9477），与 failover 的"单对话自动切换"是不同的意图。

## 可观测性

每轮 agentic 对话记录：

- chain 成员数量，以及因失效被跳过的成员 id；
- 发生切换的失败成员、下一个成员和最终成功成员（由`llm.go` 的 failover 日志提供）；
- primary 模型 id。

日志中不得记录 API key、credential 或完整请求内容。

## 测试计划

- `agenticFailoverModelIDs`：key 缺失、类型错误、空数组、非字符串元素、空字符串、顺序保持、重复项不去重（由调用方跳过）。
- `agenticModelChain`：成员顺序即尝试顺序；失效成员被跳过且不失败整轮；chain 长度为 1 时不改变既有行为。
- 流式路径：首个 delta 之前失败可以切换；已经开始输出 delta 后不切换（`llm.go` 已覆盖）。

## 分阶段发布

1. **后端链路**：`failover_llm_ids` 解析、`agenticModelChain` 组装 chain、切换到 `NewFailoverEinoChatModel`。
2. **前端配置**：Chat Settings 中的成员列表增删与排序。
3. **失效成员可见性**：模型被删除/停用时在列表中标记而非静默移除。

第一阶段即本文档描述的当前实现。
