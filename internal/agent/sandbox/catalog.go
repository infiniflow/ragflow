package sandbox

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// ValidationError marks a sandbox configuration rejected at the admin boundary.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var providerOrder = []string{
	"local", "self_managed", "ssh", "aliyun_codeinterpreter", "e2b", "tenki", "ucloud_agent_sandbox",
}

var providerMetadata = map[string]map[string]any{
	"local":                  {"name": "Local", "description": "Execute code directly on the current host process.", "tags": []any{"local", "host", "minimal"}},
	"self_managed":           {"name": "Self-Managed", "description": "On-premise deployment using Daytona/Docker", "tags": []any{"self-hosted", "low-latency", "secure"}},
	"ssh":                    {"name": "SSH", "description": "Execute code on a remote machine over SSH.", "tags": []any{"remote", "ssh", "custom-runtime"}},
	"aliyun_codeinterpreter": {"name": "Aliyun Code Interpreter", "description": "Aliyun Function Compute Code Interpreter - Code execution in serverless microVMs", "tags": []any{"saas", "cloud", "scalable", "aliyun"}},
	"e2b":                    {"name": "E2B", "description": "E2B Cloud - Code Execution Sandboxes", "tags": []any{"saas", "fast", "global"}},
	"tenki":                  {"name": "Tenki", "description": "Tenki - Disposable microVM code sandboxes", "tags": []any{"saas", "cloud", "microvm", "isolated"}},
	"ucloud_agent_sandbox":   {"name": "UCloud Agent Sandbox", "description": "UCloud Agent Sandbox - Disposable cloud sandboxes for agent code execution", "tags": []any{"saas", "cloud", "isolated", "ucloud"}},
}

func field(typ string, required bool, label string) map[string]any {
	return map[string]any{"type": typ, "required": required, "label": label}
}

func ConfigSchema(provider string) (map[string]any, error) {
	if provider == "self_managed" {
		return selfManagedSchema(), nil
	}
	s := staticSchema(provider)
	if s == nil {
		return nil, &ValidationError{Message: fmt.Sprintf("unknown provider: %s", provider)}
	}
	return cloneMap(s), nil
}

func ListProviders() []map[string]any {
	result := make([]map[string]any, 0, len(providerOrder))
	for _, id := range providerOrder {
		item := cloneMap(providerMetadata[id])
		item["id"] = id
		result = append(result, item)
	}
	return result
}

