# RAGFlow CLI 设计：无守护进程、单 owner、跨平台

- 状态：Draft（2026-11-24）
- 适用范围：`cmd/` 的 CLI 与 desktop 构建、`internal/desktop`、`internal/cli`
- 平台：macOS、Linux、Windows

## 1. 目标与非目标

目标：

- **G1 命令行自足**：`ragflow <cmd>` 不依赖任何常驻服务即可正确工作（grep 心智：进程即调用，退出即结束）。
- **G2 目录即实例**：`--data-dir` 决定一切。同一个目录只有一个 owner，其余进程是它的客户端；不同目录互不打扰。
- **G3 读得快**：热路径 5–20 ms，冷路径 ≤200 ms（口径见 §11）。
- **G4 可脚本化**：stdout 只放数据，stderr 只放日志，退出码稳定（`0`/`1`/`2`），默认支持 `--json`。

非目标：

- 不做系统服务、不做开机自启、不注册 launchd/systemd/Windows Service。
- 不支持多台机器共享同一个 data-dir（依赖本地文件锁与 mmap）。
- 不提供远程 API（那是 `serve` 形态，不属于本文范围）。

## 2. 名词

| 名词 | 含义 |
|---|---|
| data-dir | 一个实例的全部状态（SQLite、索引、对象、日志）所在目录 |
| owner | 当前持有该 data-dir 所有权锁的进程，唯一可变者 |
| client | 同目录后续启动的进程，attach 到 owner 上，不持锁 |
| 读栈 | owner 内部只读的那部分：SQLite 只读连接、索引 mmap、对象目录直读 |
| 写栈 | owner 内部可写的那部分：pebbis、NATS、S3 网关、embedded Infinity |
| 冷路径 | 没有可用 owner 时，进程自己竞选成为 owner 后处理本次命令 |

## 3. 核心决策：每目录单 owner，而非"读者无锁"

| | A：读者无锁 + 不可变文件 | B：单 owner（**选定**） |
|---|---|---|
| 跨进程一致性 | 每请求校验 `generation`，文件必须不可变，GC 需要宽限窗口 | owner 是唯一可变者，mmap 与缓存天然有效 |
| 冷启动 | 120–200 ms，无需任何锁 | ~200 ms（含取锁与读栈初始化） |
| 写 | 任意进程临时取锁 | 必须经 owner（IPC） |
| 迁移 / 压缩 / GC | 需要与读者协调 | owner 就地做，无协调 |
| 失败模式 | 无单点 | owner 卡死时新进程无法接管，需要兜底（§10） |

选 B 的理由：RAGFlow 的读路径要持有大块 mmap 与远超调用时长的缓存，跨进程让这些缓存保持一致的成本（不可变布局、GC 宽限、世代号校验）远高于"把写收敛到一个进程"。B 的代价是把写路径也搬进 owner，这正是下一节要定义的。

## 4. 进程形态

| 形态 | 何时出现 | 持锁 | 生命周期 |
|---|---|---|---|
| **cold CLI** | `ragflow search/ask/import ...`，且目录无 owner | 竞选成为 owner，即持锁 | 命令结束即退（若 `--warm` 则留驻） |
| **owner（暖）** | 同上，或由 `mcp`/`ui` 拉起 | 持有，直到退出 | 前台命令 / 父进程 / 窗口 / 空闲超时 |
| **client** | 同目录已有 owner | 不持锁 | 与它所属的命令同寿 |
| **UI owner** | `ragflow ui` | 持有 | 窗口关闭即退出 |

UI 是一个特殊的 owner：窗口进程自己就是 owner，给内嵌 WebView 开 ephemeral loopback HTTP；窗口关闭 = 进程退出 = 锁释放。

## 5. 实例身份

### 5.1 目录规范化（必须收敛到同一个 owner）

同一目录的不同写法必须映射到同一个实例，否则会出现两个 owner 写同一批文件：

