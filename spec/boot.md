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
