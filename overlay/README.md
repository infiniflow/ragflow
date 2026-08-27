# overlay/ —— Netstars-KB 二次开发覆盖层

> 作者：张彦龙

本目录存放 Netstars-KB（本 fork 的产品工作名）在 RAGFlow 官方部署之上的**部署期覆盖样例**。原则见 [docs/SECONDARY.md](../docs/SECONDARY.md)：不改核心源码、不改镜像内部，一切部署定制通过覆盖文件叠加。

## 目录内容

| 文件 | 说明 |
|---|---|
| `.env.example` | 环境变量样例（基于官方 `docker/.env` 裁剪，含 LLM / embedding 接入备注） |
| `docker-compose.override.example.yml` | Compose 覆盖样例（镜像标签、9380/9382 端口、MCP 启动参数，全部默认注释） |

## 使用方法（复制到 docker/ 目录旁）

1. 复制环境变量文件并按需修改：

```bash
cp overlay/.env.example docker/.env
```

2. 复制 compose 覆盖文件到官方 compose 文件旁（同目录），去掉 `.example` 后缀：

```bash
cp overlay/docker-compose.override.example.yml docker/docker-compose.override.yml
```

`docker compose` 会自动合并同目录下的 `docker-compose.yml` 与 `docker-compose.override.yml`，无需额外 `-f` 参数。

3. 启动：

```bash
cd docker
docker compose up -d
```

## 注意事项

- **不要提交密钥**：`docker/.env`、`docker/docker-compose.override.yml` 是实际部署文件，包含真实密码/API Key，一律不入库；仓库中只保留本目录下的 `.example` 样例。
- 镜像标签、compose 与 entrypoint 版本必须匹配，部署基线优先 `v0.26.4`，详见 [docs/SECONDARY.md](../docs/SECONDARY.md)。
- 品牌化备注：用户可见的产品名称为 Netstars-KB；浏览器标签页标题定义在 `web/index.html`。
- 插件/解析器桩：后续如需自定义解析器或插件，在本目录新建子目录存放桩代码，并在 `docs/SECONDARY.md` 登记所用扩展点。