1. `filepath.Abs`；
2. `filepath.EvalSymlinks`；
3. 平台修正：
   - macOS：APFS 默认大小写不敏感，`/Users/x/D` 与 `/Users/x/d` 是同一目录 → **不要用字符串路径做键**；
   - Windows：盘符大小写、`\\?\` 长路径前缀、UNC 路径、8.3 短名、`subst` 映射；
   - Linux：bind mount / 多次挂载同一目录 → 同样不能只用路径。

**键一律用文件系统标识**：Unix `(dev, ino)`，Windows `(volume serial, file ID)`；仅在无法取得时退回 `hash(realpath)`。

### 5.2 运行时位置

| 平台 | 通信端点 | 说明 |
|---|---|---|
| Linux | `$XDG_RUNTIME_DIR/ragflow/<hash>/owner.sock`，兜底 `/tmp/ragflow-<uid>/<hash>/` | tmpfs，登出自动清理；目录 `0700`、socket `0600` |
| macOS | `~/Library/Caches/ragflow/run/<hash>/owner.sock` | 无 XDG；`sun_path` 上限 104 字节，`<hash>` 必须短 |
| Windows | 命名管道 `\\.\pipe\ragflow-<hash>` | 不落盘、不监听 TCP、不过防火墙 |

### 5.3 data-dir 内布局

```
<data-dir>/.ragflow/
  owner.lock          # 所有权锁（flock / LockFileEx），O_CREAT，永不删除
  state/schema        # 整数：schema 版本
  state/owner.json    # 建议态诊断：pid、version、endpoint、startedAt
  state/manifest/…    # 索引清单，原子替换发布
```

`owner.json` 是**建议性**的（用于 `ragflow status` 和报错信息），不是真源；真源是 `owner.lock` 的持有状态。

## 6. 启动协议

```
realpath(dataDir) → 计算 key
open+create <dir>/.ragflow/owner.lock
flock(LOCK_EX | LOCK_NB)
├─ 成功 → 我是 owner
│   ├─ 读 state/schema
│   │   ├─ 缺失 + 命令是只读 → 释放锁，stderr 提示 "数据目录未初始化，请先 ragflow init"，退出码 2
│   │   ├─ 缺失 + 命令是写入 → 初始化（目录/schema/seed/首个 manifest）→ 写 owner.json
│   │   ├─ schema > 本进程支持 → 释放锁，提示升级，退出码 2
│   │   └─ 正常 → 起读栈（~120ms）
│   ├─ 写 owner.json、监听 owner.sock / 命名管道
│   ├─ 处理本次命令
│   └─ 空闲计时（默认 10 分钟）或父作用域结束 → 退出，释放锁
└─ 失败 → 已有 owner
    ├─ 连接（超时 200ms）→ hello 握手
    │   ├─ version/schema 匹配 → 转交本次命令（request-scoped 配置随请求传），流式收结果
    │   ├─ version 过期 → 请 owner 退出后重新竞选
    │   └─ schema 更高 → 提示升级，退出码 2
    └─ 连不上 → 视为残留端点：删除后重试一次
```

三条硬规则：

1. **先加锁，再判定 schema**。否则两个首次启动的进程会各自建表。
2. **所有失败路径必须先释放锁再退出**，否则一个失败的进程会把目录锁到空闲超时。
3. 读命令不创建、不修改任何**数据**文件（schema、manifest、索引、对象）；唯一允许的是 `owner.lock` 与建议性的 `state/owner.json`。

### 6.1 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 成功执行但没有匹配结果（grep 语义） |
| 2 | 用法错误、目录未初始化、schema 需要升级、owner 无响应 |

## 7. 请求协议

- 传输：Unix socket（macOS/Linux）、命名管道（Windows）、stdio（`ragflow mcp --stdio` 形态）。
- 帧格式：**`\n` 分隔的 JSON-RPC 2.0**。理由：三种传输都是字节流，不需要再叠一层长度前缀；Windows 上读侧需 `TrimRight("\r")`。
- `stdout` 只承载协议（stdio 形态）或命令结果，日志一律走 `stderr`。

### 7.1 hello

```json
→ {"jsonrpc":"2.0","id":0,"method":"hello","params":{
     "version":"v1.0.0-300-g9a9574278",
     "schema":42,
     "dataDir":"/Users/x/data",
     "fingerprint":"sha256:…"}}