func ValidateConfig(provider string, config map[string]any) error {
	if config == nil {
		return invalid("configuration must be an object")
	}
	schema, err := ConfigSchema(provider)
	if err != nil {
		return err
	}
	for name, raw := range schema {
		fieldSchema, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, present := config[name]
		if required, _ := fieldSchema["required"].(bool); required && !present {
			return invalid("required field %q is missing", name)
		}
		if !present {
			continue
		}
		typ, _ := fieldSchema["type"].(string)
		if !validType(value, typ) {
			return invalid("field %q must be a %s", name, typ)
		}
		if typ == "integer" {
			n := integer(value)
			if min, ok := number(fieldSchema["min"]); ok && n < min {
				return invalid("field %q must be >= %v", name, fieldSchema["min"])
			}
			if max, ok := number(fieldSchema["max"]); ok && n > max {
				return invalid("field %q must be <= %v", name, fieldSchema["max"])
			}
		}
	}
	switch provider {
	case "self_managed":
		if endpoint, _ := config["endpoint"].(string); endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			return invalid("invalid endpoint format: %s", endpoint)
		}
		poolValue, present := config["executor_manager_pool_size"]
		if !present {
			poolValue = config["pool_size"]
		}
		if pool, ok := integerValue(poolValue); ok && pool <= 0 {
			return invalid("pool size must be greater than 0")
		}
		timeout := integerOr(config["timeout"], 30)
		if timeout < 1 || timeout > 600 {
			return invalid("timeout must be between 1 and 600 seconds")
		}
		if retries, ok := integerValue(config["max_retries"]); ok && (retries < 0 || retries > 10) {
			return invalid("max retries must be between 0 and 10")
		}
	case "ssh":
		if strings.TrimSpace(stringValue(config["host"])) == "" {
			return invalid("SSH host is required")
		}
		if strings.TrimSpace(stringValue(config["username"])) == "" {
			return invalid("SSH username is required")
		}
		if stringValue(config["password"]) == "" && stringValue(config["private_key"]) == "" {
			return invalid("Either password or private_key must be provided")
		}
		for _, key := range []string{"python_bin", "node_bin"} {
			if value, present := config[key].(string); present && value != "" && strings.TrimSpace(value) == "" {
				return invalid("%s is required", key)
			}
		}
		for _, key := range []string{"timeout", "max_output_bytes", "max_artifacts", "max_artifact_bytes"} {
			n := integerOr(config[key], 0)
			if key == "max_artifacts" {
				if n < 0 {
					return invalid("max_artifacts must be greater than or equal to 0")
				}
			} else if n <= 0 {
				return invalid("%s must be greater than 0", key)
			}
		}
	case "aliyun_codeinterpreter":
		if id := stringValue(config["access_key_id"]); id != "" && !strings.HasPrefix(id, "LTAI") {
			return invalid("invalid AccessKey ID format (should start with 'LTAI')")
		}
		if stringValue(config["account_id"]) == "" {
			return invalid("Account ID is required")
		}
		if region := stringValue(config["region"]); region != "" && !slices.Contains([]string{"cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen", "cn-guangzhou"}, region) {
			return invalid("invalid region")
		}
	case "tenki":
		if strings.TrimSpace(stringValue(config["api_key"])) == "" {
			return invalid("Tenki API key is required")
		}
		for _, key := range []string{"timeout", "max_lifetime", "max_output_bytes", "max_artifact_bytes"} {
			if integerOr(config[key], 0) <= 0 {
				return invalid("%s must be greater than 0", key)
			}
		}
		if integerOr(config["max_artifacts"], 0) < 0 {
			return invalid("max_artifacts must be greater than or equal to 0")
		}
	case "ucloud_agent_sandbox":
		if strings.TrimSpace(stringValue(config["api_key"])) == "" {
			return invalid("UCloud Agent Sandbox API key is required")
		}
		if _, present := config["template"]; present && strings.TrimSpace(stringValue(config["template"])) == "" {
			return invalid("template is required")
		}
	}
	return nil
}

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

func validType(value any, typ string) bool {
	switch typ {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		_, ok := integerValue(value)
		return ok
	default:
		return true
	}
}

func integerValue(value any) (int64, bool) {
	switch n := value.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), n == float64(int64(n))
	case float32:
		return int64(n), n == float32(int64(n))
	default:
		return 0, false
	}
}

func integer(value any) int64 { n, _ := integerValue(value); return n }
func integerOr(value any, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	if n, ok := integerValue(value); ok {
		return n
	}
	return 0
}
func number(value any) (int64, bool) { return integerValue(value) }
func stringValue(value any) string {
	v, _ := value.(string)
	return v
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		switch v := value.(type) {
		case map[string]any:
			out[key] = cloneMap(v)
		case []any:
			out[key] = append([]any(nil), v...)
		default:
			out[key] = value
		}
	}
	return out
}

