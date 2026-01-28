# Aliyun Code Interpreter Provider - 使用官方 SDK

## 重要变更

### 官方资源
- **Code Interpreter API**: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
- **官方 SDK**: https://github.com/Serverless-Devs/agentrun-sdk-python
- **SDK 文档**: https://docs.agent.run

## 使用官方 SDK 的优势

从手动 HTTP 请求迁移到官方 SDK (`agentrun-sdk`) 有以下优势：

### 1. **自动签名认证**
- SDK 自动处理 Aliyun API 签名（无需手动实现 `Authorization` 头）
- 支持多种认证方式：AccessKey、STS Token
- 自动读取环境变量

### 2. **简化的 API**
```python
# 旧实现（手动 HTTP 请求）
response = requests.post(
    f"{DATA_ENDPOINT}/sandboxes/{sandbox_id}/execute",
    headers={"X-Acs-Parent-Id": account_id},
    json={"code": code, "language": "python"}
)

# 新实现（使用 SDK）
sandbox = CodeInterpreterSandbox(template_name="python-sandbox", config=config)
result = sandbox.context.execute(code="print('hello')")
```

### 3. **更好的错误处理**
- 结构化的异常类型 (`ServerError`)
- 自动重试机制
- 详细的错误信息

## 主要变更

### 1. 文件重命名

| 旧文件名 | 新文件名 | 说明 |
|---------|---------|------|
| `aliyun_opensandbox.py` | `aliyun_codeinterpreter.py` | 提供商实现 |
| `test_aliyun_provider.py` | `test_aliyun_codeinterpreter.py` | 单元测试 |
| `test_aliyun_integration.py` | `test_aliyun_codeinterpreter_integration.py` | 集成测试 |

### 2. 配置字段变更

#### 旧配置（OpenSandbox）
```json
{
  "access_key_id": "LTAI5t...",
  "access_key_secret": "...",
  "region": "cn-hangzhou",
  "workspace_id": "ws-xxxxx"
}
```

#### 新配置（Code Interpreter）
```json
{
  "access_key_id": "LTAI5t...",
  "access_key_secret": "...",
  "account_id": "1234567890...",  // 新增：阿里云主账号ID（必需）
  "region": "cn-hangzhou",
  "template_name": "python-sandbox",  // 新增：沙箱模板名称
  "timeout": 30  // 最大 30 秒（硬限制）
}
```

### 3. 关键差异

| 特性 | OpenSandbox | Code Interpreter |
|------|-------------|-----------------|
| **API 端点** | `opensandbox.{region}.aliyuncs.com` | `agentrun.{region}.aliyuncs.com` (控制面) |
| **API 版本** | `2024-01-01` | `2025-09-10` |
| **认证** | 需要 AccessKey | 需要 AccessKey + 主账号ID |
| **请求头** | 标准签名 | 需要 `X-Acs-Parent-Id` 头 |
| **超时限制** | 可配置 | **最大 30 秒**（硬限制） |
| **上下文** | 不支持 | 支持上下文（Jupyter kernel） |

### 4. API 调用方式变更

#### 旧实现（假设的 OpenSandbox）
```python
# 单一端点
API_ENDPOINT = "https://opensandbox.cn-hangzhou.aliyuncs.com"

# 简单的请求/响应
response = requests.post(
    f"{API_ENDPOINT}/execute",
    json={"code": "print('hello')", "language": "python"}
)
```

#### 新实现（Code Interpreter）
```python
# 控制面 API - 管理沙箱生命周期
CONTROL_ENDPOINT = "https://agentrun.cn-hangzhou.aliyuncs.com/2025-09-10"

# 数据面 API - 执行代码
DATA_ENDPOINT = "https://{account_id}.agentrun-data.cn-hangzhou.aliyuncs.com"

# 创建沙箱（控制面）
response = requests.post(
    f"{CONTROL_ENDPOINT}/sandboxes",
    headers={"X-Acs-Parent-Id": account_id},
    json={"templateName": "python-sandbox"}
)

# 执行代码（数据面）
response = requests.post(
    f"{DATA_ENDPOINT}/sandboxes/{sandbox_id}/execute",
    headers={"X-Acs-Parent-Id": account_id},
    json={"code": "print('hello')", "language": "python", "timeout": 30}
)
```

### 5. 迁移步骤

#### 步骤 1: 更新配置

如果您之前使用的是 `aliyun_opensandbox`：

