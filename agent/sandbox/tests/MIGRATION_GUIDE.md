# Aliyun Code Interpreter Provider - Migration to Official SDK

## Important Changes

### Official Resources
- **Official SDK**: https://github.com/Serverless-Devs/agentrun-sdk-python
- **SDK Documentation**: https://docs.agent.run

## Benefits of Using the Official SDK

Migrating from manual HTTP requests to the official SDK (`agentrun-sdk`) provides the following advantages:

### 1. **Automatic Signature Authentication**
- SDK automatically handles Aliyun API signature (no manual `Authorization` header implementation)
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

## Major Changes

### 1. File Renaming

| Old Filename | New Filename | Description |
|--------------|--------------|-------------|
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
  "template_name": "python-sandbox",  // New: Sandbox template name
  "timeout": 30  // Maximum 30 seconds (hard limit)
}
```

### 3. Key Differences

| Feature | OpenSandbox | Code Interpreter |
|---------|-------------|-----------------|
| **API Endpoint** | `opensandbox.{region}.aliyuncs.com` | `agentrun.{region}.aliyuncs.com` (Control Plane) |
| **API Version** | `2024-01-01` | `2025-09-10` |
| **Authentication** | Requires AccessKey | Requires AccessKey + Account ID |
| **Request Header** | Standard signature | Requires `X-Acs-Parent-Id` header |
| **Timeout Limit** | Configurable | **Maximum 30 seconds** (hard limit) |
| **Context** | Not supported | Supports context (Jupyter kernel) |

### 4. API Call Changes

#### Old Implementation (Assumed OpenSandbox)
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
# Control Plane API - Manage sandbox lifecycle
CONTROL_ENDPOINT = "https://agentrun.cn-hangzhou.aliyuncs.com/2025-09-10"

# Data Plane API - Execute code
DATA_ENDPOINT = "https://{account_id}.agentrun-data.cn-hangzhou.aliyuncs.com"

# Create sandbox (Control Plane)
response = requests.post(
    f"{CONTROL_ENDPOINT}/sandboxes",
    headers={"X-Acs-Parent-Id": account_id},
    json={"templateName": "python-sandbox"}
)

# Execute code (Data Plane)
response = requests.post(
    f"{DATA_ENDPOINT}/sandboxes/{sandbox_id}/execute",
    headers={"X-Acs-Parent-Id": account_id},
    json={"code": "print('hello')", "language": "python", "timeout": 30}
)
```

### 5. Migration Steps

#### Step 1: Update Configuration

If you were previously using `aliyun_opensandbox`:

**Old Configuration**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_opensandbox"
}
```

**New Configuration**:
```json
{
  "name": "sandbox.provider_type",
  "value": "aliyun_codeinterpreter"
}
```

#### Step 2: Add Required account_id

Get the primary account ID from the Aliyun console:
1. Log in to [Aliyun Console](https://ram.console.aliyun.com/manage/ak)
2. Click the profile icon in the upper right corner
3. Copy the account ID (16 digits)

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
# Unit tests (don't require real credentials)
pytest agent/sandbox/tests/test_aliyun_codeinterpreter.py -v

# Integration tests (require real credentials)
pytest agent/sandbox/tests/test_aliyun_codeinterpreter_integration.py -v -m integration
```

## File Change Checklist

### Completed

- [x] Created `aliyun_codeinterpreter.py` - New provider implementation
- [x] Updated `sandbox_spec.md` - Specification document
- [x] Updated `admin/services.py` - Service manager
- [x] Updated `providers/__init__.py` - Package exports
- [x] Created `test_aliyun_codeinterpreter.py` - Unit tests
- [x] Created `test_aliyun_codeinterpreter_integration.py` - Integration tests

### Optional Cleanup

If you want to remove the old OpenSandbox implementation:

```bash
# Delete old files (optional)
rm agent/sandbox/providers/aliyun_opensandbox.py
rm agent/sandbox/tests/test_aliyun_provider.py
rm agent/sandbox/tests/test_aliyun_integration.py
```

**Note**: Keeping old files won't affect new functionality, it's just code redundancy.

## API Reference

### Control Plane API (Sandbox Management)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/sandboxes` | POST | Create sandbox instance |
| `/sandboxes/{id}/stop` | POST | Stop instance |
| `/sandboxes/{id}` | DELETE | Delete instance |
| `/templates` | GET | List templates |

### Data Plane API (Code Execution)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/sandboxes/{id}/execute` | POST | Execute code (simplified) |
| `/sandboxes/{id}/contexts` | POST | Create context |
| `/sandboxes/{id}/contexts/{ctx_id}/execute` | POST | Execute in context |
| `/sandboxes/{id}/health` | GET | Health check |
| `/sandboxes/{id}/files` | GET/POST | File read/write |
| `/sandboxes/{id}/processes/cmd` | POST | Execute shell command |

## Frequently Asked Questions

### Q: Why do we need to add account_id?

**A**: The Code Interpreter API requires `X-Acs-Parent-Id` (Aliyun primary account ID) in the request header for authentication. This is a required parameter for the Aliyun Code Interpreter API.

### Q: Can we bypass the 30-second timeout limit?

**A**: No. This is a **hard limit** of Aliyun Code Interpreter and cannot be bypassed through configuration or request parameters. If your code execution exceeds 30 seconds, consider:
1. Optimize code logic
2. Process data in batches
3. Use context to maintain state

### Q: Can we still use the old OpenSandbox configuration?

**A**: No. OpenSandbox and Code Interpreter are two different services with incompatible APIs. You must migrate to the new configuration format.

### Q: How to get the Aliyun primary account ID?

**A**:
1. Log in to Aliyun Console
2. Click the profile icon in the upper right corner
3. The account ID will be displayed in the popup

### Q: Will migration affect existing functionality?

**A**:
- **Self-managed provider**: Not affected
- **E2B provider**: Not affected
- **Aliyun provider**: Need to update configuration and re-test

## Related Documentation

- [Official Documentation](https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter)
- [Sandbox Specification](../docs/develop/sandbox_spec.md)
- [Testing Guide](./README.md)
- [Quick Start](./QUICKSTART.md)

## Support

If you have questions:
1. Check the official documentation
2. Verify the configuration is correct
3. Check error messages in test output
4. Contact the RAGFlow team
