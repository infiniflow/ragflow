“你是一个AI助手，负责帮助用户生成代码和开发软件。请遵循以下项目开发规范清单：
# 项目开发规范清单（Checklist）

## 总则
- 遵循“设计优先、模块化、统一风格、可测试、可发布”原则。
- 新增功能必须包含最小可用实现、错误处理、日志、测试与文档。
- 所有变更需确保本地 lint、格式化与测试全部通过。

## 架构与设计
- 先写设计说明：模块边界、数据流、依赖、异常与返回结构。
- 后端分层：路由（`api/apps/*`）→ 服务（`api/db/services/*`）→ 工具（`api/utils/*`、`common/*`）。
- 前端分层：页面/组件/服务/状态（Zustand），遵循 Umi 约定式结构。
- 外部系统或插件接入应独立模块/包，接口清晰，依赖最小化。

## 代码风格与格式化
- Python：使用 Ruff；行宽 `200`；本地执行 `ruff check` 与 `ruff format`。
- 前端：执行 `npm run lint`；ESLint 继承 `umi/eslint` 与 `react-hooks` 规则集。
- Prettier：`printWidth: 80`、`singleQuote: true`、`trailingComma: all`、`endOfLine: lf`；启用导入与 `package.json` 插件。
- 启用本地钩子：`pre-commit install` 与 `lint-staged` 自动格式化变更文件。

## 命名与结构
- 前端文件名：`**/*.{jsx,tsx,js,ts}` 使用 `[a-z0-9.-]*`；目录：`src/**` 与 `mocks/*/` 采用 `KEBAB_CASE`。
- 函数/类/模块命名具备语义，避免缩写与魔法常量；公共类型优先 `TypedDict/Generic`。
- 代码组织保持小而清晰的模块，公共逻辑抽取至 `common/*` 或 `api/utils/*`。

## 错误处理与返回
- 后端统一响应结构：`{"code": RetCode, "message": str, "data": any}`。
- 异常统一通过 `server_error_response(e)` 捕获并记录日志。
- 参数校验：`@validate_request(...)`；鉴权：`@apikey_required` 或 `token_required`。
- 业务错误返回：`get_error_data_result`、`get_error_argument_result`、`get_error_permission_result`。

## 安全与鉴权
- 受限路由必须校验 `Authorization`，统一使用 `RetCode` 返回码。
- 统一 CORS 头通过 `construct_response` 设置；避免将内部错误堆栈暴露给客户端。
- 外部服务调用设置超时与重试，日志中敏感信息脱敏。

## 测试规范
- Python：`pytest`；为新功能添加单元/集成测试；使用标记 `p1/p2/p3` 表示优先级。
- 前端：`npm run test`（Jest + RTL）；`jest-setup.ts` 做环境初始化；覆盖率收集在 `jest.config.ts`。
- CI 必须通过：SDK 测试、前端 API 测试、HTTP API 测试（Elasticsearch 与 Infinity 引擎）。

## 性能与稳定性
- 外部调用具备超时、重试、退避机制；避免在请求路径上执行阻塞操作（必要时异步/任务队列）。
- 明确资源清理（如 MCP 工具会话关闭）；避免全局可变状态污染。

## 文档与注释
- 模块/公共方法保持 `docstring`：功能、参数、返回、异常。
- 新增/变更 API 在 `docs` 或相应参考文档补充说明与示例。
- 文件头添加版权与许可证注释；避免过多内联注释影响可读性。

## 提交与分支
- 提交信息清晰、可检索：动词开头，指明模块与影响范围，关联 Issue/PR。
- 单一 PR 只解决一个问题；大型改动拆分为可审查的独立 PR。
- 遵循语义化版本；避免在发布分支引入无关变更。

## CI/CD 与发布
- 变更前本地执行：后端 `ruff check`、`ruff format`、`uv run pytest`；前端 `npm run lint`、`npm run test`。
- 流水线：`tests.yml` 运行 Ruff、构建镜像、双引擎测试；`release.yml` 处理 `v*.*.*` 与 `nightly`、推送镜像与 Python 包。
- 发布日志包含构建来源与时间，确保可追溯性。

## 依赖与环境
- Python：`>=3.10,<3.13`；使用 `uv` 管理依赖与构建；清华镜像加速安装。
- Node：`>= 18.20.4`；前端使用 Umi 脚本；`postinstall` 执行 `umi setup`。
- Docker：开发与 CI 使用 `docker-compose` 启动依赖；必要时切换 `DOC_ENGINE=infinity`。

## 前端特定规范
- 组件拆分与复用，避免在页面承载复杂业务逻辑；副作用与数据通过 hooks 管理。
- i18n 文案集中在 `src/locales/*`，新增文案保持 key 一致性。
- 样式优先使用 Tailwind 原子类与主题变量，Less 用于局部需求。

## 后端特定规范
- 路由控制器保持轻逻辑：参数校验、调用服务、组装返回；业务逻辑下沉到服务层。
- ORM 操作通过服务层；迁移与连接池正确使用 Peewee/Playhouse 工具。
- 统一日志级别与格式，敏感信息脱敏。

## 变更与发布前检查
- 设计文档更新；对外 API 变更注明破坏性影响与迁移指南。
- 兼容性检查：双引擎（Elasticsearch/Infinity）、多模型供应商路径。
- 自测清单：功能、错误处理、边界条件、性能关键路径。

## 评审要点
- 设计合理性与模块边界清晰。
- 风格一致、命名规范、无死代码、无魔法常量。
- 错误处理充分、返回结构统一、日志易于排查。
- 测试到位、覆盖关键场景、CI 全绿。
- 文档与示例完整，便于集成与维护。