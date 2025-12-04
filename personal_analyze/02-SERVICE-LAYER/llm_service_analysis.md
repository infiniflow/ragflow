# LLM Service Analysis - Model Abstraction Layer

## Tong Quan

LLM Service Layer cung cấp abstraction thống nhất cho 60+ LLM providers bao gồm chat, embedding, và reranking models.

## File Locations
```
/rag/llm/
├── chat_model.py       # Chat models (1922 lines, 20+ providers)
├── embedding_model.py  # Embedding models (943 lines, 30+ providers)
├── rerank_model.py     # Reranking models (505 lines, 15+ providers)
└── __init__.py         # Factory pattern registration

/api/db/services/
├── llm_service.py      # LLMBundle wrapper with token tracking
└── tenant_llm_service.py # Model instantiation and configuration
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Application Layer                           │
│  (dialog_app, agent components, prompt generators)              │
└──────────────────────────┬──────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────┐
│                   LLMBundle/LLM4Tenant                          │
│  (Wrapper with token tracking, langfuse integration, pooling)   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────┐
│          Model Factory Pattern (ChatModel[], etc.)              │
│  Routes to specific provider implementations                    │
└──────────────────────────┬──────────────────────────────────────┘
                           │
    ┌──────────┬──────────┬┴─────────┬──────────┐
    │          │          │          │          │
┌───▼──┐  ┌───▼──┐  ┌───▼──┐  ┌───▼──┐  ┌───▼──┐
│Base  │  │LiteL │  │OpenAI│  │Azure │  │Other │
│Class │  │LMBase│  │Chat  │  │Chat  │  │Impls │
└──────┘  └──────┘  └──────┘  └──────┘  └──────┘
```

## Supported Providers

### Chat Models (20+)

| Provider | Class | Key Features |
|----------|-------|--------------|
| OpenAI | `GptTurbo` | Default base model |
| Azure OpenAI | `AzureChat` | Custom API version |
| Anthropic | `LiteLLMBase` | Via liteLLM |
| Google Gemini | `GoogleChat` | Thinking budget |
| Qwen | `LiteLLMBase` | Via Dashscope |
| AWS Bedrock | `LiteLLMBase` | Multi-credential |
| Moonshot | `LiteLLMBase` | Fast API |
| DeepSeek | `LiteLLMBase` | Cost-effective |
| Groq | `LiteLLMBase` | Speed-optimized |
| Ollama | `LiteLLMBase` | Local models |
| Mistral | `MistralChat` | Native SDK |
| Cohere | `LiteLLMBase` | Via liteLLM |

### Embedding Models (30+)

| Provider | Class | Max Tokens |
|----------|-------|-----------|
| OpenAI | `OpenAIEmbed` | 8191 |
| Azure OpenAI | `AzureEmbed` | Custom |
| Builtin | `BuiltinEmbed` | 8000/30000 |
| Qwen | `QWenEmbed` | 2048 |
| ZHIPU-AI | `ZhipuEmbed` | 512-3072 |
| Jina | `JinaEmbed` | 8196 |
| Voyage AI | `VoyageEmbed` | Custom |
| Cohere | `CoHereEmbed` | Custom |
| NVIDIA | `NvidiaEmbed` | Custom |

### Reranking Models (15+)

| Provider | Class |
|----------|-------|
| Jina | `JinaRerank` |
| Cohere | `CoHereRerank` |
| NVIDIA | `NvidiaRerank` |
| Voyage AI | `VoyageRerank` |
| BGE | `HuggingfaceRerank` |

## Core Implementation

### Base Chat Class

```python
class Base(ABC):
    def __init__(self, key, model_name, base_url, **kwargs):
        timeout = int(os.environ.get("LM_TIMEOUT_SECONDS", 600))
        self.client = OpenAI(api_key=key, base_url=base_url, timeout=timeout)
        self.model_name = model_name

        # Retry configuration
        self.max_retries = kwargs.get("max_retries",
                                      int(os.environ.get("LLM_MAX_RETRIES", 5)))
        self.base_delay = kwargs.get("retry_interval",
                                     float(os.environ.get("LLM_BASE_DELAY", 2.0)))
        self.max_rounds = kwargs.get("max_rounds", 5)

        # Tool calling
        self.is_tools = False
        self.tools = []
        self.toolcall_sessions = {}
```

### Chat Method with Retry

```python
def chat(self, system, history, gen_conf={}, **kwargs):
    if system and history and history[0].get("role") != "system":
        history.insert(0, {"role": "system", "content": system})
    gen_conf = self._clean_conf(gen_conf)

    # Exponential backoff retry strategy
    for attempt in range(self.max_retries + 1):
        try:
            return self._chat(history, gen_conf, **kwargs)
        except Exception as e:
            e = self._exceptions(e, attempt)
            if e:
                return e, 0
```

### Streaming with Reasoning Support

```python
def _chat_streamly(self, history, gen_conf, **kwargs):
    reasoning_start = False

    response = self.client.chat.completions.create(
        model=self.model_name,
        messages=history,
        stream=True,
        **gen_conf
    )

    for resp in response:
        if not resp.choices:
            continue

        # Support for reasoning models (QwQ, etc.)
        if kwargs.get("with_reasoning", True) and \
           hasattr(resp.choices[0].delta, "reasoning_content") and \
           resp.choices[0].delta.reasoning_content:
            ans = ""
            if not reasoning_start:
                reasoning_start = True
                ans = "<think>"
            ans += resp.choices[0].delta.reasoning_content + "</think>"
        else:
            reasoning_start = False
            ans = resp.choices[0].delta.content

        tol = total_token_count_from_response(resp)
        yield ans, tol
```

