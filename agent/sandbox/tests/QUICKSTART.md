# Aliyun OpenSandbox Provider - Quick Test Guide

## Test Overview

### 1. Unit Tests (No Credentials Required)

Unit tests use mocks and do **not** require real Aliyun credentials; they can be run at any time.

```bash
# Run unit tests for the Aliyun provider
pytest agent/sandbox/tests/test_aliyun_provider.py -v

# Expected output:
# test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_provider_initialization PASSED
# test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_initialize_success PASSED
# ...
# ========================= 48 passed in 2.34s ==========================
```

### 2. Integration Tests (Real Credentials Required)

Integration tests call the real Aliyun API and require credentials to be configured.

#### Step 1: Configure Environment Variables

```bash
export ALIYUN_ACCESS_KEY_ID="LTAI5t..."  # Replace with your real Access Key ID
export ALIYUN_ACCESS_KEY_SECRET="..."     # Replace with your real Access Key Secret
export ALIYUN_REGION="cn-hangzhou"        # Optional, defaults to cn-hangzhou
```

#### Step 2: Run Integration Tests

```bash
# Run all integration tests
pytest agent/sandbox/tests/test_aliyun_integration.py -v -m integration

# Run a specific test
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check -v
```

#### Step 3: Expected Output

```
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_initialize_provider PASSED
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check PASSED
test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code PASSED
...
========================== 10 passed in 15.67s ==========================
```

### 3. Test Scenarios

#### Basic Functionality Tests

```bash
# Health check
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check -v

# Create instance
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_create_python_instance -v

# Execute code
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code -v

# Destroy instance
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_destroy_instance -v
```

#### Error Handling Tests

```bash
# Code execution error
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code_with_error -v

# Timeout handling
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code_timeout -v
```

#### Real-World Scenario Tests

```bash
# Data processing workflow
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_data_processing_workflow -v

# String manipulation
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_string_manipulation -v

# Multiple executions
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios::test_multiple_executions_same_instance -v
```

## FAQ

### Q: What if I don't have credentials?

**A:** Just run the unit tests — no real credentials needed:
```bash
pytest agent/sandbox/tests/test_aliyun_provider.py -v
```

### Q: How do I skip integration tests?

**A:** Use pytest markers to skip them:
```bash
# Run only unit tests, skip integration tests
pytest agent/sandbox/tests/ -v -m "not integration"
```

### Q: What should I do if integration tests fail?

**A:** Check the following:

1. **Are the credentials correct?**
   ```bash
   echo $ALIYUN_ACCESS_KEY_ID
   echo $ALIYUN_ACCESS_KEY_SECRET
   ```

2. **Is the network connection working?**
   ```bash
   curl -I https://opensandbox.cn-hangzhou.aliyuncs.com
   ```

3. **Do you have OpenSandbox service permissions?**
   - Log in to the Aliyun console
   - Check if the OpenSandbox service is enabled
   - Verify AccessKey permissions

4. **View detailed error messages:**
   ```bash
   pytest agent/sandbox/tests/test_aliyun_integration.py -v -s
   ```

### Q: What should I do if tests time out?

**A:** Increase the timeout or check network connectivity:
```bash
# Use a longer timeout
pytest agent/sandbox/tests/test_aliyun_integration.py -v --timeout=60
```

## Quick Reference: Test Commands

| Command | Description | Credentials Required |
|------|------|---------|
| `pytest agent/sandbox/tests/test_aliyun_provider.py -v` | Unit tests | ❌ |
| `pytest agent/sandbox/tests/test_aliyun_integration.py -v` | Integration tests | ✅ |
| `pytest agent/sandbox/tests/ -v -m "not integration"` | Unit tests only | ❌ |
| `pytest agent/sandbox/tests/ -v -m integration` | Integration tests only | ✅ |
| `pytest agent/sandbox/tests/ -v` | All tests | Partially required |

## Getting Aliyun Credentials

1. Visit the [Aliyun Console](https://ram.console.aliyun.com/manage/ak)
2. Create an AccessKey
3. Save your AccessKey ID and AccessKey Secret
4. Set the environment variables

⚠️ **Security Tips:**
- Do not hardcode credentials in your code
- Use environment variables or configuration files
- Rotate AccessKeys regularly
- Restrict AccessKey permissions

## Next Steps

1. ✅ **Run unit tests** - Verify code logic
2. 🔧 **Configure credentials** - Set environment variables
3. 🚀 **Run integration tests** - Test the real API
4. 📊 **Review results** - Ensure all tests pass
5. 🎯 **Integrate into your system** - Configure the provider via the admin API

## Need Help?

- See the [full documentation](README.md)
- Check the [sandbox specification](../../../../../docs/develop/sandbox_spec.md)
- Contact the RAGFlow team