**旧配置**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_opensandbox"
}
```

**新配置**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_codeinterpreter"
}
```

#### 步骤 2: 添加必需的 account_id

在 Aliyun 控制台右上角点击头像，获取主账号 ID：
1. 登录 [阿里云控制台](https://ram.console.aliyun.com/manage/ak)
2. 点击右上角头像
3. 复制主账号 ID（16 位数字）

#### 步骤 3: 更新环境变量

```bash
# 新增必需的环境变量
export ALIYUN_ACCOUNT_ID="1234567890123456"

# 其他环境变量保持不变
export ALIYUN_ACCESS_KEY_ID="LTAI5t..."
export ALIYUN_ACCESS_KEY_SECRET="..."
export ALIYUN_REGION="cn-hangzhou"
```

#### 步骤 4: 运行测试

```bash
# 单元测试（不需要真实凭据）
pytest agent/sandbox/tests/test_aliyun_codeinterpreter.py -v

# 集成测试（需要真实凭据）
pytest agent/sandbox/tests/test_aliyun_codeinterpreter_integration.py -v -m integration
```

## 文件变更清单

### ✅ 已完成

- [x] 创建 `aliyun_codeinterpreter.py` - 新的提供商实现
- [x] 更新 `sandbox_spec.md` - 规范文档
- [x] 更新 `admin/services.py` - 服务管理器
- [x] 更新 `providers/__init__.py` - 包导出
- [x] 创建 `test_aliyun_codeinterpreter.py` - 单元测试
- [x] 创建 `test_aliyun_codeinterpreter_integration.py` - 集成测试

### 📝 可选清理

如果您想删除旧的 OpenSandbox 实现：

```bash
# 删除旧文件（可选）
rm agent/sandbox/providers/aliyun_opensandbox.py
rm agent/sandbox/tests/test_aliyun_provider.py
rm agent/sandbox/tests/test_aliyun_integration.py
```

**注意**: 保留旧文件不会影响新功能，只是代码冗余。

## API 参考

### 控制面 API（沙箱管理）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/sandboxes` | POST | 创建沙箱实例 |
| `/sandboxes/{id}/stop` | POST | 停止实例 |
| `/sandboxes/{id}` | DELETE | 删除实例 |
| `/templates` | GET | 列出模板 |

### 数据面 API（代码执行）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/sandboxes/{id}/execute` | POST | 执行代码（简化版） |
| `/sandboxes/{id}/contexts` | POST | 创建上下文 |
| `/sandboxes/{id}/contexts/{ctx_id}/execute` | POST | 在上下文中执行 |
| `/sandboxes/{id}/health` | GET | 健康检查 |
| `/sandboxes/{id}/files` | GET/POST | 文件读写 |
| `/sandboxes/{id}/processes/cmd` | POST | 执行 Shell 命令 |

## 常见问题

### Q: 为什么要添加 account_id？

**A**: Code Interpreter API 需要在请求头中提供 `X-Acs-Parent-Id`（阿里云主账号ID）进行身份验证。这是 Aliyun Code Interpreter API 的必需参数。

### Q: 30 秒超时限制可以绕过吗？

**A**: 不可以。这是 Aliyun Code Interpreter 的**硬限制**，无法通过配置或请求参数绕过。如果代码执行时间超过 30 秒，请考虑：
1. 优化代码逻辑
2. 分批处理数据
3. 使用上下文保持状态

### Q: 旧的 OpenSandbox 配置还能用吗？

**A**: 不能。OpenSandbox 和 Code Interpreter 是两个不同的服务，API 不兼容。必须迁移到新的配置格式。

### Q: 如何获取阿里云主账号 ID？

**A**:
1. 登录阿里云控制台
2. 点击右上角的头像
3. 在弹出的信息中可以看到"主账号ID"

### Q: 迁移后会影响现有功能吗？

**A**:
- **自我管理提供商（self_managed）**: 不受影响
- **E2B 提供商**: 不受影响
- **Aliyun 提供商**: 需要更新配置并重新测试

## 相关文档

- [官方文档](https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter)
- [sandbox 规范](../docs/develop/sandbox_spec.md)
- [测试指南](./README.md)
- [快速开始](./QUICKSTART.md)

## 技术支持

如有问题，请：
1. 查看官方文档
2. 检查配置是否正确
3. 查看测试输出中的错误信息
4. 联系 RAGFlow 团队
