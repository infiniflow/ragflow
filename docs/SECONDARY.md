# kb 二次开发约定（Secondary Development Conventions）

> 作者：张彦龙

本文档约定了在本 fork（`softctwo/ragflow`）上进行二次开发的边界与方式。目标只有一个：**在不改动 RAGFlow 核心代码的前提下叠加我们的定制内容，保证随时可以与上游合并。**

## 1. 产品工作名

- 内部工作名：**kb**
- 该名称仅用于内部文档、覆盖层配置与部署脚本，本 PR 及覆盖层不修改任何 UI 文案、品牌资源或应用源码。品牌化调整（如有）记录在 `overlay/` 下的说明中，通过部署期覆盖实现，而非改源码。

## 2. 上游跟踪策略

- 上游仓库：[`infiniflow/ragflow`](https://github.com/infiniflow/ragflow)，上游迭代很快，`main` 分支需定期同步。
- 部署基线：**优先使用稳定标签 `v0.26.4`**。选择部署镜像时，Docker 镜像标签（`RAGFLOW_IMAGE`）必须与所用 compose 文件、`entrypoint.sh` 的版本相互匹配——不同版本之间入口脚本参数和服务拓扑可能不兼容，混用会导致启动失败。
- 升级流程：先在测试环境用目标标签验证 `overlay/` 覆盖层可用，再切换生产。

## 3. 核心代码红线

以下内容 **禁止直接修改**，除非通过覆盖层或上游已文档化的扩展点：

- 核心解析器（`deepdoc/`、`rag/`、`internal/parser/` 等）
- API 服务端（`api/`、`internal/handler/` 等）
- 前端（`web/`）的源码与 UI 文案
- Docker 镜像内部结构（不重打镜像、不改 `entrypoint.sh`）

允许的扩展方式：

- `overlay/` 下的部署期覆盖（环境变量、compose override）
- 上游文档化的扩展点（自定义解析器/插件注册机制、Agent 组件模板等），使用前先在本文件登记

## 4. 定制内容位置

所有定制内容集中在仓库根目录的 `overlay/`：

| 内容 | 位置 |
|---|---|
| 环境变量样例 | `overlay/.env.example` |
| Compose 覆盖样例 | `overlay/docker-compose.override.example.yml` |
| 使用说明与品牌化备注 | `overlay/README.md` |
| 插件/解析器桩代码（后续按需添加） | `overlay/` 下新建子目录 |

`overlay/` 中的文件是**样例**，实际部署时复制到 `docker/` 目录旁使用，真实密钥与私有配置一律不入库。

## 5. 外部集成方式

外部系统与 kb 集成只走以下三条官方通道，不直连内部服务：

1. **HTTP API**：端口 `9380`（`.env` 中的 `SVR_HTTP_PORT`），RESTful 接口，使用 API Key 鉴权。
2. **Python SDK**：[`ragflow_sdk`](https://pypi.org/project/ragflow-sdk/)，`pip install ragflow-sdk`，底层同样走 9380 HTTP API。
3. **MCP（可选）**：端口 `9382`（`SVR_MCP_PORT`），需在 compose 中取消注释 `--enable-mcpserver` 相关启动参数后才会启用，详见 `overlay/docker-compose.override.example.yml`。

## 6. 最低硬件与软件要求

- CPU：4 核
- 内存：16 GB
- 磁盘：50 GB
- Docker：>= 24.0.0
- Docker Compose：>= v2.26.1
