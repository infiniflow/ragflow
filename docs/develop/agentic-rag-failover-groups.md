# Agentic RAG 可配置 Failover Group 设计

## 背景

当前 agentic RAG 在解析 chat model 时，会把租户名下所有 active chat model 组成一个 failover chain。这个策略会把用途不同、模型能力不同、成本不同的模型混在一起：例如一个租户同时配置了通用聊天模型、视觉模型和实验模型时，任意一个模型故障都可能切换到不适合当前对话的模型。

本设计让管理员在模型设置界面中选择若干“模型实例 + 模型 ID”，将它们归入一个 chat failover group。agentic RAG 只使用当前请求指定的 group；未指定 group 时保留单模型行为，逐步淘汰“自动纳入全部租户模型”的隐式策略。

## 目标

- 以 `(provider_id, instance_id, model_id)` 作为 failover 成员的明确身份。
- 一个 group 只包含同一能力类别的模型，默认用于 chat；不把 embedding、rerank、TTS 或 vision-only 模型加入 chat chain。
- 支持成员顺序和权重配置，第一阶段采用显式优先级顺序，后续可扩展加权或健康度策略。
- 在界面上创建、重命名、启停 group，并增删成员。
- 后端校验租户边界、模型类型、模型状态和成员归属，避免通过请求参数越权引用其他租户的模型。
- 当前模型选择仍然可以覆盖 group：请求显式指定 `llm_id` 时，它必须是该 group 的成员；否则请求失败并给出可操作错误。

## 非目标

- 不把 MiniMax 凭据中的 `group_id` 当作 RAGFlow failover group。MiniMax 的 `group_id` 继续只存在于实例 API credential 中。
- 不在一次请求中混合不同模型名称、不同上下文窗口或不同工具调用能力的模型，除非管理员明确把它们放入同一 group 且通过能力校验。
- 不改变普通聊天、embedding、rerank、OCR、TTS 的模型选择流程。
- 不在本阶段实现跨租户共享 group。

## 数据模型

仓库已有 `tenant_model_group` 和 `tenant_model_group_mapping` 表，但当前表结构不足以表达租户拥有关系和 chat failover 语义。建议在现有概念上收敛，而不是再引入一套与模型组重复的 API：

### `tenant_model_group`

增加或明确以下字段：

| 字段 | 说明 |
| --- | --- |
| `id` | group ID，服务端生成，不使用 MiniMax `group_id` |
| `tenant_id` | 所属租户，必填并建立索引 |
| `name` | 租户内展示名称，必填 |
| `group_type` | 第一阶段固定为 `chat_failover` |
| `strategy` | 第一阶段固定为 `priority` |
| `status` | `active` / `inactive` |
| `created_by`、时间字段 | 复用 `BaseModel` 或现有审计字段 |

约束：`(tenant_id, name)` 唯一；`id` 全局唯一；删除 group 前必须删除或级联删除其 mapping。若线上表已经被其他模型组功能使用，新增 `tenant_id`、`name` 时需要兼容现有记录，并将旧记录标记为非 `chat_failover`，禁止自动加入 agentic chain。

### `tenant_model_group_mapping`

保留并强化现有成员表：

| 字段 | 说明 |
| --- | --- |
| `group_id` | failover group ID |
| `provider_id` | 模型 provider |
| `instance_id` | 模型实例 |
| `model_id` | `tenant_model.id`，不是模型展示名称 |
| `priority` | 建议替代或复用 `weight`；数值越小优先级越高 |
| `status` | `active` / `inactive` |

主键继续使用 `(group_id, provider_id, instance_id, model_id)`，并增加唯一约束，避免同一模型重复加入同一个 group。服务端每次保存时验证 `model_id` 与 provider/instance 三元组一致；不能接受“只传 model_id、由服务端猜实例”的模糊请求。

如果已有调用方依赖 `weight`，第一阶段可以把 `priority` 映射到 `weight`，但 API 对外只暴露 `priority`，避免把尚未实现的加权 failover 暗示为已支持能力。

## API

建议在现有 `/api/v1` 下增加租户模型组资源：

```text
GET    /tenant-model-groups?group_type=chat_failover
POST   /tenant-model-groups
GET    /tenant-model-groups/:group_id
PATCH  /tenant-model-groups/:group_id
DELETE /tenant-model-groups/:group_id

PUT    /tenant-model-groups/:group_id/members
PATCH  /tenant-model-groups/:group_id/members/:model_id
DELETE /tenant-model-groups/:group_id/members/:model_id
```

创建请求示例：

```json
{
  "name": "生产 Chat 主备",
  "group_type": "chat_failover",
  "strategy": "priority",
  "members": [
    {
      "provider_id": "provider-a",
      "instance_id": "instance-a",
      "model_id": "model-a",
      "priority": 10
    },
    {
      "provider_id": "provider-b",
      "instance_id": "instance-b",
      "model_id": "model-b",
      "priority": 20
    }
  ]
}
```

响应应返回 group、已解析的 provider/instance/model 展示信息、成员状态和验证错误。写操作必须使用当前用户可管理的 tenant，而不能从 body 接收 `tenant_id` 作为授权依据。

## 前端交互

在“用户设置 → 模型”页面增加 **Failover Groups** 区域，和 provider instance 卡片并列：