### Tool Calling (Function Calling)

```python
def chat_with_tools(self, system: str, history: list, gen_conf: dict = {}):
    hist = deepcopy(history)

    for _ in range(self.max_rounds + 1):
        response = self.client.chat.completions.create(
            model=self.model_name,
            messages=history,
            tools=self.tools,
            tool_choice="auto",
            **gen_conf
        )

        # Check if model returned tool calls
        if not hasattr(response.choices[0].message, "tool_calls") or \
           not response.choices[0].message.tool_calls:
            return response.choices[0].message.content, tk_count

        # Process tool calls with JSON repair
        for tool_call in response.choices[0].message.tool_calls:
            name = tool_call.function.name
            args = json_repair.loads(tool_call.function.arguments)
            tool_response = self.toolcall_session.tool_call(name, args)
            history = self._append_history(history, tool_call, tool_response)
```

## Error Handling & Retry

### Error Classification

```python
class LLMErrorCode(StrEnum):
    ERROR_RATE_LIMIT = "RATE_LIMIT_EXCEEDED"
    ERROR_AUTHENTICATION = "AUTH_ERROR"
    ERROR_INVALID_REQUEST = "INVALID_REQUEST"
    ERROR_SERVER = "SERVER_ERROR"
    ERROR_TIMEOUT = "TIMEOUT"
    ERROR_CONNECTION = "CONNECTION_ERROR"
    ERROR_MODEL = "MODEL_ERROR"
    ERROR_CONTENT_FILTER = "CONTENT_FILTERED"
    ERROR_QUOTA = "QUOTA_EXCEEDED"
    ERROR_MAX_RETRIES = "MAX_RETRIES_EXCEEDED"

def _classify_error(self, error):
    error_str = str(error).lower()

    keywords_mapping = [
        (["quota", "credit", "billing"], LLMErrorCode.ERROR_QUOTA),
        (["rate limit", "429"], LLMErrorCode.ERROR_RATE_LIMIT),
        (["auth", "key", "401"], LLMErrorCode.ERROR_AUTHENTICATION),
        (["server", "503", "502"], LLMErrorCode.ERROR_SERVER),
        (["timeout"], LLMErrorCode.ERROR_TIMEOUT),
    ]
    return mapped_code
```

### Retry Strategy

```python
@property
def _retryable_errors(self) -> set[str]:
    return {
        LLMErrorCode.ERROR_RATE_LIMIT,
        LLMErrorCode.ERROR_SERVER,
    }

def _get_delay(self):
    """Exponential backoff with random jitter"""
    return self.base_delay * random.uniform(10, 150)  # 20-300 seconds
```

## LiteLLM Provider Integration

```python
class LiteLLMBase(ABC):
    _FACTORY_NAME = [
        "Tongyi-Qianwen", "Bedrock", "Moonshot", "xAI", "DeepInfra",
        "Groq", "Cohere", "Gemini", "DeepSeek", "NVIDIA", "TogetherAI",
        "Anthropic", "Ollama", "OpenRouter", "StepFun", ...
    ]

    def _construct_completion_args(self, history, stream, tools, **kwargs):
        completion_args = {
            "model": self.model_name,
            "messages": history,
            "api_key": self.api_key,
            **kwargs,
        }

        # Provider-specific configuration
        if self.provider == SupportedLiteLLMProvider.Bedrock:
            completion_args.update({
                "aws_access_key_id": self.bedrock_ak,
                "aws_secret_access_key": self.bedrock_sk,
                "aws_region_name": self.bedrock_region,
            })

        return completion_args
```

## LLMBundle Wrapper

```python
class LLMBundle(LLM4Tenant):
    """
    High-level wrapper combining:
    - Model instance (from factory)
    - Token tracking
    - Langfuse observability
    - Text truncation safety
    """

    def encode(self, texts: list):
        # Safe text truncation
        safe_texts = []
        for text in texts:
            token_size = num_tokens_from_string(text)
            if token_size > self.max_length:
                target_len = int(self.max_length * 0.95)
                safe_texts.append(text[:target_len])
            else:
                safe_texts.append(text)

        embeddings, used_tokens = self.mdl.encode(safe_texts)

        # Track usage in database
        TenantLLMService.increase_usage(
            self.tenant_id, self.llm_type, used_tokens
        )

        return embeddings, used_tokens
```

## Configuration

```python
# Environment variables
LM_TIMEOUT_SECONDS = 600          # OpenAI client timeout
LLM_MAX_RETRIES = 5               # Max retry attempts
LLM_BASE_DELAY = 2.0              # Base backoff delay

# Settings
CHAT_CFG = {
    "factory": "OpenAI",
    "api_key": os.getenv("OPENAI_API_KEY"),
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-3.5-turbo"
}
```

## Related Files

- `/rag/llm/chat_model.py` - Chat implementations
- `/rag/llm/embedding_model.py` - Embedding implementations
- `/rag/llm/rerank_model.py` - Reranking implementations
- `/api/db/services/llm_service.py` - LLMBundle wrapper
