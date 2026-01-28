# Aliyun OpenSandbox Provider - 快速测试指南

## 测试说明

### 1. 单元测试（不需要真实凭据）

单元测试使用 mock，**不需要**真实的 Aliyun 凭据，可以随时运行。

```bash
# 运行 Aliyun 提供商的单元测试
pytest agent/sandbox/tests/test_aliyun_provider.py -v

# 预期输出：
# test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_provider_initialization PASSED
# test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_initialize_success PASSED
# ...
# ========================= 48 passed in 2.34s ==========================
```

### 2. 集成测试（需要真实凭据）

集成测试会调用真实的 Aliyun API，需要配置凭据。

#### 步骤 1: 配置环境变量

```bash
export ALIYUN_ACCESS_KEY_ID="LTAI5t..."  # 替换为真实的 Access Key ID
export ALIYUN_ACCESS_KEY_SECRET="..."     # 替换为真实的 Access Key Secret
export ALIYUN_REGION="cn-hangzhou"        # 可选，默认为 cn-hangzhou
```

#### 步骤 2: 运行集成测试

```bash
# 运行所有集成测试
pytest agent/sandbox/tests/test_aliyun_integration.py -v -m integration

# 运行特定测试
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check -v
```

#### 步骤 3: 预期输出

```
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_initialize_provider PASSED
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check PASSED
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code PASSED
...
========================== 10 passed in 15.67s ==========================
```

### 3. 测试场景

#### 基础功能测试

```bash
# 健康检查
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check -v

# 创建实例
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_create_python_instance -v

# 执行代码
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code -v

# 销毁实例
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_destroy_instance -v
```

#### 错误处理测试

```bash
# 代码执行错误
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code_with_error -v

# 超时处理
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code_timeout -v
```

#### 真实场景测试

```bash
# 数据处理工作流
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_data_processing_workflow -v

# 字符串操作
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_string_manipulation -v

# 多次执行
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_multiple_executions_same_instance -v
```

## 常见问题

### Q: 没有凭据怎么办？

**A:** 运行单元测试即可，不需要真实凭据：
```bash
pytest agent/sandbox/tests/test_aliyun_provider.py -v
```

### Q: 如何跳过集成测试？

**A:** 使用 pytest 标记跳过：
```bash
# 只运行单元测试，跳过集成测试
pytest agent/sandbox/tests/ -v -m "not integration"
```

### Q: 集成测试失败怎么办？

**A:** 检查以下几点：

1. **凭据是否正确**
   ```bash
   echo $ALIYUN_ACCESS_KEY_ID
   echo $ALIYUN_ACCESS_KEY_SECRET
   ```

2. **网络连接是否正常**
   ```bash
   curl -I https://opensandbox.cn-hangzhou.aliyuncs.com
   ```

3. **是否有 OpenSandbox 服务权限**
   - 登录阿里云控制台
   - 检查是否已开通 OpenSandbox 服务
   - 检查 AccessKey 权限

4. **查看详细错误信息**
   ```bash
   pytest agent/sandbox/tests/test_aliyun_integration.py -v -s
   ```

### Q: 测试超时怎么办？

**A:** 增加超时时间或检查网络：
```bash
# 使用更长的超时
pytest agent/sandbox/tests/test_aliyun_integration.py -v --timeout=60
```

## 测试命令速查表

| 命令 | 说明 | 需要凭据 |
|------|------|---------|
| `pytest agent/sandbox/tests/test_aliyun_provider.py -v` | 单元测试 | ❌ |
| `pytest agent/sandbox/tests/test_aliyun_integration.py -v` | 集成测试 | ✅ |
| `pytest agent/sandbox/tests/ -v -m "not integration"` | 仅单元测试 | ❌ |
| `pytest agent/sandbox/tests/ -v -m integration` | 仅集成测试 | ✅ |
| `pytest agent/sandbox/tests/ -v` | 所有测试 | 部分需要 |

## 获取 Aliyun 凭据

1. 访问 [阿里云控制台](https://ram.console.aliyun.com/manage/ak)
2. 创建 AccessKey
3. 保存 AccessKey ID 和 AccessKey Secret
4. 设置环境变量

⚠️ **安全提示：**
- 不要在代码中硬编码凭据
- 使用环境变量或配置文件
- 定期轮换 AccessKey
- 限制 AccessKey 权限

## 下一步

1. ✅ **运行单元测试** - 验证代码逻辑
2. 🔧 **配置凭据** - 设置环境变量
3. 🚀 **运行集成测试** - 测试真实 API
4. 📊 **查看结果** - 确保所有测试通过
5. 🎯 **集成到系统** - 使用 admin API 配置提供商

## 需要帮助？

- 查看 [完整文档](README.md)
- 检查 [sandbox 规范](../../../../../docs/develop/sandbox_spec.md)
- 联系 RAGFlow 团队