func env(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(env(key, strconv.FormatInt(fallback, 10)), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func selfManagedSchema() map[string]any {
	s := staticSchema("self_managed")
	s["executor_manager_image"].(map[string]any)["default"] = env("SANDBOX_EXECUTOR_MANAGER_IMAGE", "infiniflow/sandbox-executor-manager:latest")
	s["executor_manager_pool_size"].(map[string]any)["default"] = envInt("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE", 3)
	s["base_python_image"].(map[string]any)["default"] = env("SANDBOX_BASE_PYTHON_IMAGE", "infiniflow/sandbox-base-python:latest")
	s["base_nodejs_image"].(map[string]any)["default"] = env("SANDBOX_BASE_NODEJS_IMAGE", "infiniflow/sandbox-base-nodejs:latest")
	s["executor_manager_port"].(map[string]any)["default"] = envInt("SANDBOX_EXECUTOR_MANAGER_PORT", 9385)
	s["enable_seccomp"].(map[string]any)["default"] = strings.EqualFold(env("SANDBOX_ENABLE_SECCOMP", "false"), "true")
	s["max_memory"].(map[string]any)["default"] = env("SANDBOX_MAX_MEMORY", "256m")
	s["container_network"].(map[string]any)["default"] = env("SANDBOX_CONTAINER_NETWORK", "none")
	s["sandbox_timeout"].(map[string]any)["default"] = env("SANDBOX_TIMEOUT", "10s")
	return cloneMap(s)
}

func staticSchema(provider string) map[string]any {
	switch provider {
	case "local":
		return map[string]any{
			"python_bin":         fieldWith("string", false, "Python Binary", "default", "python3", "description", "Python executable used for local code execution."),
			"node_bin":           fieldWith("string", false, "Node.js Binary", "default", "node", "description", "Node.js executable used for local JavaScript execution."),
			"work_dir":           fieldWith("string", false, "Working Directory", "default", "/tmp/ragflow-codeexec", "description", "Directory used to store temporary scripts and artifacts on the current host."),
			"timeout":            limits("integer", false, "Timeout (seconds)", 30, 1, 600, "Maximum execution time for each local run. Unit: seconds."),
			"max_memory_mb":      limits("integer", false, "Max Memory (MB)", 512, 1, 65536, "Address-space memory limit for the local child process. Unit: MB."),
			"max_output_bytes":   limits("integer", false, "Max Output (bytes)", 1048576, 1024, 10485760, "Maximum combined stdout and stderr size. Unit: bytes."),
			"max_artifacts":      limits("integer", false, "Max Artifacts", 20, 0, 100, "Maximum number of files collected from the artifacts directory."),
			"max_artifact_bytes": limits("integer", false, "Max Artifact Size (bytes)", 10485760, 1024, 104857600, "Maximum size of a single artifact file. Unit: bytes."),
		}
	case "self_managed":
		return map[string]any{
			"endpoint":                   fieldWith("string", true, "Executor Manager Endpoint", "placeholder", "http://sandbox-executor-manager:9385", "default", "http://sandbox-executor-manager:9385", "description", "HTTP endpoint used by RAGFlow to call sandbox-executor-manager.", "scope", "runtime", "readonly", false),
			"api_token":                  fieldWith("string", false, "Executor Manager API Token", "secret", true, "placeholder", "Optional; defaults to SANDBOX_EXECUTOR_MANAGER_API_TOKEN", "default", "", "description", "Shared secret authenticating RAGFlow to sandbox-executor-manager. Must match SANDBOX_EXECUTOR_MANAGER_API_TOKEN on the executor-manager side.", "scope", "runtime", "readonly", false),
			"timeout":                    limitsScoped("integer", false, "Request Timeout (seconds)", 30, 5, 300, "Maximum request time for a single code execution call. Unit: seconds.", "runtime", false),
			"executor_manager_image":     fieldWith("string", false, "Executor Manager Image", "default", "infiniflow/sandbox-executor-manager:latest", "description", "Docker image used by sandbox-executor-manager.", "scope", "deployment", "readonly", true),
			"executor_manager_pool_size": limitsScoped("integer", false, "Container Pool Size", int64(3), 1, 100, "Container pool size used by sandbox-executor-manager.", "deployment", true),
			"base_python_image":          fieldWith("string", false, "Base Python Image", "default", "infiniflow/sandbox-base-python:latest", "description", "Python runtime image used by executor-managed containers.", "scope", "deployment", "readonly", true),
			"base_nodejs_image":          fieldWith("string", false, "Base Node.js Image", "default", "infiniflow/sandbox-base-nodejs:latest", "description", "Node.js runtime image used by executor-managed containers.", "scope", "deployment", "readonly", true),
			"executor_manager_port":      limitsScoped("integer", false, "Executor Manager Port", int64(9385), 1, 65535, "Host port exposed by sandbox-executor-manager.", "deployment", true),
			"enable_seccomp":             fieldWith("boolean", false, "Enable Seccomp", "default", false, "description", "Whether sandbox-executor-manager starts containers with seccomp enabled.", "scope", "deployment", "readonly", true),
			"max_memory":                 fieldWith("string", false, "Max Memory", "default", "256m", "description", "Memory limit applied to each sandbox container. Common format: 256m or 1g.", "scope", "deployment", "readonly", true),
			"container_network":          fieldWith("string", false, "Container Network", "default", "none", "description", "Docker network attached to sandbox containers. Defaults to 'none' (no external network access); set to 'bridge' only if sandboxed code needs outbound network.", "scope", "deployment", "readonly", true),
			"sandbox_timeout":            fieldWith("string", false, "Sandbox Timeout", "default", "10s", "description", "Executor-manager container timeout for each sandbox run. Common format: 10s or 1m.", "scope", "deployment", "readonly", true),
		}
	case "ssh":
		return sshSchema()
	case "aliyun_codeinterpreter":
		return map[string]any{
			"access_key_id":     fieldWith("string", true, "Access Key ID", "placeholder", "LTAI5t...", "description", "Aliyun AccessKey ID for authentication", "secret", false),
			"access_key_secret": fieldWith("string", true, "Access Key Secret", "placeholder", "••••••••••••••••", "description", "Aliyun AccessKey Secret for authentication", "secret", true),
			"account_id":        fieldWith("string", true, "Account ID", "placeholder", "1234567890...", "description", "Aliyun primary account ID, required for API calls"),
			"region":            fieldWith("string", false, "Region", "default", "cn-hangzhou", "description", "Aliyun region for Code Interpreter service", "options", []any{"cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen", "cn-guangzhou"}),
			"template_name":     fieldWith("string", false, "Template Name", "placeholder", "my-interpreter", "description", "Optional sandbox template name for pre-configured environments"),
			"timeout":           limits("integer", false, "Execution Timeout (seconds)", 30, 1, 30, "Code execution timeout (max 30 seconds - hard limit)"),
		}
	case "e2b":
		return map[string]any{"api_key": fieldWith("string", true, "API Key", "placeholder", "e2b_sk_...", "description", "E2B API key for authentication", "secret", true), "region": fieldWith("string", false, "Region", "default", "us", "description", "E2B service region (us or eu)"), "timeout": limits("integer", false, "Request Timeout (seconds)", 30, 5, 300, "API request timeout for code execution")}
	case "tenki":
		return tenkiSchema()
	case "ucloud_agent_sandbox":
		return ucloudSchema()
	}
	return nil
}

func fieldWith(typ string, required bool, label string, extras ...any) map[string]any {
	out := field(typ, required, label)
	for i := 0; i < len(extras); i += 2 {
		out[extras[i].(string)] = extras[i+1]
	}
	return out
}

func limits(typ string, required bool, label string, def, min, max int64, description string) map[string]any {
	return fieldWith(typ, required, label, "default", def, "min", min, "max", max, "description", description)
}

func limitsNoDescription(typ string, required bool, label string, def, min, max int64) map[string]any {
	return fieldWith(typ, required, label, "default", def, "min", min, "max", max)
}

func limitsScoped(typ string, required bool, label string, def, min, max int64, description, scope string, readonly bool) map[string]any {
	return fieldWith(typ, required, label, "default", def, "min", min, "max", max, "description", description, "scope", scope, "readonly", readonly)
}

func sshSchema() map[string]any {
	return map[string]any{
		"host":               fieldWith("string", true, "SSH Host", "placeholder", "192.168.1.10", "description", "Remote host that will execute generated code."),
		"port":               limits("integer", true, "SSH Port", 22, 1, 65535, "SSH port on the remote host."),
		"username":           fieldWith("string", true, "SSH Username", "placeholder", "ragflow", "description", "Username used to connect to the remote host."),
		"password":           fieldWith("string", false, "SSH Password", "secret", true, "placeholder", "Optional when using a private key", "description", "Password-based SSH authentication."),
		"private_key":        fieldWith("string", false, "SSH Private Key", "secret", true, "multiline", true, "placeholder", "Paste PEM content or enter a local file path", "description", "Private key PEM content or a readable private key path on the RAGFlow host."),
		"passphrase":         fieldWith("string", false, "Private Key Passphrase", "secret", true, "placeholder", "Optional", "description", "Passphrase for the private key if it is encrypted."),
		"known_hosts":        fieldWith("string", false, "SSH known_hosts File", "placeholder", "/etc/ragflow/ssh_known_hosts", "description", "Path to an OpenSSH-format known_hosts file used to verify the remote host's key. When set, the file is loaded on top of the system host keys (~/.ssh/known_hosts). When unset, only system keys are used and unknown hosts are rejected."),
		"python_bin":         fieldWith("string", false, "Python Binary", "default", "python3", "description", "Python executable used for remote code execution."),
		"node_bin":           fieldWith("string", false, "Node.js Binary", "default", "node", "description", "Node.js executable used for remote JavaScript execution."),
		"work_dir":           fieldWith("string", false, "Remote Workspace Root", "default", "/tmp", "placeholder", "/tmp", "description", "Writable remote directory used to create a temporary workspace."),
		"timeout":            limits("integer", false, "Timeout (seconds)", 30, 1, 600, "Maximum SSH execution time for a single run."),
		"max_output_bytes":   limits("integer", false, "Max Output Bytes", 1048576, 1024, 10485760, "Maximum combined stdout and stderr size."),
		"max_artifacts":      limits("integer", false, "Max Artifacts", 20, 0, 100, "Maximum number of files collected from the remote artifacts directory."),
		"max_artifact_bytes": limits("integer", false, "Max Artifact Bytes", 10485760, 1024, 104857600, "Maximum size of a single artifact file in bytes."),
	}
}

func tenkiSchema() map[string]any {
	return map[string]any{
		"api_key":            fieldWith("string", true, "API Key", "secret", true, "placeholder", "tk_...", "description", "Tenki API key. Create one at https://app.tenki.cloud under API Keys."),
		"base_url":           fieldWith("string", false, "API Endpoint", "placeholder", "https://api.tenki.cloud", "description", "Override the Tenki API endpoint. Leave empty for the default."),
		"image":              fieldWith("string", false, "Sandbox Image", "description", "Base image for sandboxes. Empty uses the Tenki default image (includes python3 and node)."),
		"allow_outbound":     fieldWith("boolean", false, "Allow Outbound Network", "default", false, "description", "Security-relevant. Disabled by default so sandboxed code has no network access. Enable it to let code make outbound connections (e.g. to install packages)."),
		"timeout":            limits("integer", false, "Timeout (seconds)", 30, 1, 600, "Maximum execution time for a single run."),
		"max_lifetime":       limits("integer", false, "Max Sandbox Lifetime (seconds)", 3600, 60, 86400, "Tenki reclaims a sandbox after this, guarding against leaks if the run is interrupted."),
		"cpu_cores":          limits("integer", false, "vCPU Cores", 0, 0, 16, "vCPU per sandbox. 0 uses the Tenki default."),
		"memory_mb":          limits("integer", false, "Memory (MB)", 0, 0, 65536, "Memory per sandbox in MB. 0 uses the Tenki default."),
		"disk_size_gb":       limits("integer", false, "Disk Size (GB)", 0, 0, 100, "Disk size per sandbox in GB. 0 uses the Tenki default."),
		"max_output_bytes":   limits("integer", false, "Max Output Bytes", 1048576, 1024, 10485760, "Maximum combined stdout and stderr size."),
		"max_artifacts":      limits("integer", false, "Max Artifacts", 20, 0, 100, "Maximum number of files collected from the sandbox artifacts directory."),
		"max_artifact_bytes": limits("integer", false, "Max Artifact Bytes", 10485760, 1024, 104857600, "Maximum size of a single artifact file in bytes."),
	}
}

func ucloudSchema() map[string]any {
	return map[string]any{
		"api_key":               fieldWith("string", true, "API Key", "secret", true, "description", "UCloud Agent Sandbox API key."),
		"region":                fieldWith("string", false, "Region", "default", "cn-wlcb", "description", "UCloud Agent Sandbox region, for example cn-wlcb or us-ca."),
		"domain":                fieldWith("string", false, "Domain", "description", "Override the sandbox domain. Leave empty to derive it from Region."),
		"api_url":               fieldWith("string", false, "API URL", "description", "Override the UCloud Agent Sandbox control-plane API URL."),
		"template":              fieldWith("string", false, "Template", "default", "base", "description", "Sandbox template. The base template includes Python and Node.js."),
		"allow_internet_access": fieldWith("boolean", false, "Allow Internet Access", "default", false, "description", "Allow sandboxed code to access the internet. Disabled by default."),
		"insecure_http":         fieldWith("boolean", false, "Use Insecure HTTP", "default", false, "description", "Use HTTP instead of HTTPS. Enable only for trusted private deployments."),
		"timeout":               limitsNoDescription("integer", false, "Execution Timeout (seconds)", 30, 1, 600),
		"sandbox_timeout":       limitsNoDescription("integer", false, "Sandbox Lifetime (seconds)", 300, 60, 86400),
		"max_output_bytes":      limitsNoDescription("integer", false, "Max Output Bytes", 1048576, 1024, 10485760),
		"max_artifacts":         limitsNoDescription("integer", false, "Max Artifacts", 20, 0, 100),
		"max_artifact_bytes":    limitsNoDescription("integer", false, "Max Artifact Bytes", 10485760, 1024, 104857600),
	}
}
