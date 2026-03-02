# RAGFlow Sandbox multi-provider architecture - design specification

## 1. Overview

The goal of this design specification is to enable RAGFlow to support multiple Sandbox deployment modes:

- Self-Managed: On-premise deployment using Daytona/Docker (current implementation)
- SaaS providers: Cloud-based sandbox services (Aliyun Code Interpreter, E2B)

### Key requirements

- Provider-agnostic interface for sandbox operations
- Admin-configurable provider settings with dynamic schema
- Multi-tenant isolation (1:1 session-to-sandbox mapping)
- Graceful fallback and error handling
- Unified monitoring and observability

## Architecture

### Provider abstraction layer

Defines a unified `SandboxProvider` interface, and is located at `agent/sandbox/providers/`.

```python
# agent/sandbox/providers/base.py
from abc import ABC, abstractmethod
from typing import Dict, Any, Optional
from dataclasses import dataclass

@dataclass
class SandboxInstance:
    instance_id: str
    provider: str
    status: str  # running, stopped, error
    metadata: Dict[str, Any]

@dataclass
class ExecutionResult:
    stdout: str
    stderr: str
    exit_code: int
    execution_time: float
    metadata: Dict[str, Any]

class SandboxProvider(ABC):
    """Base interface for all sandbox providers"""

    @abstractmethod
    def initialize(self, config: Dict[str, Any]) -> bool:
        """Initialize provider with configuration"""
        pass

    @abstractmethod
    def create_instance(self, template: str = "python") -> SandboxInstance:
        """Create a new sandbox instance"""
        pass

    @abstractmethod
    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10
    ) -> ExecutionResult:
        """Execute code in the sandbox"""
        pass

    @abstractmethod
    def destroy_instance(self, instance_id: str) -> bool:
        """Destroy a sandbox instance"""
        pass

    @abstractmethod
    def health_check(self) -> bool:
        """Check if provider is healthy"""
        pass

    @abstractmethod
    def get_supported_languages(self) -> list[str]:
        """Get list of supported programming languages"""
        pass

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """
        Return configuration schema for this provider.

        Returns a dictionary mapping field names to their schema definitions,
        including type, required status, validation rules, labels, and descriptions.
        """
        pass

    def validate_config(self, config: Dict[str, Any]) -> tuple[bool, Optional[str]]:
        """
        Validate provider-specific configuration.

        This method allows providers to implement custom validation logic beyond
        the basic schema validation. Override this method to add provider-specific
        checks like URL format validation, API key format validation, etc.

        Args:
            config: Configuration dictionary to validate

        Returns:
            Tuple of (is_valid, error_message):
                - is_valid: True if configuration is valid, False otherwise
                - error_message: Error message if invalid, None if valid
        """
        # Default implementation: no custom validation
        return True, None
```

### Provider implementations

#### Self-managed provider

Wraps the existing executor_manager implementation. The implementation file is located at `agent/sandbox/providers/self_managed.py`.

**Prerequisites**:

- gVisor (runsc): Required for secure container isolation. Install with:
  ```bash
  go install gvisor.dev/gvisor/runsc@latest
  sudo cp ~/go/bin/runsc /usr/local/bin/
  runsc --version
  ```
  Or download from: https://github.com/google/gvisor/releases
- Docker: Docker runtime with gVisor support.
- Base Images: Pull sandbox base images:
  ```bash
  docker pull infiniflow/sandbox-base-python:latest
  docker pull infiniflow/sandbox-base-nodejs:latest
  ```

**Configuration**: Docker API endpoint, pool size, resource limits:

- `endpoint`: HTTP endpoint (default: "http://localhost:9385")
- `timeout`: Request timeout in seconds (default: 30)
- `max_retries`: Maximum retry attempts (default: 3)
- `pool_size`: Container pool size (default: 10)

**Languages**: 
- Python
- Node.js
- JavaScript

**Security**: 
- gVisor (runsc runtime)
- seccomp
- read-only filesystem
- memory limits

**Advantages**:
- Low latency (&lt;90ms), data privacy, full control
- No per-execution costs
- Supports `arguments` parameter for passing data to `main()` function

**Limitations**:
- Operational overhead, finite resources
- Requires gVisor installation for security
- Pool exhaustion causes "Container pool is busy" errors

**Common issues**:
- `"Container pool is busy"`: Increase `SANDBOX_EXECUTOR_MANAGER_POOL_SIZE` (default: 1 in .env, should be 5+)
- `Container creation fails`: Ensure gVisor is installed and accessible at `/usr/local/bin/runsc`

#### 2.2.2 Aliyun code interpreter provider

**File**: `agent/sandbox/providers/aliyun_codeinterpreter.py`

SaaS integration with Aliyun Function Compute Code Interpreter service using the official agentrun-sdk.

**Official Resources**:
- API Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
- Official SDK: https://github.com/Serverless-Devs/agentrun-sdk-python
- SDK Docs: https://docs.agent.run

**Implementation**:
- Uses official `agentrun-sdk` package
- SDK handles authentication (AccessKey signature) automatically
- Supports environment variable configuration
- Structured error handling with `ServerError` exceptions

**Configuration**:
- `access_key_id`: Aliyun AccessKey ID
- `access_key_secret`: Aliyun AccessKey Secret
- `account_id`: Aliyun primary account ID - Required for API calls
- `region`: Region (cn-hangzhou, cn-beijing, cn-shanghai, cn-shenzhen, cn-guangzhou)
- `template_name`: Optional sandbox template name for pre-configured environments
- `timeout`: Execution timeout (max 30 seconds - hard limit)

**Languages**: Python, JavaScript

**Security**: Serverless microVM isolation, 30-second hard timeout limit

**Advantages**:
- Official SDK with automatic signature handling
- Unlimited scalability, no maintenance
- China region support with low latency
- Built-in file system management
- Support for execution contexts (Jupyter kernel)
- Context-based execution for state persistence

**Limitations**:
- Network dependency
- 30-second execution time limit (hard limit)
- Pay-as-you-go costs
- Requires Aliyun primary account ID for API calls

**Setup instructions - Creating a RAM user with minimal privileges**:

⚠️ **Security warning**: Never use your Aliyun primary account (root account) AccessKey for SDK operations. Primary accounts have full resource permissions, and leaked credentials pose significant security risks.

**Step 1: Create a RAM user**

