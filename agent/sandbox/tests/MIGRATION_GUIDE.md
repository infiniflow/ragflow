# Aliyun Code Interpreter Provider - Using the Official SDK

## Important Changes

### Official Resources
- **Code Interpreter API**: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
- **Official SDK**: https://github.com/Serverless-Devs/agentrun-sdk-python
- **SDK Documentation**: https://docs.agent.run

## Advantages of Using the Official SDK

Migrating from manual HTTP requests to the official SDK (`agentrun-sdk`) offers the following benefits:

### 1. **Automatic Signature Authentication**
- The SDK automatically handles Aliyun API signing (no need to manually implement `Authorization` headers)
- Supports multiple authentication methods: AccessKey, STS Token
- Automatically reads environment variables

### 2. **Simplified API**
```python
# Old implementation (manual HTTP requests)
response = requests.post(
    f"{DATA_ENDPOINT}/sandboxes/{sandbox_id}/execute",
    headers={"X-Acs-Parent-Id": account_id},
    json={"code": code, "language": "python"}
)

# New implementation (using SDK)
sandbox = CodeInterpreterSandbox(template_name="python-sandbox", config=config)
result = sandbox.context.execute(code="print('hello')")
```

### 3. **Better Error Handling**
- Structured exception types (`ServerError`)
- Automatic retry mechanism
- Detailed error messages

## Key Changes

### 1. File Renames

| Old Filename | New Filename | Description |
|---------|---------|------|
| `aliyun_opensandbox.py` | `aliyun_codeinterpreter.py` | Provider implementation |
| `test_aliyun_provider.py` | `test_aliyun_codeinterpreter.py` | Unit tests |
| `test_aliyun_integration.py` | `test_aliyun_codeinterpreter_integration.py` | Integration tests |

### 2. Configuration Field Changes

#### Old Configuration (OpenSandbox)
```json
{
  "access_key_id": "LTAI5t...",
  "access_key_secret": "...",
  "region": "cn-hangzhou",
  "workspace_id": "ws-xxxxx"
}
```

#### New Configuration (Code Interpreter)
```json
{
  "access_key_id": "LTAI5t...",
  "access_key_secret": "...",
  "account_id": "1234567890...",  // New: Aliyun primary account ID (required)
  "region": "cn-hangzhou",
  "template_name": "python-sandbox",  // New: sandbox template name
  "timeout": 30  // Max 30 seconds (hard limit)
}
```

### 3. Key Differences

| Feature | OpenSandbox | Code Interpreter |
|------|-------------|-----------------|
| **API Endpoint** | `opensandbox.{region}.aliyuncs.com` | `agentrun.{region}.aliyuncs.com` (control plane) |
| **API Version** | `2024-01-01` | `2025-09-10` |
| **Authentication** | AccessKey required | AccessKey + primary account ID required |
| **Request Headers** | Standard signature | Requires `X-Acs-Parent-Id` header |
| **Timeout Limit** | Configurable | **Max 30 seconds** (hard limit) |
| **Context** | Not supported | Supports context (Jupyter kernel) |

### 4. API Call Changes

#### Old Implementation (assumed OpenSandbox)
```python
# Single endpoint
API_ENDPOINT = "https://opensandbox.cn-hangzhou.aliyuncs.com"

# Simple request/response
response = requests.post(
    f"{API_ENDPOINT}/execute",
    json={"code": "print('hello')", "language": "python"}
)
```

#### New Implementation (Code Interpreter)
```python
# Control plane API - manage sandbox lifecycle
CONTROL_ENDPOINT = "https://agentrun.cn-hangzhou.aliyuncs.com/2025-09-10"

# Data plane API - execute code
DATA_ENDPOINT = "https://{account_id}.agentrun-data.cn-hangzhou.aliyuncs.com"

# Create sandbox (control plane)
response = requests.post(
    f"{CONTROL_ENDPOINT}/sandboxes",
    headers={"X-Acs-Parent-Id": account_id},
    json={"templateName": "python-sandbox"}
)

# Execute code (data plane)
response = requests.post(
    f"{DATA_ENDPOINT}/sandboxes/{sandbox_id}/execute",
    headers={"X-Acs-Parent-Id": account_id},
    json={"code": "print('hello')", "language": "python", "timeout": 30}
)
```

### 5. Migration Steps

#### Step 1: Update Configuration

