# Model Configuration Reference

This document explains the JSON field conventions used in `conf/models/*.json` and `conf/all_models.json`, and the decimal vs. binary conventions used by different model vendors.

## Table of Contents

- [JSON Fields](#json-fields)
- [Field Relationship Diagram](#field-relationship-diagram)
- [Migration Note](#migration-note)
- [Decimal vs. Binary Conventions](#decimal-vs-binary-conventions)
- [Vendor Breakdown](#vendor-breakdown)
- [Aggregators & Platforms](#aggregators--platforms)
- [How to Add a New Model](#how-to-add-a-new-model)
- [How to Update an Existing Model](#how-to-update-an-existing-model)
- [Quick Reference](#quick-reference)
- [Troubleshooting](#troubleshooting)

---

## JSON Fields

Each model entry in a provider JSON file (`conf/models/<provider>.json`) or in the global catalog (`conf/all_models.json`) supports the following fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Canonical model identifier (e.g. `gpt-4o`, `claude-opus-4-8`). Must be unique within a provider file. |
| `context_length` | integer | No | Maximum **context window** in tokens — the total number of tokens (input + output) the model can process in a single request. Previously named `max_tokens` (until PR #17807). |
| `max_output` | integer | No | Maximum **output generation** in tokens — the upper bound for tokens the model will generate. It may be a fixed vendor limit, or dynamic (computed as `context_length - input_tokens`). See [Vendor Breakdown](#vendor-breakdown). |
| `model_types` | string[] | Yes | Capabilities of the model. Common values: `chat`, `vision`, `embedding`, `rerank`, `asr`, `tts`, `ocr`, `doc_parse`. |
| `thinking` | object | No | Extended-thinking configuration (see [Thinking Object](#thinking-object)). |
| `tools` | object | No | Tool-use capability (see [Tools Object](#tools-object)). |
| `class` | string | No | Provider-specific model class used to select the correct driver (e.g. `glm`, `kimi`). |
| `max_dimension` | integer | No | Maximum supported embedding dimension. Used by `embedding`-type models (e.g. `1536`). |
| `dimensions` | integer[] | No | Supported embedding dimensions (e.g. `[256, 512, 1024, 1536]`). When non-empty, a requested dimension must match one of these values. When empty `[]` (or omitted), any dimension up to `max_dimension` is accepted. |
| `batch_size` | integer | No | Maximum number of text inputs that can be submitted to the embedding API in a single request. Used by `embedding`-type models. Values come from each provider's official documentation; models with no documented provider limit use a conservative high cap. When omitted, no explicit cap is declared. |
| `alias` | string[] | No | Alternative names for the same model. Used for model lookup when a tenant refers to the model by an alias. **Must be unique across all models.** |
| `rank` | integer | No | Sort priority (lower = higher rank). Used when ordering model lists in the UI. |

### Example Entry

```json
{
  "name": "claude-opus-4-8",
  "context_length": 1000000,
  "max_output": 128000,
  "model_types": ["chat", "vision"],
  "thinking": {
    "default_value": true,
    "clear_thinking": true
  },
  "tools": {
    "support": true
  }
}
```

### Thinking Object

```jsonc
{
  "thinking": {
    "default_value": true,    // Whether thinking mode is enabled by default
    "clear_thinking": true    // Whether the API can disable thinking per-request
  }
}
```

### Tools Object

```jsonc
{
  "tools": {
    "support": true           // Whether the model supports function/tool calling
  }
}
```

---

## Field Relationship Diagram

```text
┌─────────────────────────────────────────────────────┐
│                  context_length                      │
│  (total context window: input + output combined)    │
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │           prompt tokens (input)              │    │
│  └─────────────────────────────────────────────┘    │
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │        max_output (generated tokens)         │    │
│  │  May be fixed OR dynamic (context - input)   │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

- `context_length` is the **total** budget (input + output).
- `max_output` is the **generation** budget alone.
- For most models, `max_output <= context_length`. Some vendors set them equal (output can fill the entire window).
- **Dynamic max_output**: Some models (e.g. Kimi K2.6) define max_output as `context_length - input_tokens`. In these cases, the configured `max_output` represents the upper bound; the actual available output decreases as the prompt grows.

---

## Migration Note

Before PR #17807, a single `max_tokens` field served double duty — it was documented as the context window but often used as the output cap at runtime. The split into `context_length` + `max_output` removes this ambiguity:

- **Old `max_tokens`** → used only as migration context; do not copy it blindly.
- **New `context_length`** → set the vendor-documented context window.
- **New `max_output`** → set the vendor-documented generation cap.

Every migrated model **must define both `context_length` and `max_output`**, each taken from the official vendor model specification.

---

## Decimal vs. Binary Conventions

Different vendors express context windows using different numerical conventions. **This configuration preserves the exact numbers from each vendor's official documentation**, even when vendors disagree on whether "128K" means 128,000 or 131,072.

### How to Identify

| Convention | Pattern | Example |
|---|---|---|
| **Decimal (base-10)** | Round numbers in powers of 10 | 128,000 · 200,000 · 400,000 · 1,000,000 |
| **Binary (base-2)** | Powers of 2 (exact) | 131,072 = 2^17 · 262,144 = 2^18 · 1,048,576 = 2^20 |

A quick test: if `n & (n-1) == 0`, the value is a power of 2 (binary). Otherwise, it is decimal.

---

## Vendor Breakdown

| Vendor | `context_length` convention | `max_output` convention | Source |
|---|---|---|---|
| **OpenAI** | Binary | Binary | [OpenAI Models](https://developers.openai.com/api/docs/models/) |
| **Anthropic** | Decimal (200K, 1M) | Binary (8K, 16K, 32K, 64K, 128K) | [Anthropic Docs](https://docs.anthropic.com/en/docs/about-claude/models) |
| **Google (Gemini)** | Binary (1M, 2M) | Binary (8K, 64K) | [Google AI Docs](https://ai.google.dev/gemini-api/docs/models/gemini) |
| **Google (Gemma)** | Binary | Binary | [Gemma Docs](https://ai.google.dev/gemma/docs) |
| **Meta (Llama)** | Binary | Binary | [Llama Model Cards](https://github.com/meta-llama/llama-models) |
| **DeepSeek** | Varies by model — binary (128K, 1M) | Varies by model — binary (8K, 32K, 64K, 384K) | [DeepSeek API Docs](https://api-docs.deepseek.com/) |
| **Alibaba (Qwen)** | Binary (32K, 128K, 256K, 1M) | Binary (8K, 16K, 32K, 64K) | [Alibaba Bailian Docs](https://help.aliyun.com/zh/model-studio/) |
| **Moonshot (Kimi)** | Binary (256K = 262144, 1M = 1048576) | Dynamic — up to `context_length - input_tokens` (API default 32768) | [Kimi API Docs](https://platform.kimi.com/docs/api/models-overview) |
| **Mistral** | Binary | Binary (= context_length) | [Mistral Docs](https://docs.mistral.ai/getting-started/models/models_overview/) |
| **NVIDIA** | Binary | Binary | [NVIDIA NIM Docs](https://build.nvidia.com/nemotron) |
| **xAI (Grok)** | Decimal (131K, 262K) | Decimal (128K, 131K) | [xAI Docs](https://docs.x.ai/docs/models) |
| **GLM (Zhipu)** | Decimal (128000, 200000, 204800, 1000000) | Decimal (4096, 16384, 96000, 128000) | [Zhipu AI Docs](https://docs.bigmodel.cn/cn/guide/start/model-overview) |
| **MiniMax** | Decimal (204800 = 200K) | Decimal (128000 = 128K) | [MiniMax Docs](https://platform.minimaxi.com/docs/guides/text-generation) |
| **Cohere** | Decimal (128K, 256K) | Decimal (4K, 8K, 32K, 64K) | [Cohere Docs](https://docs.cohere.com/docs/models) |
| **Baichuan** | Decimal (32K, 128K, 192K) | Decimal (8K) | [Baichuan Docs](https://platform.baichuan-ai.com/docs) |
| **Amazon (Bedrock / Nova)** | Decimal (128K, 300K) | Decimal (5K) | [AWS Bedrock Docs](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html) |
| **Perplexity** | Decimal (128K, 200K) | Binary (128K) | [Perplexity Docs](https://docs.perplexity.ai/docs/sonar/models) |
| **Tencent (Hunyuan)** | Decimal (32K, 131K, 262K) | Decimal (8K, 64K) | [Tencent Cloud Docs](https://cloud.tencent.com/document/product/1759) |
| **Xiaomi (MiMo)** | Binary (1M) | Binary (8K) | [MiMo Docs](https://huggingface.co/XiaomiMiMo) |
| **HuggingFace** | Varies (hosted models) | Varies | [HuggingFace Model Cards](https://huggingface.co/docs/hub/model-cards) |

### Key Takeaways

1. **Never round or convert** a value to match a different convention. If Anthropic says 200K, write `200000` — not `2097152` or `262144`.
2. **OpenAI, Google, Meta, NVIDIA, DeepSeek, Qwen, Kimi, Mistral** all use binary (powers of 2).
3. **Anthropic, xAI, GLM/Zhipu, MiniMax, Cohere, Baichuan, Amazon** use decimal (powers of 10, or vendor-specific round numbers).
4. **Some vendors mix conventions** within their own catalog (e.g. Anthropic uses decimal for context but binary for output).
5. **When in doubt**, check the official API documentation linked above. The number in this config should match the vendor's stated limit exactly.

---

## Aggregators & Platforms

The following providers are **aggregators** — they host models from multiple upstream creators. Their `context_length` / `max_output` values inherit from the underlying model, not from a native convention of their own. When updating an aggregator's model entry, refer to the upstream creator's documentation (see table above).

| Aggregator | Notes |
|---|---|
| **302ai** | Hosts OpenAI, Anthropic, Google, etc. |
| **Alibaba Cloud (Bailian)** | Hosts Qwen and third-party models |
| **Aliyun** | Chinese cloud platform |
| **AstraFlow** | Multi-provider aggregator |
| **Avian** | Multi-provider aggregator |
| **Baidu (Qianwen)** | Ernie + third-party models |
| **CometAPI** | Multi-provider aggregator |
| **DeepInfra** | Open-source model hosting |
| **FuturMix** | Multi-provider aggregator |
| **GiteeAI** | Chinese aggregator |
| **GreenPT** | GLM-based models |
| **Huawei Cloud** | Hosts GLM, Kimi, etc. |
| **JieKouAI** | Multi-provider aggregator |
| **LongCat** | Meituan's model platform |
| **N1N** | Multi-provider aggregator |
| **Novita** | Open-source model hosting |
| **OpenRouter** | Multi-provider router |
| **OrcaRouter** | Auto-routing layer |
| **PPIO** | Edge AI platform |
| **Qiniu** | Chinese cloud platform |
| **Replicate** | Open-source model hosting |
| **SiliconFlow** | Chinese aggregator |
| **TogetherAI** | Open-source model hosting |
| **TokenHub** | Multi-provider aggregator |
| **TokenPony** | Multi-provider aggregator |
| **Volcengine (Doubao)** | ByteDance's cloud (hosts Doubao + third-party) |

---

## How to Add a New Model

1. Determine the model's `context_length` (context window) and `max_output` (generation cap) from the **official API documentation**.
2. Use the exact number stated — do not convert between decimal and binary.
3. For `embedding`-type models, also determine `batch_size` — the provider's documented maximum number of inputs per request — and add it to the entry.
4. Add the entry to the appropriate `conf/models/<provider>.json` file.
5. If the model is also listed in `conf/all_models.json`, update that entry too (or add it).
6. Run `go test ./internal/entity/models/...` to verify the config loads correctly.

---

## How to Update an Existing Model

1. Find the latest official spec from the vendor's documentation.
2. Update `context_length` and/or `max_output` to match.
3. If the model is an embedding model, update `batch_size` to the provider's documented per-request input limit.
4. If the model appears in multiple provider files (e.g. DeepSeek models appear in `deepseek.json`, `ppio.json`, `qiniu.json`), update all copies.
5. Update `conf/all_models.json` if the model has an entry there.
6. Run `go test ./internal/entity/models/...` to verify.

---

## Quick Reference

### Common model_types Values

| Type | Description |
|---|---|
| `chat` | Text generation / conversation |
| `vision` | Image understanding (multimodal) |
| `embedding` | Text embedding vectors |
| `rerank` | Document re-ranking |
| `asr` | Automatic speech recognition (speech-to-text) |
| `tts` | Text-to-speech |
| `ocr` | Optical character recognition |
| `doc_parse` | Document parsing (PDF, DOCX, etc.) |

### Token Count Rule of Thumb

| Language | Tokens per character |
|---|---|
| English | ~0.3 tokens/char (1 token ≈ 4 chars) |
| Chinese | ~0.6 tokens/char (1 token ≈ 1.5 chars) |
| Code | ~0.4 tokens/char |

Example: A 10,000-character English document ≈ 3,000 tokens.

### Validation Command

```bash
go test ./internal/entity/models/...
```

This loads all provider configs and `conf/all_models.json`, checking for:
- Valid JSON syntax
- Unique aliases across all models
- Correct field types

---

## Troubleshooting

### Duplicate Alias Error

```
InitProviderManager: duplicate alias "X" for models "A" and "B"
```

**Cause**: Two models share the same alias. Aliases must be globally unique.

**Fix**: In `conf/all_models.json`, find the conflicting entries and remove or rename the duplicate alias. Also check `conf/models/*.json` files for the same alias.

### Model Not Found

**Cause**: Model name or alias mismatch between tenant configuration and provider catalog.

**Fix**: Check both `conf/all_models.json` (aliases) and the specific `conf/models/<provider>.json` for the model name.

### Context Length Mismatch

**Symptom**: API returns errors about exceeding context limits.

**Cause**: `context_length` in config does not match the vendor's actual limit.

**Fix**: Verify against official vendor documentation and update accordingly.