1. Log in to [RAM Console](https://ram.console.aliyun.com/)
2. Navigate to **People** → **Users**
3. Click **Create User**
4. Configure the user:
   - **Username**: e.g., `ragflow-sandbox-user`
   - **Display Name**: e.g., `RAGFlow Sandbox Service Account`
   - **Access Mode**: Check ✅ **OpenAPI/Programmatic Access** (this creates an AccessKey)
   - **Console Login**: Optional (not needed for SDK-only access)
5. Click **OK** and save the AccessKey ID and Secret immediately (displayed only once!)

**Step 2: Create a custom authorization policy**

Navigate to **Permissions** → **Policies** → **Create Policy** → **Custom Policy** → **Configuration Script (JSON)**

Choose one of the following policy options based on your security requirements:

**Option A: Minimal privilege policy (Recommended)**

Grants only the permissions required by the AgentRun SDK:

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "agentrun:CreateTemplate",
        "agentrun:GetTemplate",
        "agentrun:UpdateTemplate",
        "agentrun:DeleteTemplate",
        "agentrun:ListTemplates",
        "agentrun:CreateSandbox",
        "agentrun:GetSandbox",
        "agentrun:DeleteSandbox",
        "agentrun:StopSandbox",
        "agentrun:ListSandboxes",
        "agentrun:CreateContext",
        "agentrun:ExecuteCode",
        "agentrun:DeleteContext",
        "agentrun:ListContexts",
        "agentrun:CreateFile",
        "agentrun:GetFile",
        "agentrun:DeleteFile",
        "agentrun:ListFiles",
        "agentrun:CreateProcess",
        "agentrun:GetProcess",
        "agentrun:KillProcess",
        "agentrun:ListProcesses",
        "agentrun:CreateRecording",
        "agentrun:GetRecording",
        "agentrun:DeleteRecording",
        "agentrun:ListRecordings",
        "agentrun:CheckHealth"
      ],
      "Resource": [
        "acs:agentrun:*:{account_id}:template/*",
        "acs:agentrun:*:{account_id}:sandbox/*"
      ]
    }
  ]
}
```

> Replace `{account_id}` with your Aliyun primary account ID

**Option B: Resource-Level privilege control (most secure)**

Limits access to specific resource prefixes:

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "agentrun:CreateTemplate",
        "agentrun:GetTemplate",
        "agentrun:UpdateTemplate",
        "agentrun:DeleteTemplate",
        "agentrun:ListTemplates"
      ],
      "Resource": "acs:agentrun:*:{account_id}:template/ragflow-*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "agentrun:CreateSandbox",
        "agentrun:GetSandbox",
        "agentrun:DeleteSandbox",
        "agentrun:StopSandbox",
        "agentrun:ListSandboxes",
        "agentrun:CheckHealth"
      ],
      "Resource": "acs:agentrun:*:{account_id}:sandbox/*"
    },
    {
      "Effect": "Allow",
      "Action": ["agentrun:*"],
      "Resource": "acs:agentrun:*:{account_id}:sandbox/*/context/*"
    },
    {
      "Effect": "Allow",
      "Action": ["agentrun:*"],
      "Resource": "acs:agentrun:*:{account_id}:sandbox/*/file/*"
    },
    {
      "Effect": "Allow",
      "Action": ["agentrun:*"],
      "Resource": "acs:agentrun:*:{account_id}:sandbox/*/process/*"
    },
    {
      "Effect": "Allow",
      "Action": ["agentrun:*"],
      "Resource": "acs:agentrun:*:{account_id}:sandbox/*/recording/*"
    }
  ]
}
```

> This limits template creation to only those prefixed with `ragflow-*`

**Option C: Full access (not recommended for production)**

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "agentrun:*",
      "Resource": "*"
    }
  ]
}
```

**Step 3: Authorize the RAM user**

1. Return to **Users** list
2. Find the user you just created (e.g., `ragflow-sandbox-user`)
3. Click **Add Permissions** in the Actions column
4. In the **Custom Policy** tab, select the policy you created in Step 2
5. Click **OK**

**Step 4: Configure RAGFlow with the RAM User credentials**

After creating the RAM user and obtaining the AccessKey, configure it in RAGFlow's admin settings or environment variables:

```bash
# Method 1: Environment variables (for development/testing)
export AGENTRUN_ACCESS_KEY_ID="LTAI5t..."  # RAM user's AccessKey ID
export AGENTRUN_ACCESS_KEY_SECRET="xxx..."  # RAM user's AccessKey Secret
export AGENTRUN_ACCOUNT_ID="123456789..."  # Your primary account ID
export AGENTRUN_REGION="cn-hangzhou"
```

Or via Admin UI (recommended for production):

1. Navigate to **Admin Settings** → **Sandbox Providers**
2. Select **Aliyun Code Interpreter** provider
3. Fill in the configuration:
   - `access_key_id`: RAM user's AccessKey ID
   - `access_key_secret`: RAM user's AccessKey Secret
   - `account_id`: Your primary account ID
   - `region`: e.g., `cn-hangzhou`

**Step 5: Verify permissions**

Test if the RAM user permissions are correctly configured:

```python
from agentrun.sandbox import Sandbox, TemplateInput, TemplateType

try:
    # Test template creation
    template = Sandbox.create_template(
        input=TemplateInput(
            template_name="ragflow-permission-test",
            template_type=TemplateType.CODE_INTERPRETER
        )
    )
    print("✅ RAM user permissions are correctly configured")
except Exception as e:
    print(f"❌ Permission test failed: {e}")
finally:
    # Cleanup test resources
    try:
        Sandbox.delete_template("ragflow-permission-test")
    except:
        pass
```

**Security best practices**:

- **Always use RAM user AccessKeys**, never primary account AccessKeys.
- **Follow the principle of least privilege** - grant only necessary permissions.
- **Rotate AccessKeys regularly** - recommend every 3-6 months.
- **Enable MFA** - enable multi-factor authentication for RAM users.
- **Use secure storage** - store credentials in environment variables or secret management services, never hardcode in code.
- **Restrict IP access** - add IP whitelist policies for RAM users if needed.
- **Monitor access logs** - regularly check RAM user access logs in CloudTrail.

**References**:

- [Aliyun RAM Documentation](https://help.aliyun.com/product/28625.html)
- [RAM Policy Language](https://help.aliyun.com/document_detail/100676.html)
- [AgentRun Official Documentation](https://docs.agent.run)
- [AgentRun SDK GitHub](https://github.com/Serverless-Devs/agentrun-sdk-python)

#### E2B provider

The file is located at `agent/sandbox/providers/e2b.py`.

SaaS integration with E2B Cloud.
- **Configuration**: api_key, region (us/eu)
- **Languages**: Python, JavaScript, Go, Bash, etc.
- **Security**: Firecracker microVMs
- **Advantages**: Global CDN, fast startup, multiple language support
- **Limitations**: International network latency for China users

### Provider management

The file is located at  `agent/sandbox/providers/manager.py`.

Since we only use one active provider at a time (configured globally), the provider management is simplified:

```python
class ProviderManager:
    """Manages the currently active sandbox provider"""

    def __init__(self):
        self.current_provider: Optional[SandboxProvider] = None
        self.current_provider_name: Optional[str] = None

    def set_provider(self, name: str, provider: SandboxProvider):
        """Set the active provider"""
        self.current_provider = provider
        self.current_provider_name = name

    def get_provider(self) -> Optional[SandboxProvider]:
        """Get the active provider"""
        return self.current_provider

    def get_provider_name(self) -> Optional[str]:
        """Get the active provider name"""
        return self.current_provider_name
