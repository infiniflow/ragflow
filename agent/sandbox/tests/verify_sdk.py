#!/usr/bin/env python3
"""
Quick verification script for Aliyun Code Interpreter provider using official SDK.
"""

import importlib.util
import sys

sys.path.insert(0, ".")

print("=" * 60)
print("Aliyun Code Interpreter Provider - SDK Verification")
print("=" * 60)

# Test 1: Import provider
print("\n[1/5] Testing provider import...")
try:
    from agent.sandbox.providers.aliyun_codeinterpreter import AliyunCodeInterpreterProvider

    print("✓ Provider imported successfully")
except ImportError as e:
    print(f"✗ Import failed: {e}")
    sys.exit(1)

# Test 2: Check provider class
print("\n[2/5] Testing provider class...")
provider = AliyunCodeInterpreterProvider()
assert hasattr(provider, "initialize")
assert hasattr(provider, "create_instance")
assert hasattr(provider, "execute_code")
assert hasattr(provider, "destroy_instance")
assert hasattr(provider, "health_check")
print("✓ Provider has all required methods")

# Test 3: Check SDK imports
print("\n[3/5] Testing SDK imports...")
try:
    # Check if agentrun SDK is available using importlib
    if (
        importlib.util.find_spec("agentrun.sandbox") is None
        or importlib.util.find_spec("agentrun.utils.config") is None
        or importlib.util.find_spec("agentrun.utils.exception") is None
    ):
        raise ImportError("agentrun SDK not found")

    # Verify imports work (assign to _ to indicate they're intentionally unused)
    from agentrun.sandbox import CodeInterpreterSandbox, TemplateType, CodeLanguage
    from agentrun.utils.config import Config
    from agentrun.utils.exception import ServerError
    _ = (CodeInterpreterSandbox, TemplateType, CodeLanguage, Config, ServerError)

    print("✓ SDK modules imported successfully")
except ImportError as e:
    print(f"✗ SDK import failed: {e}")
    sys.exit(1)

# Test 4: Check config schema
print("\n[4/5] Testing configuration schema...")
schema = AliyunCodeInterpreterProvider.get_config_schema()
required_fields = ["access_key_id", "access_key_secret", "account_id"]
for field in required_fields:
    assert field in schema
    assert schema[field]["required"] is True
print(f"✓ All required fields present: {', '.join(required_fields)}")

# Test 5: Check supported languages
print("\n[5/5] Testing supported languages...")
languages = provider.get_supported_languages()
assert "python" in languages
assert "javascript" in languages
print(f"✓ Supported languages: {', '.join(languages)}")

print("\n" + "=" * 60)
print("All verification tests passed! ✓")
print("=" * 60)
print("\nNote: This provider now uses the official agentrun-sdk.")
print("SDK Documentation: https://github.com/Serverless-Devs/agentrun-sdk-python")
print("API Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter")