← {"result":{"role":"owner","pid":1234,"startedAt":"…","capabilities":["search","ask","import"]}}
```

`version` 不匹配 → 客户端要求 owner 退出后重新竞选；`schema` 更高 → 客户端报"请升级"，退出码 2。

`fingerprint` 只覆盖**不可按请求覆盖**的配置（存储路径、索引格式、向量维度）。它不一致意味着两个进程对同一批文件的解释不同，属于配置错误：**不 attach，也不允许无锁冷读**（owner 可能正在写），直接退出码 2 并提示该目录的实例信息。可按请求覆盖的配置（模型选择、`top-k`、LLM key）不进入指纹，随请求传。

### 7.2 方法与流式

| 方法 | 说明 |
|---|---|
| `search` / `ask` | 只读，流式返回（先若干 `{"chunk":…}` 帧，最后 `{"end":true}`） |
| `import` / `watch` | 写，返回 `{taskId}`，随后 `progress` 帧，支持 `cancel` |
| `status` | 版本、schema、owner pid、数据规模、就绪状态 |
| `reload` | 要求 owner 丢弃缓存（升级后或外部修改后使用） |
| `shutdown` | 供版本不匹配与测试使用，优雅退出 |

## 8. owner 的角色切换

| 能力 | 读栈（owner 启动即有） | 写栈（首个写请求时启动，一次性 ~300ms） |
|---|---|---|
| 元数据 | SQLite 只读连接（WAL） | SQLite 读写连接 |
| 文档引擎 | Infinity 只读打开（manifest → chunk mmap） | embedded Infinity + pebbis + NATS + S3 网关 |
| 对象存储 | 直接读 `s3-objects/` | versitygw |
| 分词器 | 1 个实例（懒加载） | analyzer pool |
| 迁移 / GC | 不做 | 只在 owner 内做 |

要点：

- **迁移与 DDL 只在 owner 内执行**。owner 是唯一读者，不会被自己的连接挡住；但执行前要先丢弃只读连接，避免自锁。
- 写栈启动后**不因空闲而关闭**（重开代价 300ms 且涉及目录锁竞争），统一由 owner 的空闲超时回收。
- 长任务（`import`）必须在协议里带 `deadline`、`cancel` 与可恢复标记，owner 崩溃后下一个进程能续跑。

## 9. 一致性

1. **manifest 原子替换**：写新文件 → 写临时 manifest → `rename(2)`/`MoveFileEx` 覆盖。崩溃最多丢一次提交，不会暴露半成品。
2. **不需要 `generation` 轮询**：attach 的客户端直接问 owner；owner 死后接管的进程从磁盘冷读，本来就没有旧缓存。
3. **SQLite**：沿用现有 `journal_mode(WAL)` + `busy_timeout(5000)`；owner 退出前做一次 checkpoint（无读者时 `TRUNCATE`），这样下次冷启动可以用纯只读方式打开。
4. **对象目录**由 owner 独占，读客户端不经文件系统直读——统一走 IPC，避免 Windows 上"打开中的文件无法删除"的语义差异泄漏到业务层。

## 10. 生命周期与失败兜底

| 场景 | 行为 |
|---|---|
| 前台命令结束 | owner 退出（`--warm` 则留驻到空闲超时） |
| stdio / 父进程结束 | stdin EOF + 父进程句柄失效（Windows）→ 退出 |
| UI 窗口关闭 | 进程退出 |
| 空闲超时 | 默认 10 分钟，`RAGFLOW_OWNER_IDLE` 可配；到期优雅退出并清理端点 |
| 崩溃 / `kill -9` | 锁由 OS 回收，端点残留 → 下次 `ECONNREFUSED` 后删除并重新竞选 |
| 升级二进制 | `version` 不匹配 → 旧 owner 自杀；Windows 上 owner 每请求 `stat(os.Executable())`，变化即退出（exe 无法被替换） |
| owner 卡死 | `hello` 200ms 超时 → 报 `<dataDir> 的实例 (pid) 无响应`；**不提供抢锁**，只能终止该进程（防止双写） |
| 数据目录被删/移动 | owner 检测到锁文件 inode 变化即退出；客户端按 key 重新竞选 |

## 11. 性能预算

macOS 实测（2026-11，Apple Silicon，页面缓存已热）：

| 场景 | 实测/目标 |
|---|---|
| 进程 exec + Go runtime + 二进制页入（`--version`） | 80 ms（地板） |
| 冷路径：竞选 + 读栈 + 首次查询 | ~200 ms |
| 暖路径：attach + 查询 | 5–20 ms |
| owner 首次 promote 到写栈 | ~300 ms |
| 当前 desktop 全量启动（对照，非本设计） | 719 ms |

Windows 预期：冷路径 300 ms–1 s（Defender 扫描 400 MB+ exe），因此 **Windows 上 owner 常驻的收益更大**，二进制瘦身优先级更高。

CI 闸门（三平台各跑一遍）：

- `cold p95`：Linux/macOS ≤ 250 ms，Windows ≤ 600 ms；
- `warm p95` ≤ 20 ms；
- owner 重启（崩溃后首个命令） ≤ 400 ms。

## 12. 失败矩阵

| 情况 | 表现 | 处理 |
|---|---|---|
| 双初始化竞态 | 两个进程同时首次启动 | 先锁后判定；败者等待/退出码 2 |
| 残留端点 | 连上即 EOF | 删除端点，重试一次竞选 |
| 指纹不一致（见下） | 两个进程对同一批文件的解释不同 | 退出码 2，报出该目录的 owner 信息；**不允许无锁冷读** |
| owner 卡死 | 命令 200 ms 超时 | 报 pid，让用户终止；不做抢锁 |
| schema 更高 | 旧二进制读新库 | 退出码 2，"请升级 ragflow" |
| 目录只读（U盘/CD） | 无法建 `owner.lock` | 明确报"数据目录需要可写"；不提供只读挂载支持 |
| 磁盘满 | 初始化/提交失败 | 提交前检查，失败保留上一次 manifest |

**config/env 指纹**：owner 用某组配置启动（存储路径、索引格式、向量维度、端口）。客户端若与之不同，复用它会产生"配置没生效"或"解释错了文件"的问题。规则：

- 指纹只包含**不可按请求覆盖**的项；不一致 → 退出码 2，提示关闭该目录的实例后重试；
- request-scoped 配置（模型选择、`top-k`、LLM key、rerank 端点）**不进入指纹**，随请求传，因此不同客户端可以共用一个 owner；
- 任何情况下都不允许"绕过锁直接读文件"：owner 可能正在写。

## 13. 跨平台原语

| 需求 | Linux | macOS | Windows | Go |
|---|---|---|---|---|
| 所有权锁 | `flock` | `flock` | `LockFileEx` | `gofrs/flock` 或 `x/sys/unix` + `x/sys/windows` |
| 本地 IPC | unix socket | unix socket（`sun_path` ≤ 104） | 命名管道 | `net.Listen("unix")` / `go-winio` |
| 帧格式 | `\n`-JSON | 同 | 同（trim `\r`） | `bufio` |
| 原子替换小文件 | `rename(2)` | `rename(2)` | `MoveFileEx` + `FILE_SHARE_DELETE` | `os.Rename` / `os.Root` |
| 只读 mmap | `mmap` | `mmap` | `CreateFileMapping` + `MapViewOfFile` | `unix.Mmap` / `x/sys/windows` |
| 父进程死亡 | EOF | EOF | 继承父进程句柄 + `WaitForSingleObject` | `SysProcAttr.AdditionalInheritedHandles` |
| 连带终止子树 | `setsid` + 进程组 | 同 | Job Object（`KILL_ON_JOB_CLOSE`） | `x/sys/windows` |
| 默认 data-dir | `~/.ragflow/desktop` | 同 | `%LOCALAPPDATA%\RAGFlow` | `os.UserHomeDir` |

### 13.1 Windows 专属规则（必须写进实现，不能事后补）

1. **`FILE_SHARE_DELETE`**：Go 的 `syscall.Open` 只带 `FILE_SHARE_READ|FILE_SHARE_WRITE`，因此被打开的文件无法被替换或删除。需要被 `rename` 覆盖的小文件（manifest、`owner.json`）用 **`os.Root`**（Go 1.24+，其 Windows 打开路径带 `FILE_SHARE_DELETE`）或自建 `CreateFile` 包装；不可变大文件用普通 `os.Open`，但 GC 必须容忍 `ERROR_SHARING_VIOLATION`（跳过、下次再删）。
2. **命名管道安全**：显式安全描述符只授予当前用户（`winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;<userSID>)"}`），并设置 `PIPE_REJECT_REMOTE_CLIENTS`。
3. **不要让父进程死亡依赖 stdin EOF**：额外继承父进程句柄并在子进程 `WaitForSingleObject`，EOF 作为第二重信号，空闲超时作为第三重。
4. **数据目录不能落在 OneDrive/网络盘**：WAL 的 `-shm`、SQLite 的 `LockFileEx`、mmap 在这些位置上会异常。启动时检测 reparse point / 网络卷，命中则拒绝写模式。
5. **长路径**：对象 key 超过 260 字符会失败，对象存储布局必须做哈希分层；同时避免从 GUI 派生进程时弹出控制台窗口（`CREATE_NO_WINDOW`）。