```

**Rationale**: With global configuration, there's only one active provider at a time. The provider manager simply holds a reference to the currently active provider, making it a thin wrapper rather than a complex multi-provider manager.

## Admin configuration

### Database Schema

Use the existing **SystemSettings** table for global sandbox configuration:

```python
# In api/db/db_models.py

class SystemSettings(DataBaseModel):
    name = CharField(max_length=128, primary_key=True)
    source = CharField(max_length=32, null=False, index=False)
    data_type = CharField(max_length=32, null=False, index=False)
    value = CharField(max_length=1024, null=False, index=False)
```

**Rationale**: Sandbox manager is a **system-level service** shared by all tenants:
- No per-tenant configuration needed (unlike LLM providers where each tenant has their own API keys)
- Global settings like system email, DOC_ENGINE, etc.
- Managed by administrators only
- Leverages existing `SettingsMgr` in admin interface

**Storage Strategy**: Each provider's configuration stored as a **single JSON object**:
- `sandbox.provider_type` - Active provider selection ("self_managed", "aliyun_codeinterpreter", "e2b")
- `sandbox.self_managed` - JSON config for self-managed provider
- `sandbox.aliyun_codeinterpreter` - JSON config for Aliyun Code Interpreter provider
- `sandbox.e2b` - JSON config for E2B provider

**Note**: The `value` field has a 1024 character limit, which should be sufficient for typical sandbox configurations. If larger configs are needed, consider using a TextField or a separate configuration table.

### Configuration Schema

Each provider's configuration is stored as a **single JSON object** in the `value` field:

#### Self-managed provider

```json
{
  "name": "sandbox.self_managed",
  "source": "variable",
  "data_type": "json",
  "value": "{\"endpoint\": \"http://localhost:9385\", \"pool_size\": 10, \"max_memory\": \"256m\", \"timeout\": 30}"
}
```

#### Aliyun code interpreter
```json
{
  "name": "sandbox.aliyun_codeinterpreter",
  "source": "variable",
  "data_type": "json",
  "value": "{\"access_key_id\": \"LTAI5t...\", \"access_key_secret\": \"xxxxx\", \"account_id\": \"1234567890...\", \"region\": \"cn-hangzhou\", \"timeout\": 30}"
}
```

#### E2B
```json
{
  "name": "sandbox.e2b",
  "source": "variable",
  "data_type": "json",
  "value": "{\"api_key\": \"e2b_sk_...\", \"region\": \"us\", \"timeout\": 30}"
}
```

#### Active provider selection
```json
{
  "name": "sandbox.provider_type",
  "source": "variable",
  "data_type": "string",
  "value": "self_managed"
}
```

### Provider self-describing Schema

Each provider class implements a static method to describe its configuration schema:

```python
# agent/sandbox/providers/base.py

class SandboxProvider(ABC):
    """Base interface for all sandbox providers"""

    @abstractmethod
    def initialize(self, config: Dict[str, Any]) -> bool:
        """Initialize provider with configuration"""
        pass

    @abstractmethod
    def create_instance(self, template: str = "python") -> SandboxInstance:
        """Create a new sandbox instance"""
        pass

    @abstractmethod
    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10
    ) -> ExecutionResult:
        """Execute code in the sandbox"""
        pass

    @abstractmethod
    def destroy_instance(self, instance_id: str) -> bool:
        """Destroy a sandbox instance"""
        pass

    @abstractmethod
    def health_check(self) -> bool:
        """Check if provider is healthy"""
        pass

    @abstractmethod
    def get_supported_languages(self) -> list[str]:
        """Get list of supported programming languages"""
        pass

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """Return configuration schema for this provider"""
        return {}
```

**Example implementation**:

```python
# agent/sandbox/providers/self_managed.py

class SelfManagedProvider(SandboxProvider):
    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "endpoint": {
                "type": "string",
                "required": True,
                "label": "API Endpoint",
                "placeholder": "http://localhost:9385"
            },
            "pool_size": {
                "type": "integer",
                "default": 10,
                "label": "Container Pool Size",
                "min": 1,
                "max": 100
            },
            "max_memory": {
                "type": "string",
                "default": "256m",
                "label": "Max Memory per Container",
                "options": ["128m", "256m", "512m", "1g"]
            },
            "timeout": {
                "type": "integer",
                "default": 30,
                "label": "Execution Timeout (seconds)",
                "min": 5,
                "max": 300
            }
        }

# agent/sandbox/providers/aliyun_codeinterpreter.py

class AliyunCodeInterpreterProvider(SandboxProvider):
    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "access_key_id": {
                "type": "string",
                "required": True,
                "secret": True,
                "label": "Access Key ID",
                "description": "Aliyun AccessKey ID for authentication"
            },
            "access_key_secret": {
                "type": "string",
                "required": True,
                "secret": True,
                "label": "Access Key Secret",
                "description": "Aliyun AccessKey Secret for authentication"
            },
            "account_id": {
                "type": "string",
                "required": True,
                "label": "Account ID",
                "description": "Aliyun primary account ID (主账号ID), required for API calls"
            },
            "region": {
                "type": "string",
                "default": "cn-hangzhou",
                "label": "Region",
                "options": ["cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen", "cn-guangzhou"],
                "description": "Aliyun region for Code Interpreter service"
            },
            "template_name": {
                "type": "string",
                "required": False,
                "label": "Template Name",
                "description": "Optional sandbox template name for pre-configured environments"
            },
            "timeout": {
                "type": "integer",
                "default": 30,
                "label": "Execution Timeout (seconds)",
                "min": 1,
                "max": 30,
                "description": "Code execution timeout (max 30 seconds - hard limit)"
            }
        }

# agent/sandbox/providers/e2b.py

