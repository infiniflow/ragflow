# 项目中转站说明 (Proxy Repository Info)

## 项目定位
当前本地项目 (`~/Developer/element_workspace/ragflow`) 作为一个**中转站项目**。

## 工作流流转说明
1. **获取源**: 代码最初来源于 GitHub 上 fork 的开源项目。
2. **本地化加工**: 在本仓库进行代码的二次开发、本地化适配、安全清理或自定义修改。
3. **推送到 Gitee**: 加工完成后的代码将被推送到私有或国内的 Gitee 仓库 (`git@gitee.com:GFCM/ragflow.git`)。
4. **服务器部署**: 生产/测试服务器将直接从 Gitee 仓库中拉取和部署代码，以保证国内网络环境下的拉取速度和数据安全性。

---
*注：请在向 Gitee 推送前确保敏感信息（如 API Keys、云服务密码等）已被正确清理或替换为占位符。*

---

## 同步 Skill

本中转站的 upstream 同步流程已封装为 Claude Code skill，**调用此 skill 即驱动整条 sync 流程**：

- Skill 文件: `~/.claude/skills/sync-to-gitee.md`
- 触发关键词: "sync upstream" / "sync to gitee" / "同步到 gitee" / "拉取 upstream"
- 配套文档:
  - 长期规范: `spec/branch_sync_playbook.md`（从历史 gitee commit `3d00f5005` 取回，本地重建）
  - 历史 sync 记录: `spec/branch_sync_logs/YYYY-MM-DD/merge_log.md`
  - 入口: 本文件 `spec/boot.md`

调用 skill 后，流程将自动覆盖 10 步：
1. preflight（检查 working tree、3 个 remote、commit 差距）
2. 选择 base + 创建 worktree（避免污染原 `ragflow0251/gitee-export`）
3. `git merge upstream/main --no-ff`（不 rebase）
4. 3U 关键文件冲突解决（`docker/.env`、`conf/service_conf.yaml`、`rag/app/naive.py` 等）
5. 重新应用 `spec/boot.md` 本地化
6. **敏感信息扫描**（硬编码 API key / secret 规则见 skill 文档）
7. 写 `spec/branch_sync_logs/YYYY-MM-DD/merge_log.md`
8. 提交 `chore: add merge log for ...` commit
9. 本地验证
10. **人工 gate**: 询问用户是否 `git push gitee HEAD:master`，未经明确 yes 不推送

任何 `git push gitee` 之前必须有人工确认 — 不可绕过。