### 13.2 代码组织约束

- 平台差异**只允许**出现在 `internal/warm`（锁、IPC、端点、父进程监视）与 `internal/storage`（mmap、原子替换）两个包里；
- 业务代码禁止出现 `runtime.GOOS` 分支；
- 每个平台特有的路径必须有一个与其对称的集成测试（见 §11 闸门、§15 M4）。

## 14. 与现有代码的映射

| 现状 | 变化 |
|---|---|
| `cmd/ragflow_server.go` 的 `main()` 与 `startDesktopBackends` 钩子 | 变成 owner 竞选入口；钩子改为"owner 生命周期"（读栈起步、按需 promote） |
| `--data-dir` 标志（`desktopDataDir`） | 成为实例身份来源；非 desktop 构建同样支持 |
| `internal/desktop` | 拆成 `ReadOnly`（SQLite RO / Infinity 只读 / 对象直读）与 `Backends`（现有写栈） |
| `internal/server/bootstrap` 的 `SetDesktopEndpoints` | owner promote 时注入写栈端点，保持现有方式 |
| `internal/dao` 的 SQLite DSN（WAL + busy_timeout） | 增加只读连接与 checkpoint-on-exit |
| `internal/cli` | 增加 `--json`、退出码、attach 逻辑与 `--no-warm` |
| 新增 `internal/warm` | 锁、端点、握手、`owner.json`、父进程监视、空闲计时 |