If you were previously using `aliyun_opensandbox`:

**Old configuration**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_opensandbox"
}
```

**New configuration**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_codeinterpreter"
}
```

#### Step 2: Add the Required account_id

Get your primary account ID from the Aliyun console:
1. Log in to the [Aliyun Console](https://ram.console.aliyun.com/manage/ak)
2. Click on your avatar in the top-right corner
3. Copy the primary account ID (16-digit number)

#### Step 3: Update Environment Variables

```bash
# New required environment variable
export ALIYUN_ACCOUNT_ID="1234567890123456"

# Other environment variables remain unchanged
export ALIYUN_ACCESS_KEY_ID="LTAI5t..."
export ALIYUN_ACCESS_KEY_SECRET="..."
export ALIYUN_REGION="cn-hangzhou"
```

#### Step 4: Run Tests

```bash
# Unit tests (no real credentials required)
pytest agent/sandbox/tests/test_aliyun_codeinterpreter.py -v

# Integration tests (real credentials required)
pytest agent/sandbox/tests/test_aliyun_codeinterpreter_integration.py -v -m integration
```

## File Change Checklist

### ✅ Completed

- [x] Created `aliyun_codeinterpreter.py` - new provider implementation
- [x] Updated `sandbox_spec.md` - specification document
- [x] Updated `admin/services.py` - service manager
- [x] Updated `providers/__init__.py` - package exports
- [x] Created `test_aliyun_codeinterpreter.py` - unit tests
- [x] Created `test_aliyun_codeinterpreter_integration.py` - integration tests

### 📝 Optional Cleanup

If you want to remove the old OpenSandbox implementation:

```bash
# Remove old files (optional)
rm agent/sandbox/providers/aliyun_opensandbox.py
rm agent/sandbox/tests/test_aliyun_provider.py
rm agent/sandbox/tests/test_aliyun_integration.py
```

**Note**: Keeping the old files does not affect the new functionality; it just results in redundant code.

## API Reference

### Control Plane API (Sandbox Management)

| Endpoint | Method | Description |
|------|------|------|
| `/sandboxes` | POST | Create a sandbox instance |
| `/sandboxes/{id}/stop` | POST | Stop an instance |
| `/sandboxes/{id}` | DELETE | Delete an instance |
| `/templates` | GET | List templates |

### Data Plane API (Code Execution)

| Endpoint | Method | Description |
|------|------|------|
| `/sandboxes/{id}/execute` | POST | Execute code (simplified) |
| `/sandboxes/{id}/contexts` | POST | Create a context |
| `/sandboxes/{id}/contexts/{ctx_id}/execute` | POST | Execute within a context |
| `/sandboxes/{id}/health` | GET | Health check |
| `/sandboxes/{id}/files` | GET/POST | File read/write |
| `/sandboxes/{id}/processes/cmd` | POST | Execute shell command |

## FAQ

### Q: Why is account_id required?

**A**: The Code Interpreter API requires the `X-Acs-Parent-Id` (Aliyun primary account ID) header for authentication. This is a required parameter for the Aliyun Code Interpreter API.

### Q: Can the 30-second timeout limit be bypassed?

**A**: No. This is a **hard limit** of Aliyun Code Interpreter and cannot be bypassed through configuration or request parameters. If your code execution exceeds 30 seconds, consider:
1. Optimizing the code logic
2. Processing data in batches
3. Using contexts to maintain state

### Q: Can the old OpenSandbox configuration still be used?

**A**: No. OpenSandbox and Code Interpreter are two different services with incompatible APIs. You must migrate to the new configuration format.

### Q: How do I get the Aliyun primary account ID?

**A**:
1. Log in to the Aliyun console
2. Click on your avatar in the top-right corner
3. The primary account ID will be displayed in the popup

### Q: Will the migration affect existing functionality?

**A**:
- **Self-managed provider (self_managed)**: Not affected
- **E2B provider**: Not affected
- **Aliyun provider**: Configuration update and re-testing required

## Related Documentation

- [Official Documentation](https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter)
- [Sandbox Specification](../docs/develop/sandbox_spec.md)
- [Testing Guide](./README.md)
- [Quick Start](./QUICKSTART.md)

## Support

If you have any issues:
1. Review the official documentation
2. Verify the configuration is correct
3. Check the error messages in the test output
4. Contact the RAGFlow team