class E2BProvider(SandboxProvider):
    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "secret": True,
                "label": "API Key"
            },
            "region": {
                "type": "string",
                "default": "us",
                "label": "Region",
                "options": ["us", "eu"]
            },
            "timeout": {
                "type": "integer",
                "default": 30,
                "label": "Execution Timeout (seconds)",
                "min": 5,
                "max": 300
            }
        }
```

**Benefits of Self-describing providers**:

- Single source of truth - schema defined alongside implementation
- Easy to add new providers - no central registry to update
- Type safety - schema stays in sync with provider code
- Flexible - frontend can use schema for validation or hardcode if preferred

### Admin API endpoints

Follow existing pattern in `admin/server/routes.py` and use `SettingsMgr`:

```python
# admin/server/routes.py (add new endpoints)

from flask import request, jsonify
import json
from api.db.services.system_settings_service import SystemSettingsService
from agent.agent.sandbox.providers.self_managed import SelfManagedProvider
from agent.agent.sandbox.providers.aliyun_codeinterpreter import AliyunCodeInterpreterProvider
from agent.agent.sandbox.providers.e2b import E2BProvider
from admin.server.services import SettingsMgr

# Map provider IDs to their classes
PROVIDER_CLASSES = {
    "self_managed": SelfManagedProvider,
    "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
    "e2b": E2BProvider,
}

@admin_bp.route('/api/admin/sandbox/providers', methods=['GET'])
def list_sandbox_providers():
    """List available sandbox providers with their schemas"""
    providers = []
    for provider_id, provider_class in PROVIDER_CLASSES.items():
        schema = provider_class.get_config_schema()
        providers.append({
            "id": provider_id,
            "name": provider_id.replace("_", " ").title(),
            "config_schema": schema
        })
    return jsonify({"data": providers})

@admin_bp.route('/api/admin/sandbox/config', methods=['GET'])
def get_sandbox_config():
    """Get current sandbox configuration"""
    # Get active provider
    active_provider_setting = SystemSettingsService.get_by_name("sandbox.provider_type")
    active_provider = active_provider_setting[0].value if active_provider_setting else None

    config = {"active": active_provider}

    # Load all provider configs
    for provider_id in PROVIDER_CLASSES.keys():
        setting = SystemSettingsService.get_by_name(f"sandbox.{provider_id}")
        if setting:
            try:
                config[provider_id] = json.loads(setting[0].value)
            except json.JSONDecodeError:
                config[provider_id] = {}
        else:
            # Return default values from schema
            provider_class = PROVIDER_CLASSES[provider_id]
            schema = provider_class.get_config_schema()
            config[provider_id] = {
                key: field_def.get("default", "")
                for key, field_def in schema.items()
            }

    return jsonify({"data": config})

@admin_bp.route('/api/admin/sandbox/config', methods=['POST'])
def set_sandbox_config():
    """
    Update sandbox provider configuration.

    Request Parameters:
    - provider_type: Provider identifier (e.g., "self_managed", "e2b")
    - config: Provider configuration dictionary
    - set_active: (optional) If True, also set this provider as active.
                  Default: True for backward compatibility.
                  Set to False to update config without switching providers.
    - test_connection: (optional) If True, test connection before saving

    Response: Success message
    """
    req = request.json
    provider_type = req.get('provider_type')
    config = req.get('config')
    set_active = req.get('set_active', True)  # Default to True

    # Validate provider exists
    if provider_type not in PROVIDER_CLASSES:
        return jsonify({"error": "Unknown provider"}), 400

    # Validate configuration against schema
    provider_class = PROVIDER_CLASSES[provider_type]
    schema = provider_class.get_config_schema()
    validation_result = validate_config(config, schema)
    if not validation_result.valid:
        return jsonify({"error": "Invalid config", "details": validation_result.errors}), 400

    # Test connection if requested
    if req.get('test_connection'):
        test_result = test_provider_connection(provider_type, config)
        if not test_result.success:
            return jsonify({"error": "Connection failed", "details": test_result.error}), 400

    # Store entire config as a single JSON record
    config_json = json.dumps(config)
    setting_name = f"sandbox.{provider_type}"

    existing = SystemSettingsService.get_by_name(setting_name)
    if existing:
        SettingsMgr.update_by_name(setting_name, config_json)
    else:
        SystemSettingsService.save(
            name=setting_name,
            source="variable",
            data_type="json",
            value=config_json
        )

    # Set as active provider if requested (default: True)
    if set_active:
        SettingsMgr.update_by_name("sandbox.provider_type", provider_type)

    return jsonify({"message": "Configuration saved"})

@admin_bp.route('/api/admin/sandbox/test', methods=['POST'])
def test_sandbox_connection():
    """Test connection to sandbox provider"""
    provider_type = request.json.get('provider_type')
    config = request.json.get('config')

    test_result = test_provider_connection(provider_type, config)
    return jsonify({
        "success": test_result.success,
        "message": test_result.message,
        "latency_ms": test_result.latency_ms
    })

@admin_bp.route('/api/admin/sandbox/active', methods=['PUT'])
def set_active_sandbox_provider():
    """Set active sandbox provider"""
    provider_name = request.json.get('provider')

    if provider_name not in PROVIDER_CLASSES:
        return jsonify({"error": "Unknown provider"}), 400

    # Check if provider is configured
    provider_setting = SystemSettingsService.get_by_name(f"sandbox.{provider_name}")
    if not provider_setting:
        return jsonify({"error": "Provider not configured"}), 400

    SettingsMgr.update_by_name("sandbox.provider_type", provider_name)
    return jsonify({"message": "Active provider updated"})
```

## Frontend integration

### Admin settings UI

**Location**: `web/src/pages/SandboxSettings/index.tsx`

```typescript
import { Form, Select, Input, Button, Card, Space, Tag, message } from 'antd';
import { listSandboxProviders, getSandboxConfig, setSandboxConfig, testSandboxConnection } from '@/utils/api';