## 15. 落地顺序

| 里程碑 | 内容 | 验收 |
|---|---|---|
| M0 | `owner.lock` + 竞选 + 退出码（无 IPC：拿不到锁就报错退出） | 两个进程同目录，第二个明确报错 |
| M1 | 只读栈 + 客户端 attach + `mcp --stdio` 形态 | 两次 `search`：第一次 ~200 ms、第二次 ≤20 ms |
| M2 | 写栈 promote + `import` 经 IPC（进度/取消/续跑） | import 时另一进程 search 不阻塞 |
| M3 | UI owner（内嵌 WebView + 前端 `go:embed`） | 关窗即退出、锁释放 |
| M4 | 三平台 CI 矩阵 + §11 闸门 + §12 故障用例 | Windows/macOS/Linux 全绿 |

## 16. 待定

1. owner 空闲退出后，写命令是否自己当 owner（当前倾向：是，规则统一）。
2. `--warm` 默认值（当前倾向：`off`；`mcp`/`ui` 强制 `on`）。
3. 指纹具体包含哪些**不变项**（索引格式版本、向量维度、资源路径）；request-scoped 项一律不进指纹。
4. Windows 是否保留 Unix socket 兜底（当前倾向：不保留，只走命名管道）。
5. 只读挂载（U 盘/只读镜像）是否需要支持（当前：不支持，明确拒绝）。
