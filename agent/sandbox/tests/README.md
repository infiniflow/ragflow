# Sandbox Provider Tests

This directory contains tests for the RAGFlow sandbox provider system.

## Test Structure

```
tests/
├── pytest.ini                           # Pytest configuration
├── test_providers.py                    # Unit tests for all providers (mocked)
├── test_aliyun_provider.py              # Unit tests for Aliyun provider (mocked)
├── test_aliyun_integration.py           # Integration tests for Aliyun (real API)
└── sandbox_security_tests_full.py      # Security tests for self-managed provider
```

## Test Types

### 1. Unit Tests (No Credentials Required)

Unit tests use mocks and don't require any external services or credentials.

**Files:**
- `test_providers.py` - Tests for base provider interface and manager
- `test_aliyun_provider.py` - Tests for Aliyun provider with mocked API calls

**Run unit tests:**
```bash
# Run all unit tests
pytest agent/sandbox/tests/test_providers.py -v
pytest agent/sandbox/tests/test_aliyun_provider.py -v

# Run specific test
pytest agent/sandbox/tests/test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_initialize_success -v

# Run all unit tests (skip integration)
pytest agent/sandbox/tests/ -v -m "not integration"
```

### 2. Integration Tests (Real Credentials Required)

Integration tests make real API calls to Aliyun OpenSandbox service.

**Files:**
- `test_aliyun_integration.py` - Tests with real Aliyun API calls

**Setup environment variables:**
```bash
export ALIYUN_ACCESS_KEY_ID="LTAI5t..."
export ALIYUN_ACCESS_KEY_SECRET="..."
export ALIYUN_REGION="cn-hangzhou"  # Optional, defaults to cn-hangzhou
export ALIYUN_WORKSPACE_ID="ws-..."  # Optional
```

**Run integration tests:**
```bash
# Run only integration tests
pytest agent/sandbox/tests/test_aliyun_integration.py -v -m integration

# Run all tests including integration
pytest agent/sandbox/tests/ -v

# Run specific integration test
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check -v
```

### 3. Security Tests

Security tests validate the security features of the self-managed sandbox provider.

**Files:**
- `sandbox_security_tests_full.py` - Comprehensive security tests

**Run security tests:**
```bash
# Run all security tests
pytest agent/sandbox/tests/sandbox_security_tests_full.py -v

# Run specific security test
pytest agent/sandbox/tests/sandbox_security_tests_full.py -k "test_dangerous_imports" -v
```

## Test Commands

### Quick Test Commands

```bash
# Run all sandbox tests (unit only, fast)
pytest agent/sandbox/tests/ -v -m "not integration" --tb=short

# Run tests with coverage
pytest agent/sandbox/tests/ -v --cov=agent.sandbox --cov-report=term-missing -m "not integration"

# Run tests and stop on first failure
pytest agent/sandbox/tests/ -v -x -m "not integration"

# Run tests in parallel (requires pytest-xdist)
pytest agent/sandbox/tests/ -v -n auto -m "not integration"
```

### Aliyun Provider Testing

```bash
# 1. Run unit tests (no credentials needed)
pytest agent/sandbox/tests/test_aliyun_provider.py -v

# 2. Set up credentials for integration tests
export ALIYUN_ACCESS_KEY_ID="your-key-id"
export ALIYUN_ACCESS_KEY_SECRET="your-secret"
export ALIYUN_REGION="cn-hangzhou"

# 3. Run integration tests (makes real API calls)
pytest agent/sandbox/tests/test_aliyun_integration.py -v

# 4. Test specific scenarios
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code -v
pytest agent/sandbox/tests/test_aliyun_integration.py::TestAliyunRealWorldScenarios -v
```

## Understanding Test Results

### Unit Test Output

```
agent/sandbox/tests/test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_initialize_success PASSED
agent/sandbox/tests/test_aliyun_provider.py::TestAliyunOpenSandboxProvider::test_create_instance_python PASSED
...
========================== 48 passed in 2.34s ===========================
```

### Integration Test Output

```
agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check PASSED
agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_create_python_instance PASSED
agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_execute_python_code PASSED
...
========================== 10 passed in 15.67s ===========================
```

**Note:** Integration tests will be skipped if credentials are not set:
```
agent/sandbox/tests/test_aliyun_integration.py::TestAliyunOpenSandboxIntegration::test_health_check SKIPPED
...
========================== 48 skipped, 10 passed in 0.12s ===========================
```

## Troubleshooting

### Integration Tests Fail

1. **Check credentials:**
   ```bash
   echo $ALIYUN_ACCESS_KEY_ID
   echo $ALIYUN_ACCESS_KEY_SECRET
   ```

2. **Check network connectivity:**
   ```bash
   curl -I https://opensandbox.cn-hangzhou.aliyuncs.com
   ```

3. **Verify permissions:**
   - Make sure your Aliyun account has OpenSandbox service enabled
   - Check that your AccessKey has the required permissions

4. **Check region:**
   - Verify the region is correct for your account
   - Try different regions: cn-hangzhou, cn-beijing, cn-shanghai, etc.

### Tests Timeout

If tests timeout, increase the timeout in the test configuration or run with a longer timeout:
```bash
pytest agent/sandbox/tests/test_aliyun_integration.py -v --timeout=60
```

### Mock Tests Fail

If unit tests fail, it's likely a code issue, not a credentials issue:
1. Check the test error message
2. Review the code changes
3. Run with verbose output: `pytest -vv`

## Contributing

When adding new providers:

1. **Create unit tests** in `test_{provider}_provider.py` with mocks
2. **Create integration tests** in `test_{provider}_integration.py` with real API calls
3. **Add markers** to distinguish test types
4. **Update this README** with provider-specific testing instructions

Example:
```python
@pytest.mark.integration
def test_new_provider_real_api():
    """Test with real API calls."""
    # Your test here
```

## Continuous Integration

In CI/CD pipelines:

```yaml
# Run unit tests only (fast, no credentials)
pytest agent/sandbox/tests/ -v -m "not integration"

# Run integration tests if credentials available
if [ -n "$ALIYUN_ACCESS_KEY_ID" ]; then
    pytest agent/sandbox/tests/test_aliyun_integration.py -v -m integration
fi
```