const SandboxSettings: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [configs, setConfigs] = useState<Config[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<string>('');
  const [testing, setTesting] = useState(false);

  const providerSchema = providers.find(p => p.id === selectedProvider);

  const renderConfigForm = () => {
    if (!providerSchema) return null;

    return (
      <Form layout="vertical">
        {Object.entries(providerSchema.config_schema).map(([key, schema]) => (
          <Form.Item
            key={key}
            name={key}
            label={schema.label}
            rules={[{ required: schema.required }]}
          >
            {schema.secret ? (
              <Input.Password placeholder={schema.placeholder} />
            ) : schema.type === 'integer' ? (
              <InputNumber min={schema.min} max={schema.max} />
            ) : schema.options ? (
              <Select>
                {schema.options.map((opt: string) => (
                  <Option key={opt} value={opt}>{opt}</Option>
                ))}
              </Select>
            ) : (
              <Input placeholder={schema.placeholder} />
            )}
          </Form.Item>
        ))}
      </Form>
    );
  };

  return (
    <Card title="Sandbox Provider Configuration">
      <Space direction="vertical" style={{ width: '100%' }}>
        {/* Provider Selection */}
        <Form.Item label="Select Provider">
          <Select
            style={{ width: '100%' }}
            onChange={setSelectedProvider}
            value={selectedProvider}
          >
            {providers.map(provider => (
              <Option key={provider.id} value={provider.id}>
                <Space>
                  <Icon type={provider.icon} />
                  {provider.name}
                  {provider.tags.map(tag => (
                    <Tag key={tag}>{tag}</Tag>
                  ))}
                </Space>
              </Option>
            ))}
          </Select>
        </Form.Item>

        {/* Dynamic Configuration Form */}
        {renderConfigForm()}

        {/* Actions */}
        <Space>
          <Button type="primary" onClick={handleSave}>
            Save Configuration
          </Button>
          <Button onClick={handleTest} loading={testing}>
            Test Connection
          </Button>
        </Space>
      </Space>
    </Card>
  );
};
```

### API client

**File**: `web/src/utils/api.ts`

```typescript
export async function listSandboxProviders() {
  return request<{ data: Provider[] }>('/api/admin/sandbox/providers');
}

export async function getSandboxConfig() {
  return request<{ data: SandboxConfig }>('/api/admin/sandbox/config');
}

export async function setSandboxConfig(config: SandboxConfigRequest) {
  return request('/api/admin/sandbox/config', {
    method: 'POST',
    data: config,
  });
}

export async function testSandboxConnection(provider: string, config: any) {
  return request('/api/admin/sandbox/test', {
    method: 'POST',
    data: { provider, config },
  });
}

export async function setActiveSandboxProvider(provider: string) {
  return request('/api/admin/sandbox/active', {
    method: 'PUT',
    data: { provider },
  });
}
```

### 4.3 Type Definitions

**File**: `web/src/types/sandbox.ts`

```typescript
interface Provider {
  id: string;
  name: string;
  description: string;
  icon: string;
  tags: string[];
  config_schema: Record<string, ConfigField>;
  supported_languages: string[];
}

interface ConfigField {
  type: 'string' | 'integer' | 'boolean';
  required: boolean;
  secret?: boolean;
  label: string;
  placeholder?: string;
  default?: any;
  options?: string[];
  min?: number;
  max?: number;
}

// Configuration response grouped by provider
interface SandboxConfig {
  active: string;  // Currently active provider
  self_managed?: Record<string, string>;
  aliyun_codeinterpreter?: Record<string, string>;
  e2b?: Record<string, string>;
  // Add more providers as needed
}

// Request to update provider configuration
interface SandboxConfigRequest {
  provider_type: string;
  config: Record<string, string | number | boolean>;
  test_connection?: boolean;
  set_active?: boolean;
}
```

## Integration with Agent system

### Agent component usage

The agent system will use the sandbox through the simplified provider manager, loading global configuration from SystemSettings:

```python
# In agent/components/code_executor.py

import json
from agent.agent.sandbox.providers.manager import ProviderManager
from agent.agent.sandbox.providers.self_managed import SelfManagedProvider
from agent.agent.sandbox.providers.aliyun_codeinterpreter import AliyunCodeInterpreterProvider
from agent.agent.sandbox.providers.e2b import E2BProvider
from api.db.services.system_settings_service import SystemSettingsService

# Map provider IDs to their classes
PROVIDER_CLASSES = {
    "self_managed": SelfManagedProvider,
    "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
    "e2b": E2BProvider,
}

class CodeExecutorComponent:
    def __init__(self):
        self.provider_manager = ProviderManager()
        self._load_active_provider()

    def _load_active_provider(self):
        """Load the active provider from system settings"""
        # Get active provider
        active_setting = SystemSettingsService.get_by_name("sandbox.provider_type")
        if not active_setting:
            raise RuntimeError("No sandbox provider configured")

        active_provider = active_setting[0].value

        # Load configuration for active provider (single JSON record)
        provider_setting = SystemSettingsService.get_by_name(f"sandbox.{active_provider}")
        if not provider_setting:
            raise RuntimeError(f"Sandbox provider {active_provider} not configured")

        # Parse JSON configuration
        try:
            config = json.loads(provider_setting[0].value)
        except json.JSONDecodeError as e:
            raise RuntimeError(f"Invalid sandbox configuration for {active_provider}: {e}")

        # Get provider class
        provider_class = PROVIDER_CLASSES.get(active_provider)
        if not provider_class:
            raise RuntimeError(f"Unknown provider: {active_provider}")

        # Initialize provider
        provider = provider_class()
        provider.initialize(config)

        # Set as active provider in manager
        self.provider_manager.set_provider(active_provider, provider)

    def execute(self, code: str, language: str) -> ExecutionResult:
        """Execute code using the active provider"""
        provider = self.provider_manager.get_provider()

        if not provider:
            raise RuntimeError("No sandbox provider configured")

        # Create instance
        instance = provider.create_instance(template=language)

        try:
            # Execute code
            result = provider.execute_code(
                instance_id=instance.instance_id,
                code=code,
                language=language
            )
            return result
        finally:
            # Always cleanup
            provider.destroy_instance(instance.instance_id)