1. 点击“新建 Group”，输入名称，类型默认为 `Chat Failover`。
2. 从当前租户可用的 chat 模型列表中选择成员。每一行显示 provider、instance、model name、model ID、上下文窗口和工具调用能力。
3. 通过拖拽或上下移动设置 priority。
4. 保存前在前端提示：至少 2 个成员才能形成 failover；inactive 或类型不兼容的模型不可选。
5. 删除或停用 group 时提示 agentic RAG 将回退到请求指定的单模型行为。
6. provider instance 或 model 被删除、停用时，group 页面显示失效成员并允许移除；不能静默改写 group。

`Group ID` 字段继续留在 MiniMax 实例凭据表单中，文案改为“MiniMax Group ID（凭据）”或增加说明，避免与 RAGFlow failover group 混淆。

## 请求配置

agentic RAG 的请求参数新增：

```json
{
  "agent_mode": "smart-reasoning",
  "failover_group_id": "<tenant-model-group-id>"
}
```

`failover_group_id` 的解析规则：

1. 有值：加载该 tenant 的 active `chat_failover` group 和 active mappings。
2. 按 priority 升序加载成员，并解析每个 `model_id` 的 API 配置。
3. 若 `llm_id` 有值，它必须是 group 成员；将其移动到 chain 首位，其他成员按 priority 保持相对顺序。
4. 无值且 `llm_id` 有值：只创建该模型的单成员 chain，不再自动把租户其他模型加入 chain。
5. 两者都无值：使用租户默认 chat model 的单成员 chain。
6. group 不存在、无 active 成员、成员解析失败或 `llm_id` 不属于 group 时，返回明确错误，不静默扩大到全租户模型。

这样可以保持用户选择的 primary model 语义，同时让 failover 范围完全由 group 决定。

## 后端实现

### 服务层

新增 `TenantModelGroupService`，负责：

- tenant-scoped CRUD；
- 成员集合的事务性替换；
- provider/instance/model 一致性校验；
- chat 能力、active 状态和重复成员校验；
- 删除模型实例时返回受影响的 group，供 UI 提示或阻止危险删除。

将 `ListTenantChatModelRefs` 改为按 group 查询，例如：

```go
ListChatModelRefsByFailoverGroup(ctx, tenantID, groupID) ([]ChatModelRef, error)
```

`agenticModelChain` 接收 `failover_group_id`，仅从该方法获取 chain。现有“列出租户全部 chat model”的方法可以保留给模型设置页，但不得继续作为 agentic failover 的隐式来源。

### failover 行为

`NewFailoverEinoChatModelWithLabels` 继续负责运行时 failover。label 改为：

```text
<group-name>: <provider-name>/<instance-name>/<model-name>
```

它只用于日志和诊断，不作为稳定 ID。每次请求创建独立的 failover wrapper，避免不同请求共享 cursor、cooldown 或工具配置。

第一阶段只支持 priority 顺序；模型失败后按顺序尝试下一个成员，保持已有 sticky cursor 和 cooldown 语义。group 配置变更在下一请求生效，不要求重启服务。

## 迁移与兼容

1. 增加数据库迁移：`tenant_model_group.tenant_id/name/status`、mapping 的 `priority`（或明确复用 `weight`）以及所需索引。
2. 不自动把现有租户全部 chat model 生成 group，避免上线后改变模型路由。
3. 旧请求不带 `failover_group_id` 时使用单模型 chain；这与当前“所有模型 failover”不同，但行为更安全、可预测。
4. 在配置发布说明中明确：若要恢复多模型 failover，管理员需要创建 group 并把成员加入其中。
5. 删除旧的全租户 fallback 分支及其专用测试；保留 group 解析和越权校验测试。

## 可观测性

每次 agentic turn 记录：

- `failover_group_id`、group name；
- chain 成员数量和成员 labels；
- primary model；
- 发生切换的失败成员、下一个成员和最终成功成员；
- group 配置解析失败原因。

日志中不得记录 API key、MiniMax credential `group_id` 或完整请求内容。模型使用量记录可增加 `failover_group_id`，便于按 group 统计成本和故障率。

## 测试计划

- DAO/service：跨 tenant 访问、重复成员、provider/instance/model 不匹配、inactive 模型、空 group。
- handler：CRUD、成员替换、权限检查、错误码和响应排序。
- frontend：创建 group、排序、删除失效成员、MiniMax Group ID 文案不混淆。
- agentic pipeline：指定 group 只加载 group 成员；指定 `llm_id` 置首；非成员 `llm_id` 被拒绝；未指定 group 不扩大到租户全部模型。
- failover runtime：按 priority 切换、sticky cursor、cooldown、单成员 group 不发生无意义重试。
- 集成测试：删除/停用 provider instance 后 group 状态和 agentic 请求错误符合设计。

## 分阶段发布

1. **数据与只读 API**：迁移表结构，提供 group 列表和模型成员校验。
2. **前端配置**：开放 group CRUD 和成员排序，但 agentic 仍通过 feature flag 使用旧逻辑。
3. **agentic 接入**：启用 `failover_group_id`，未指定时使用单模型 chain。
4. **清理旧路径**：删除“租户全部 chat model 自动 failover”的实现、配置说明和兼容测试。