```

## Security considerations

### Credential storage
- Sensitive credentials (API keys, secrets) encrypted at rest in database
- Use RAGFlow's existing encryption mechanisms (AES-256)
- Never log or expose credentials in error messages or API responses
- Credentials redacted in UI (show only last 4 characters)

### Tenant isolation

- **Configuration**: Global sandbox settings shared by all tenants (admin-only access)
- **Execution**: Sandboxes never shared across tenants/sessions during runtime
- **Instance IDs**: Scoped to tenant: `{tenant_id}:{session_id}:{instance_id}`
- **Network Isolation**: Between tenant sandboxes (VPC per tenant for SaaS providers)
- **Resource Quotas**: Per-tenant limits on concurrent executions, total execution time
- **Audit Logging**: All sandbox executions logged with tenant_id for traceability

### Resource limits
- Timeout limits per execution (configurable per provider, default 30s)
- Memory/CPU limits enforced at provider level
- Automatic cleanup of stale instances (max lifetime: 5 minutes)
- Rate limiting per tenant (max concurrent executions: 10)

### Code security
- For self-managed: AST-based security analysis before execution
- Blocked operations: file system writes, network calls, system commands
- Allowlist approach: only specific imports allowed
- Runtime monitoring for malicious patterns

### Network security
- Self-managed: Network isolation by default, no external access
- SaaS: HTTPS only, certificate pinning
- IP whitelisting for self-managed endpoint access

## Monitoring and observability

### Metrics to track

**Common metrics (all providers)**:
- Execution success rate (target: >95%)
- Average execution time (p50, p95, p99)
- Error rate by error type
- Active instance count
- Queue depth (for self-managed pool)

**Self-managed specific**:
- Container pool utilization (target: 60-80%)
- Host resource usage (CPU, memory, disk)
- Container creation latency
- Container restart rate
- gVisor runtime health

**SaaS specific**:
- API call latency by region
- Rate limit usage and throttling events
- Cost estimation (execution count × unit cost)
- Provider availability (uptime %)
- API error rate by error code

### Logging

Structured logging for all provider operations:
```json
{
  "timestamp": "2025-01-26T10:00:00Z",
  "tenant_id": "tenant_123",
  "provider": "aliyun_codeinterpreter",
  "operation": "execute_code",
  "instance_id": "inst_xyz",
  "language": "python",
  "code_hash": "sha256:...",
  "duration_ms": 1234,
  "status": "success",
  "exit_code": 0,
  "memory_used_mb": 64,
  "region": "cn-hangzhou"
}
```

### Alerts

**Critical alerts**:
- Provider availability < 99%
- Error rate > 5%
- Average execution time > 10s
- Container pool exhaustion (0 available)

**Warning alerts**:
- Cost spike (2x daily average)
- Rate limit approaching (>80%)
- High memory usage (>90%)
- Slow execution times (p95 > 5s)

## Migration path

### Phase 1: Refactor existing code (week 1-2)
**Goals**: Extract current implementation into provider pattern

**Tasks**:
- [ ] Create `agent/sandbox/providers/base.py` with `SandboxProvider` interface
- [ ] Implement `agent/sandbox/providers/self_managed.py` wrapping executor_manager
- [ ] Create `agent/sandbox/providers/manager.py` for provider management
- [ ] Write unit tests for self-managed provider
- [ ] Document existing behavior and configuration

**Deliverables**:
- Provider abstraction layer
- Self-managed provider implementation
- Unit test suite

### Phase 2: Database entegration (week 3)
**Goals**: Add sandbox configuration to admin system

**Tasks**:
- [ ] Add sandbox entries to `conf/system_settings.json` initialization file
- [ ] Extend `SettingsMgr` in `admin/server/services.py` with sandbox-specific methods
- [ ] Add admin endpoints to `admin/server/routes.py`
- [ ] Implement configuration validation logic
- [ ] Add provider connection testing
- [ ] Write API tests

**Deliverables**:
- SystemSettings integration
- Admin API endpoints (`/api/admin/sandbox/*`)
- Configuration validation
- API test suite

### Phase 3: Frontend UI (week 4)
**Goals**: Build admin settings interface

**Tasks**:
- [ ] Create `web/src/pages/SandboxSettings/index.tsx`
- [ ] Implement dynamic form generation from provider schema
- [ ] Add connection testing UI
- [ ] Create TypeScript types
- [ ] Write frontend tests

**Deliverables**:
- Admin settings UI
- Type definitions
- Frontend test suite

### Phase 4: SaaS provider implementation (Week 5-6)
**Goals**: Implement Aliyun Code Interpreter and E2B providers

**Tasks**:
- [ ] Implement `agent/sandbox/providers/aliyun_codeinterpreter.py`
- [ ] Implement `agent/sandbox/providers/e2b.py`
- [ ] Add provider-specific tests with mocking
- [ ] Document provider-specific behaviors
- [ ] Create provider setup guides

**Deliverables**:
- Aliyun Code Interpreter provider
- E2B provider
- Provider documentation

### Phase 5: Agent integration (week 7)
**Goals**: Update agent components to use new provider system

**Tasks**:
- [ ] Update `agent/components/code_executor.py` to use ProviderManager
- [ ] Implement fallback logic
- [ ] Add tenant-specific provider loading
- [ ] Update agent tests
- [ ] Performance testing

**Deliverables**:
- Agent integration
- Fallback mechanism
- Updated test suite

### Phase 6: Monitoring & documentation (week 8)
**Goals**: Add observability and complete documentation

**Tasks**:
- [ ] Implement metrics collection
- [ ] Add structured logging
- [ ] Configure alerts
- [ ] Write deployment guide
- [ ] Write user documentation
- [ ] Create troubleshooting guide

**Deliverables**:
- Monitoring dashboards
- Complete documentation
- Deployment guides

## Testing strategy

### Unit tests

**Provider tests** (`test/agent/sandbox/providers/test_*.py`):
```python
class TestSelfManagedProvider:
    def test_initialize_with_config():
        provider = SelfManagedProvider()
        assert provider.initialize({"endpoint": "http://localhost:9385"})

    def test_create_python_instance():
        provider = SelfManagedProvider()
        provider.initialize(test_config)
        instance = provider.create_instance("python")
        assert instance.status == "running"

    def test_execute_code():
        provider = SelfManagedProvider()
        result = provider.execute_code(instance_id, "print('hello')", "python")
        assert result.exit_code == 0
        assert "hello" in result.stdout
```

**Configuration tests**:
- Test configuration validation for each provider schema
- Test error handling for invalid configurations
- Test secret field redaction

### Integration tests

**Provider Switching**:
- Test switching between providers
- Test fallback mechanism
- Test concurrent provider usage

**Multi-Tenant Isolation**:
- Test tenant configuration isolation
- Test instance ID scoping
- Test resource separation

**Admin API Tests**:
- Test CRUD operations for configurations
- Test connection testing endpoint
- Test validation error responses

### E2E tests

**Complete flow tests**:
```python
def test_sandbox_execution_flow():
    # 1. Configure provider via admin API
    setSandboxConfig(provider="self_managed", config={...})

    # 2. Create agent task with code execution
    task = create_agent_task(code="print('test')")

    # 3. Execute task
    result = execute_agent_task(task.id)

    # 4. Verify result
    assert result.status == "success"
    assert "test" in result.output

    # 5. Verify sandbox cleanup
    assert get_active_instances() == 0
```

**Admin UI tests**:
- Test provider configuration flow
- Test connection testing
- Test error handling in UI

### Performance tests

**Load Testing**:
- Test 100 concurrent executions
- Test pool exhaustion behavior
- Test queue performance (self-managed)

**Latency Testing**:
- Measure cold start time per provider
- Measure execution latency percentiles
- Compare provider performance

## Cost considerations

### Self-managed costs

**Infrastructure**:
- Server hosting: $X/month (depends on specs)
- Maintenance: engineering time
- Scaling: manual, requires additional servers

**Pros**:
- Predictable costs
- No per-execution fees
- Full control over resources

**Cons**:
- High initial setup cost
- Operational overhead
- Finite capacity

### SaaS costs

**Aliyun Code Interpreter** (estimated):
- Pricing: execution time × memory configuration
- Example: 1000 executions/day × 30s × $0.01/1000s = ~$0.30/day

**E2B** (estimated):
- Pricing: $0.02/execution-second
- Example: 1000 executions/day × 30s × $0.02/s = ~$600/day

**Pros**:
- No upfront costs
- Automatic scaling
- No maintenance

**Cons**:
- Variable costs (can spike with usage)
- Network dependency
- Potential for runaway costs

### Cost optimization

**Recommendations**:
- **Hybrid Approach**: Use self-managed for base load, SaaS for spikes
- **Cost Monitoring**: Set budget alerts per tenant
- **Resource Limits**: Enforce max executions per tenant/day
- **Caching**: Reuse instances when possible (self-managed pool)
- **Smart Routing**: Route to cheapest provider based on availability

## Future extensibility

The architecture supports easy addition of new providers:

### Adding a new provider

**Step 1**: Implement provider class with schema

```python
# agent/sandbox/providers/new_provider.py
from .base import SandboxProvider

class NewProvider(SandboxProvider):
    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "secret": True,
                "label": "API Key"
            },
            "region": {
                "type": "string",
                "default": "us-east-1",
                "label": "Region"
            }
        }

    def initialize(self, config: Dict[str, Any]) -> bool:
        self.api_key = config.get("api_key")
        self.region = config.get("region", "us-east-1")
        # Initialize client
        return True

    # Implement other abstract methods...
```

**Step 2**: Register in provider mapping

```python
# In api/apps/sandbox_app.py or wherever providers are listed
from agent.agent.sandbox.providers.new_provider import NewProvider

PROVIDER_CLASSES = {
    "self_managed": SelfManagedProvider,
    "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
    "e2b": E2BProvider,
    "new_provider": NewProvider,  # Add here
}
```

**No central registry to update** - just import and add to the mapping!

### Potential future providers

- **GitHub Codespaces**: For GitHub-integrated workflows
- **Gitpod**: Cloud development environments
- **CodeSandbox**: Frontend code execution
- **AWS Firecracker**: Raw microVM management
- **Custom Provider**: User-defined provider implementations

### Advanced features

**Feature pooling**:
- Share instances across executions (same language, same user)
- Warm pool for reduced latency
- Instance hibernation for cost savings

**Feature multi-region**:
- Route to nearest region
- Failover across regions
- Regional cost optimization

**Feature hybrid execution**:
- Split workloads between providers
- Dynamic provider selection based on cost/performance
- A/B testing for provider performance

## Appendix

### Configuration examples

**SystemSettings initialization file** (`conf/system_settings.json` - add these entries):

```json
{
  "system_settings": [
    {
      "name": "sandbox.provider_type",
      "source": "variable",
      "data_type": "string",
      "value": "self_managed"
    },
    {
      "name": "sandbox.self_managed",
      "source": "variable",
      "data_type": "json",
      "value": "{\"endpoint\": \"http://sandbox-internal:9385\", \"pool_size\": 20, \"max_memory\": \"512m\", \"timeout\": 60, \"enable_seccomp\": true, \"enable_ast_analysis\": true}"
    },
    {
      "name": "sandbox.aliyun_codeinterpreter",
      "source": "variable",
      "data_type": "json",
      "value": "{\"access_key_id\": \"\", \"access_key_secret\": \"\", \"account_id\": \"\", \"region\": \"cn-hangzhou\", \"template_name\": \"\", \"timeout\": 30}"
    },
    {
      "name": "sandbox.e2b",
      "source": "variable",
      "data_type": "json",
      "value": "{\"api_key\": \"\", \"region\": \"us\", \"timeout\": 30}"
    }
  ]
}
```

**Admin API request example** (POST to `/api/admin/sandbox/config`):

```json
{
  "provider_type": "self_managed",
  "config": {
    "endpoint": "http://sandbox-internal:9385",
    "pool_size": 20,
    "max_memory": "512m",
    "timeout": 60,
    "enable_seccomp": true,
    "enable_ast_analysis": true
  },
  "test_connection": true,
  "set_active": true
}
```

**Note**: The `config` object in the request is a plain JSON object. The API will serialize it to a JSON string before storing in SystemSettings.

**Admin API response example** (GET from `/api/admin/sandbox/config`):

```json
{
  "data": {
    "active": "self_managed",
    "self_managed": {
      "endpoint": "http://sandbox-internal:9385",
      "pool_size": 20,
      "max_memory": "512m",
      "timeout": 60,
      "enable_seccomp": true,
      "enable_ast_analysis": true
    },
    "aliyun_codeinterpreter": {
      "access_key_id": "",
      "access_key_secret": "",
      "region": "cn-hangzhou",
      "workspace_id": ""
    },
    "e2b": {
      "api_key": "",
      "region": "us",
      "timeout": 30
    }
  }
}
```

**Note**: The response deserializes the JSON strings back to objects for easier frontend consumption.

### Error codes

| Code | Description | Resolution |
|------|-------------|------------|
| SB001 | Provider not initialized | Configure provider in admin |
| SB002 | Invalid configuration | Check configuration values |
| SB003 | Connection failed | Check network and credentials |
| SB004 | Instance creation failed | Check provider capacity |
| SB005 | Execution timeout | Increase timeout or optimize code |
| SB006 | Out of memory | Reduce memory usage or increase limits |
| SB007 | Code blocked by security policy | Remove blocked imports/operations |
| SB008 | Rate limit exceeded | Reduce concurrency or upgrade plan |
| SB009 | Provider unavailable | Check provider status or use fallback |

### References

- [Daytona Documentation](https://daytona.dev/docs)
- [Aliyun Code Interpreter](https://help.aliyun.com/...)
- [E2B Documentation](https://e2b.dev/docs)

---

**Document version**: 1.0
**Last updated**: 2026-01-26
**Author**: RAGFlow Team
**Status**: Design Specification - Ready for Review

## Appendix C: configuration storage considerations

### Current implementation
- **Storage**: SystemSettings table with `value` field as `TextField` (unlimited length)
- **Migration**: Database migration added to convert from `CharField(1024)` to `TextField`
- **Benefit**: Supports arbitrarily long API keys, workspace IDs, and other SaaS provider credentials

### Validation
- **Schema validation**: Type checking, range validation, required field validation
- **Provider-specific validation**: Custom validation via `validate_config()` method
- **Example**: SelfManagedProvider validates URL format, timeout ranges, pool size constraints

### Configuration storage format
Each provider's configuration is stored as JSON in `SystemSettings.value`:
- `sandbox.provider_type`: Active provider selection
- `sandbox.self_managed`: Self-managed provider JSON config
- `sandbox.aliyun_codeinterpreter`: Aliyun provider JSON config
- `sandbox.e2b`: E2B provider JSON config

## Appendix D: Configuration hot reload limitations

### Current behavior
**Provider configuration requires restart**: When switching sandbox providers in the admin panel, the ragflow service must be restarted for changes to take effect.

**Reason**:
- Admin and ragflow are separate processes
- ragflow loads sandbox provider configuration only at startup
- The `get_provider_manager()` function caches the provider globally
- Configuration changes in MySQL are not automatically detected

**Impact**:
- Switching from `self_managed` → `aliyun_codeinterpreter` requires ragflow restart
- Updating credentials/config requires ragflow restart
- Not a dynamic configuration system

**Workarounds**:
1. **Production**: Restart ragflow service after configuration changes:
   ```bash
   cd docker
   docker compose restart ragflow-server
   ```

2. **Development**: Use the `reload_provider()` function in code:
   ```python
   from agent.sandbox.client import reload_provider
   reload_provider()  # Reloads from MySQL settings
   ```

**Future enhancement**:
To support hot reload without restart, implement configuration change detection:
```python
# In agent/sandbox/client.py
_config_timestamp: Optional[int] = None

def get_provider_manager() -> ProviderManager:
    global _provider_manager, _config_timestamp

    # Check if configuration has changed
    setting = SystemSettingsService.get_by_name("sandbox.provider_type")
    current_timestamp = setting[0].update_time if setting else 0

    if _config_timestamp is None or current_timestamp > _config_timestamp:
        # Configuration changed, reload provider
        _provider_manager = None
        _load_provider_from_settings()
        _config_timestamp = current_timestamp

    return _provider_manager
```

However, this adds overhead on every `execute_code()` call. For production use, explicit restart is preferred for simplicity and reliability.

## Appendix E: Arguments parameter support

### Overview
All sandbox providers support passing arguments to the `main()` function in user code. This enables dynamic parameter injection for code execution.

### Implementation details

**Base interface**:
```python
# agent/sandbox/providers/base.py
@abstractmethod
def execute_code(
    self,
    instance_id: str,
    code: str,
    language: str,
    timeout: int = 10,
    arguments: Optional[Dict[str, Any]] = None
) -> ExecutionResult:
    """
    Execute code in the sandbox.

    The code should contain a main() function that will be called with:
    - Python: main(**arguments) if arguments provided, else main()
    - JavaScript: main(arguments) if arguments provided, else main()
    """
    pass
```

**Provider implementations**:

1. **Self-managed provider** ([self_managed.py:164](agent/sandbox/providers/self_managed.py:164)):
   - Passes arguments via HTTP API: `"arguments": arguments or {}`
   - executor_manager receives and passes to code via command line
   - Runner script: `args = json.loads(sys.argv[1])` then `result = main(**args)`

2. **Aliyun Code Interpreter** ([aliyun_codeinterpreter.py:260-275](agent/sandbox/providers/aliyun_codeinterpreter.py:260-275)):
   - Wraps user code to call `main(**arguments)` or `main()` if no arguments
   - Python example:
     ```python
     if arguments:
         wrapped_code = f'''{code}

     if __name__ == "__main__":
         import json
         result = main(**{json.dumps(arguments)})
         print(json.dumps(result) if isinstance(result, dict) else result)
     '''
     ```
   - JavaScript example:
     ```javascript
     if arguments:
         wrapped_code = f'''{code}

     const result = main({json.dumps(arguments)});
     console.log(typeof result === 'object' ? JSON.stringify(result) : String(result));
     '''
     ```

**Client layer** ([client.py:138-190](agent/sandbox/client.py:138-190)):
```python
def execute_code(
    code: str,
    language: str = "python",
    timeout: int = 30,
    arguments: Optional[Dict[str, Any]] = None
) -> ExecutionResult:
    provider_manager = get_provider_manager()
    provider = provider_manager.get_provider()

    instance = provider.create_instance(template=language)
    try:
        result = provider.execute_code(
            instance_id=instance.instance_id,
            code=code,
            language=language,
            timeout=timeout,
            arguments=arguments  # Passed through to provider
        )
        return result
    finally:
        provider.destroy_instance(instance.instance_id)
```

**CodeExec tool integration** ([code_exec.py:136-165](agent/tools/code_exec.py:136-165)):
```python
def _execute_code(self, language: str, code: str, arguments: dict):
    # ... collect arguments from component configuration

    result = sandbox_execute_code(
        code=code,
        language=language,
        timeout=int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)),
        arguments=arguments  # Passed through to sandbox client
    )
```

### Usage examples

**Python code with arguments**:
```python
# User code
def main(name: str, count: int) -> dict:
    """Generate greeting"""
    return {"message": f"Hello {name}!" * count}

# Called with: arguments={"name": "World", "count": 3}
# Result: {"message": "Hello World!Hello World!Hello World!"}
```

**JavaScript code with arguments**:
```javascript
// User code
function main(args) {
  const { name, count } = args;
  return `Hello ${name}!`.repeat(count);
}

// Called with: arguments={"name": "World", "count": 3}
// Result: "Hello World!Hello World!Hello World!"
```

### Important notes

1. **Function signature**: Code MUST define a `main()` function
   - Python: `def main(**kwargs)` or `def main()` if no arguments
   - JavaScript: `function main(args)` or `function main()` if no arguments

2. **Type consistency**: Arguments are passed as JSON, so types are preserved:
   - Numbers → int/float
   - Strings → str
   - Booleans → bool
   - Objects → dict (Python) / object (JavaScript)
   - Arrays → list (Python) / array (JavaScript)

3. **Return value**: Return value is serialized as JSON for parsing
   - Python: `print(json.dumps(result))` if dict
   - JavaScript: `console.log(JSON.stringify(result))` if object

4. **Provider alignment**: All providers (self_managed, aliyun_codeinterpreter, e2b) implement arguments passing consistently
