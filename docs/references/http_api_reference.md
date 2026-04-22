---
sidebar_position: 4
slug: /http_api_reference
sidebar_custom_props: {
  categoryIcon: LucideGlobe
}
---
# HTTP API

A complete reference for RAGFlow's RESTful API. Before proceeding, please ensure you [have your RAGFlow API key ready for authentication](https://ragflow.io/docs/dev/acquire_ragflow_api_key).

---

## ERROR CODES

---

| Code | Message               | Description                |
|------|-----------------------|----------------------------|
| 400  | Bad Request           | Invalid request parameters |
| 401  | Unauthorized          | Unauthorized access        |
| 403  | Forbidden             | Access denied              |
| 404  | Not Found             | Resource not found         |
| 500  | Internal Server Error | Server internal error      |
| 1001 | Invalid Chunk ID      | Invalid Chunk ID           |
| 1002 | Chunk Update Failed   | Chunk update failed        |

---

## OpenAI-Compatible API

---

### Create chat completion

**POST** `/api/v1/chats_openai/{chat_id}/chat/completions`

Creates a model response for a given chat conversation.

This API follows the same request and response format as OpenAI's API. It allows you to interact with the model in a manner similar to how you would with [OpenAI's API](https://platform.openai.com/docs/api-reference/chat/create).

#### Request

- Method: POST
- URL: `/api/v1/chats_openai/{chat_id}/chat/completions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"model"`: `string`
  - `"messages"`: `object list`
  - `"stream"`: `boolean`
  - `"extra_body"`: `object` (optional)

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chats_openai/{chat_id}/chat/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
        "model": "model",
        "messages": [{"role": "user", "content": "Say this is a test!"}],
        "stream": true,
        "extra_body": {
          "reference": true,
          "reference_metadata": {
            "include": true,
            "fields": ["author", "year", "source"]
          },
          "metadata_condition": {
            "logic": "and",
            "conditions": [
              {
                "name": "author",
                "comparison_operator": "is",
                "value": "bob"
              }
            ]
          }
        }
      }'
```

##### Request Parameters

- `model` (*Body parameter*) `string`, *Required*  
  The model used to generate the response. The server will parse this automatically, so you can set it to any value for now.

- `messages` (*Body parameter*) `list[object]`, *Required*  
  A list of historical chat messages used to generate the response. This must contain at least one message with the `user` role.

- `stream` (*Body parameter*) `boolean`  
  Whether to receive the response as a stream. Set this to `false` explicitly if you prefer to receive the entire response in one go instead of as a stream.

- `extra_body` (*Body parameter*) `object`  
  Extra request parameters:  
  - `reference`: `boolean` - include reference in the final chunk (stream) or in the final message (non-stream).
  - `reference_metadata`: `object` - include document metadata in each reference chunk.
    - `include`: `boolean` - enable document metadata in reference chunks.
    - `fields`: `list[string]` - optional allowlist of metadata keys. Omit to include all. Use an empty list to include none.
  - `metadata_condition`: `object` - metadata filter conditions applied to retrieval results.

#### Response

Stream:

```json
data:{
    "id": "chatcmpl-3b0397f277f511f0b47f729e3aa55728",
    "choices": [
        {
            "delta": {
                "content": "Hello! It seems like you're just greeting me. If you have a specific",
                "role": "assistant",
                "function_call": null,
                "tool_calls": null,
                "reasoning_content": null
            },
            "finish_reason": null,
            "index": 0,
            "logprobs": null
        }
    ],
    "created": 1755084508,
    "model": "model",
    "object": "chat.completion.chunk",
    "system_fingerprint": "",
    "usage": null
}

data:{"id": "chatcmpl-3b0397f277f511f0b47f729e3aa55728", "choices": [{"delta": {"content": " question or need information, feel free to ask, and I'll do my best", "role": "assistant", "function_call": null, "tool_calls": null, "reasoning_content": null}, "finish_reason": null, "index": 0, "logprobs": null}], "created": 1755084508, "model": "model", "object": "chat.completion.chunk", "system_fingerprint": "", "usage": null}

data:{"id": "chatcmpl-3b0397f277f511f0b47f729e3aa55728", "choices": [{"delta": {"content": " to assist you based on the knowledge base provided.", "role": "assistant", "function_call": null, "tool_calls": null, "reasoning_content": null}, "finish_reason": null, "index": 0, "logprobs": null}], "created": 1755084508, "model": "model", "object": "chat.completion.chunk", "system_fingerprint": "", "usage": null}

data:{"id": "chatcmpl-3b0397f277f511f0b47f729e3aa55728", "choices": [{"delta": {"content": null, "role": "assistant", "function_call": null, "tool_calls": null, "reasoning_content": null}, "finish_reason": "stop", "index": 0, "logprobs": null}], "created": 1755084508, "model": "model", "object": "chat.completion.chunk", "system_fingerprint": "", "usage": {"prompt_tokens": 5, "completion_tokens": 188, "total_tokens": 193}}

data:[DONE]
```

Non-stream:

```json
{
    "choices": [
        {
            "finish_reason": "stop",
            "index": 0,
            "logprobs": null,
            "message": {
                "content": "Hello! I'm your smart assistant. What can I do for you?",
                "role": "assistant"
            }
        }
    ],
    "created": 1755084403,
    "id": "chatcmpl-3b0397f277f511f0b47f729e3aa55728",
    "model": "model",
    "object": "chat.completion",
    "usage": {
        "completion_tokens": 55,
        "completion_tokens_details": {
            "accepted_prediction_tokens": 55,
            "reasoning_tokens": 5,
            "rejected_prediction_tokens": 0
        },
        "prompt_tokens": 5,
        "total_tokens": 60
    }
}
```

Failure:

```json
{
  "code": 102,
  "message": "The last content of this conversation is not from user."
}
```

---

### Create agent completion

**POST** `/api/v1/agents_openai/{agent_id}/chat/completions`

Creates a model response for a given chat conversation.

This API follows the same request and response format as OpenAI's API. It allows you to interact with the model in a manner similar to how you would with [OpenAI's API](https://platform.openai.com/docs/api-reference/chat/create).

#### Request

- Method: POST
- URL: `/api/v1/agents_openai/{agent_id}/chat/completions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"model"`: `string`
  - `"messages"`: `object list`
  - `"stream"`: `boolean`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/agents_openai/{agent_id}/chat/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
        "model": "model",
        "messages": [{"role": "user", "content": "Say this is a test!"}],
        "stream": true
      }'
```

##### Request Parameters

- `model` (*Body parameter*) `string`, *Required*  
  The model used to generate the response. The server will parse this automatically, so you can set it to any value for now.

- `messages` (*Body parameter*) `list[object]`, *Required*  
  A list of historical chat messages used to generate the response. This must contain at least one message with the `user` role.

- `stream` (*Body parameter*) `boolean`  
  Whether to receive the response as a stream. Set this to `false` explicitly if you prefer to receive the entire response in one go instead of as a stream.

- `session_id` (*Body parameter*) `string`  
  Agent session id.

#### Response

Stream:

```json
...

data: {
    "id": "c39f6f9c83d911f0858253708ecb6573",
    "object": "chat.completion.chunk",
    "model": "d1f79142831f11f09cc51795b9eb07c0",
    "choices": [
        {
            "delta": {
                "content": " terminal"
            },
            "finish_reason": null,
            "index": 0
        }
    ]
}

data: {
    "id": "c39f6f9c83d911f0858253708ecb6573",
    "object": "chat.completion.chunk",
    "model": "d1f79142831f11f09cc51795b9eb07c0",
    "choices": [
        {
            "delta": {
                "content": "."
            },
            "finish_reason": null,
            "index": 0
        }
    ]
}

data: {
    "id": "c39f6f9c83d911f0858253708ecb6573",
    "object": "chat.completion.chunk",
    "model": "d1f79142831f11f09cc51795b9eb07c0",
    "choices": [
        {
            "delta": {
                "content": "",
                "reference": {
                    "chunks": {
                        "20": {
                            "id": "4b8935ac0a22deb1",
                            "content": "```cd /usr/ports/editors/neovim/ && make install```## Android[Termux](https://github.com/termux/termux-app) offers a Neovim package.",
                            "document_id": "4bdd2ff65e1511f0907f09f583941b45",
                            "document_name": "INSTALL22.md",
                            "document_metadata": {
                                "author": "bob",
                                "year": "2023",
                                "source": "internal"
                            },
                            "dataset_id": "456ce60c5e1511f0907f09f583941b45",
                            "image_id": "",
                            "positions": [
                                [
                                    12,
                                    11,
                                    11,
                                    11,
                                    11
                                ]
                            ],
                            "url": null,
                            "similarity": 0.5697155305154673,
                            "vector_similarity": 0.7323851005515574,
                            "term_similarity": 0.5000000005,
                            "doc_type": ""
                        }
                    },
                    "doc_aggs": {
                        "INSTALL22.md": {
                            "doc_name": "INSTALL22.md",
                            "doc_id": "4bdd2ff65e1511f0907f09f583941b45",
                            "count": 3
                        },
                        "INSTALL.md": {
                            "doc_name": "INSTALL.md",
                            "doc_id": "4bd7fdd85e1511f0907f09f583941b45",
                            "count": 2
                        },
                        "INSTALL(1).md": {
                            "doc_name": "INSTALL(1).md",
                            "doc_id": "4bdfb42e5e1511f0907f09f583941b45",
                            "count": 2
                        },
                        "INSTALL3.md": {
                            "doc_name": "INSTALL3.md",
                            "doc_id": "4bdab5825e1511f0907f09f583941b45",
                            "count": 1
                        }
                    }
                }
            },
            "finish_reason": null,
            "index": 0
        }
    ]
}

data: [DONE]
```

Non-stream:

```json
{
    "choices": [
        {
            "finish_reason": "stop",
            "index": 0,
            "logprobs": null,
            "message": {
                "content": "\nTo install Neovim, the process varies depending on your operating system:\n\n### For Windows:\n1. **Download from GitHub**: \n   - Visit the [Neovim releases page](https://github.com/neovim/neovim/releases)\n   - Download the latest Windows installer (nvim-win64.msi)\n   - Run the installer and follow the prompts\n\n2. **Using winget** (Windows Package Manager):\n...",
                "reference": {
                    "chunks": {
                        "20": {
                            "content": "```cd /usr/ports/editors/neovim/ && make install```## Android[Termux](https://github.com/termux/termux-app) offers a Neovim package.",
                            "dataset_id": "456ce60c5e1511f0907f09f583941b45",
                            "doc_type": "",
                            "document_id": "4bdd2ff65e1511f0907f09f583941b45",
                            "document_name": "INSTALL22.md",
                            "document_metadata": {
                                "author": "bob",
                                "year": "2023",
                                "source": "internal"
                            },
                            "id": "4b8935ac0a22deb1",
                            "image_id": "",
                            "positions": [
                                [
                                    12,
                                    11,
                                    11,
                                    11,
                                    11
                                ]
                            ],
                            "similarity": 0.5697155305154673,
                            "term_similarity": 0.5000000005,
                            "url": null,
                            "vector_similarity": 0.7323851005515574
                        }
                    },
                    "doc_aggs": {
                        "INSTALL(1).md": {
                            "count": 2,
                            "doc_id": "4bdfb42e5e1511f0907f09f583941b45",
                            "doc_name": "INSTALL(1).md"
                        },
                        "INSTALL.md": {
                            "count": 2,
                            "doc_id": "4bd7fdd85e1511f0907f09f583941b45",
                            "doc_name": "INSTALL.md"
                        },
                        "INSTALL22.md": {
                            "count": 3,
                            "doc_id": "4bdd2ff65e1511f0907f09f583941b45",
                            "doc_name": "INSTALL22.md"
                        },
                        "INSTALL3.md": {
                            "count": 1,
                            "doc_id": "4bdab5825e1511f0907f09f583941b45",
                            "doc_name": "INSTALL3.md"
                        }
                    }
                },
                "role": "assistant"
            }
        }
    ],
    "created": null,
    "id": "c39f6f9c83d911f0858253708ecb6573",
    "model": "d1f79142831f11f09cc51795b9eb07c0",
    "object": "chat.completion",
    "param": null,
    "usage": {
        "completion_tokens": 415,
        "completion_tokens_details": {
            "accepted_prediction_tokens": 0,
            "reasoning_tokens": 0,
            "rejected_prediction_tokens": 0
        },
        "prompt_tokens": 6,
        "total_tokens": 421
    }
}
```

Failure:

```json
{
  "code": 102,
  "message": "The last content of this conversation is not from user."
}
```

## DATASET MANAGEMENT

---

### Create dataset

**POST** `/api/v1/datasets`

Creates a dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"description"`: `string`
  - `"embedding_model"`: `string`
  - `"permission"`: `string`
  - `"chunk_method"`: `string`
  - `"parser_config"`: `object`
  - `"parse_type"`: `int`
  - `"pipeline_id"`: `string`

##### A basic request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
      "name": "test_1"
      }'
```

##### A request example specifying ingestion pipeline

:::caution WARNING
You must *not* include `"chunk_method"` or `"parser_config"` when specifying an ingestion pipeline.
:::

```bash
curl --request POST \
  --url http://{address}/api/v1/datasets \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer <YOUR_API_KEY>' \
  --data '{
   "name": "test-sdk",
   "parse_type": <NUMBER_OF_PARSERS_IN_YOUR_PARSER_COMPONENT>,
   "pipeline_id": "<PIPELINE_ID_32_HEX>"
  }'
```

##### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*  
  The unique name of the dataset to create. It must adhere to the following requirements:  
  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters
  - Case-insensitive

- `"avatar"`: (*Body parameter*), `string`  
  Base64 encoding of the avatar.
  - Maximum 65535 characters

- `"description"`: (*Body parameter*), `string`  
  A brief description of the dataset to create.
  - Maximum 65535 characters

- `"embedding_model"`: (*Body parameter*), `string`  
  The name of the embedding model to use. For example: `"BAAI/bge-large-zh-v1.5@BAAI"`
  - Maximum 255 characters
  - Must follow `model_name@model_factory` format

- `"permission"`: (*Body parameter*), `string`  
  Specifies who can access the dataset to create. Available options:  
  - `"me"`: (Default) Only you can manage the dataset.
  - `"team"`: All team members can manage the dataset.

- `"chunk_method"`: (*Body parameter*), `enum<string>`  
  The default chunk method of the dataset to create. Mutually exclusive with `"parse_type"` and `"pipeline_id"`. If you set `"chunk_method"`, do not include `"parse_type"` or `"pipeline_id"`.  
  Available options:  
  - `"naive"`: General (default)
  - `"book"`: Book
  - `"email"`: Email
  - `"laws"`: Laws
  - `"manual"`: Manual
  - `"one"`: One
  - `"paper"`: Paper
  - `"picture"`: Picture
  - `"presentation"`: Presentation
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"tag"`: Tag

- `"parser_config"`: (*Body parameter*), `object`  
  The configuration settings for the dataset parser. The attributes in this JSON object vary with the selected `"chunk_method"`:  
  - If `"chunk_method"` is `"naive"`, the `"parser_config"` object contains the following attributes:
    - `"auto_keywords"`: `int`
      - Defaults to `0`
      - Minimum: `0`
      - Maximum: `32`
    - `"auto_questions"`: `int`
      - Defaults to `0`
      - Minimum: `0`
      - Maximum: `10`
    - `"chunk_token_num"`: `int`
      - Defaults to `512`
      - Minimum: `1`
      - Maximum: `2048`
    - `"delimiter"`: `string`
      - Defaults to `"\n"`.
    - `"html4excel"`: `bool`
      - Whether to convert Excel documents into HTML format.
      - Defaults to `false`
    - `"layout_recognize"`: `string`
      - Defaults to `DeepDOC`
    - `"tag_kb_ids"`: `array<string>`
      - IDs of datasets to be parsed using the ​​Tag chunk method.
      - Before setting this, ensure a tag set is created and properly configured. For details, see [Use tag set](https://ragflow.io/docs/dev/use_tag_sets).
    - `"task_page_size"`: `int`
      - For PDFs only.
      - Defaults to `12`
      - Minimum: `1`
    - `"raptor"`: `object` RAPTOR-specific settings.
      - Defaults to: `{"use_raptor": false}`
    - `"graphrag"`: `object` GRAPHRAG-specific settings.
      - Defaults to: `{"use_graphrag": false}`
    - `"parent_child"`: `object` Parent-child chunking settings. When enabled, each chunk is further split into smaller child chunks using `children_delimiter`. At retrieval time, matched child chunks are replaced by their parent's full text before being passed to the LLM, giving precise vector matching with broader context.
      - `"use_parent_child"`: `bool` Whether to enable parent-child chunking. Defaults to `false`.
      - `"children_delimiter"`: `string` The delimiter used to split a parent chunk into child chunks. Only takes effect when `"use_parent_child"` is `true`. Defaults to `"\n"`.
  - If `"chunk_method"` is `"qa"`, `"manuel"`, `"paper"`, `"book"`, `"laws"`, or `"presentation"`, the `"parser_config"` object contains the following attribute:  
    - `"raptor"`: `object` RAPTOR-specific settings.
      - Defaults to: `{"use_raptor": false}`.
  - If `"chunk_method"` is `"table"`, `"picture"`, `"one"`, or `"email"`, `"parser_config"` is an empty JSON object.

- `"parse_type"`: (*Body parameter*), `int`  
  The ingestion pipeline parse type identifier, i.e., the number of parsers in your **Parser** component.  
  - Required (along with `"pipeline_id"`) if specifying an ingestion pipeline.
  - Must not be included when `"chunk_method"` is specified.

- `"pipeline_id"`: (*Body parameter*), `string`  
  The ingestion pipeline ID. Can be found in the corresponding URL in the RAGFlow UI.
  - Required (along with `"parse_type"`) if specifying an ingestion pipeline.
  - Must be a 32-character lowercase hexadecimal string, e.g., `"d0bebe30ae2211f0970942010a8e0005"`.
  - Must not be included when `"chunk_method"` is specified.

:::caution WARNING
You can choose either of the following ingestion options when creating a dataset, but *not* both:

- Use a built-in chunk method -- specify `"chunk_method"` (optionally with `"parser_config"`).
- Use an ingestion pipeline -- specify both `"parse_type"` and `"pipeline_id"`.

If none of `"chunk_method"`, `"parse_type"`, or `"pipeline_id"` are provided, the system defaults to `chunk_method = "naive"`.
:::

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "chunk_count": 0,
        "chunk_method": "naive",
        "create_date": "Mon, 28 Apr 2025 18:40:41 GMT",
        "create_time": 1745836841611,
        "created_by": "3af81804241d11f0a6a79f24fc270c7f",
        "description": null,
        "document_count": 0,
        "embedding_model": "BAAI/bge-large-zh-v1.5@BAAI",
        "id": "3b4de7d4241d11f0a6a79f24fc270c7f",
        "language": "English",
        "name": "RAGFlow example",
        "pagerank": 0,
        "parser_config": {
            "chunk_token_num": 128, 
            "delimiter": "\\n!?;。；！？", 
            "html4excel": false, 
            "layout_recognize": "DeepDOC", 
            "raptor": {
                "use_raptor": false
                }
            },
        "permission": "me",
        "similarity_threshold": 0.2,
        "status": "1",
        "tenant_id": "3af81804241d11f0a6a79f24fc270c7f",
        "token_num": 0,
        "update_date": "Mon, 28 Apr 2025 18:40:41 GMT",
        "update_time": 1745836841611,
        "vector_similarity_weight": 0.3,
    },
}
```

Failure:

```json
{
    "code": 101,
    "message": "Field: <name> - Message: <String should have at least 1 character> - Value: <>"
}
```

---

### Delete datasets

**DELETE** `/api/v1/datasets`

Deletes datasets by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/datasets`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]` or `null`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
     "ids": ["d94a8dc02c9711f0930f7fbc369eab6d", "e94a8dc02c9711f0930f7fbc369eab6e"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
     "delete_all": true
     }'
```

##### Request parameters

- `"ids"`: (*Body parameter*), `list[string]` or `null`
  Specifies the datasets to delete:
  - If omitted, or set to `null` or an empty array, no datasets are deleted.
  - If an array of IDs is provided, only the datasets matching those IDs are deleted.
- `"delete_all"`: (*Body parameter*), `boolean`  
  Whether to delete all datasets owned by the current user when`"ids"` is omitted, or set to `null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0 
}
```

Failure:

```json
{
    "code":108,
    "message":"User '<tenant_id>' lacks permission for datasets: '<dataset_ids>'"
}

```

---

### Update dataset

**PUT** `/api/v1/datasets/{dataset_id}`

Updates configurations for a specified dataset.

#### Request

- Method: PUT
- URL: `/api/v1/datasets/{dataset_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"description"`: `string`
  - `"embedding_model"`: `string`
  - `"permission"`: `string`
  - `"chunk_method"`: `string`
  - `"pagerank"`: `int`
  - `"parser_config"`: `object`

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/datasets/{dataset_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "name": "updated_dataset"
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the dataset to update.
- `"name"`: (*Body parameter*), `string`  
  The revised name of the dataset.
  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters
  - Case-insensitive
- `"avatar"`: (*Body parameter*), `string`  
  The updated base64 encoding of the avatar.
  - Maximum 65535 characters
- `"embedding_model"`: (*Body parameter*), `string`  
  The updated embedding model name.  
  - Ensure that `"chunk_count"` is `0` before updating `"embedding_model"`.
  - Maximum 255 characters
  - Must follow `model_name@model_factory` format
- `"permission"`: (*Body parameter*), `string`  
  The updated dataset permission. Available options:  
  - `"me"`: (Default) Only you can manage the dataset.
  - `"team"`: All team members can manage the dataset.
- `"pagerank"`: (*Body parameter*), `int`  
  refer to [Set page rank](https://ragflow.io/docs/dev/set_page_rank)
  - Default: `0`
  - Minimum: `0`
  - Maximum: `100`
- `"chunk_method"`: (*Body parameter*), `enum<string>`  
  The chunking method for the dataset. Available options:  
  - `"naive"`: General (default)
  - `"book"`: Book
  - `"email"`: Email
  - `"laws"`: Laws
  - `"manual"`: Manual
  - `"one"`: One
  - `"paper"`: Paper
  - `"picture"`: Picture
  - `"presentation"`: Presentation
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"tag"`: Tag
- `"parser_config"`: (*Body parameter*), `object`  
  The configuration settings for the dataset parser. The attributes in this JSON object vary with the selected `"chunk_method"`:  
  - If `"chunk_method"` is `"naive"`, the `"parser_config"` object contains the following attributes:
    - `"auto_keywords"`: `int`
      - Defaults to `0`
      - Minimum: `0`
      - Maximum: `32`
    - `"auto_questions"`: `int`
      - Defaults to `0`
      - Minimum: `0`
      - Maximum: `10`
    - `"chunk_token_num"`: `int`
      - Defaults to `512`
      - Minimum: `1`
      - Maximum: `2048`
    - `"delimiter"`: `string`
      - Defaults to `"\n"`.
    - `"html4excel"`: `bool` Indicates whether to convert Excel documents into HTML format.
      - Defaults to `false`
    - `"layout_recognize"`: `string`
      - Defaults to `DeepDOC`
    - `"tag_kb_ids"`: `array<string>` refer to [Use tag set](https://ragflow.io/docs/dev/use_tag_sets)
      - Must include a list of dataset IDs, where each dataset is parsed using the ​​Tag Chunking Method
    - `"task_page_size"`: `int` For PDF only.
      - Defaults to `12`
      - Minimum: `1`
    - `"raptor"`: `object` RAPTOR-specific settings.
      - Defaults to: `{"use_raptor": false}`
    - `"graphrag"`: `object` GRAPHRAG-specific settings.
      - Defaults to: `{"use_graphrag": false}`
    - `"parent_child"`: `object` Parent-child chunking settings. When enabled, each chunk is further split into smaller child chunks using `children_delimiter`. At retrieval time, matched child chunks are replaced by their parent's full text before being passed to the LLM, giving precise vector matching with broader context.
      - `"use_parent_child"`: `bool` Whether to enable parent-child chunking. Defaults to `false`.
      - `"children_delimiter"`: `string` The delimiter used to split a parent chunk into child chunks. Only takes effect when `"use_parent_child"` is `true`. Defaults to `"\n"`.
  - If `"chunk_method"` is `"qa"`, `"manuel"`, `"paper"`, `"book"`, `"laws"`, or `"presentation"`, the `"parser_config"` object contains the following attribute:  
    - `"raptor"`: `object` RAPTOR-specific settings.
      - Defaults to: `{"use_raptor": false}`.
  - If `"chunk_method"` is `"table"`, `"picture"`, `"one"`, or `"email"`, `"parser_config"` is an empty JSON object.

#### Response

Success:

```json
{
    "code": 0 
}
```

Failure:

```json
{
    "code": 102,
    "message": "Can't change tenant_id."
}
```

---

### List datasets

**GET** `/api/v1/datasets?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}&include_parsing_status={include_parsing_status}`

Lists datasets.

#### Request

- Method: GET
- URL: `/api/v1/datasets?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}&include_parsing_status={include_parsing_status}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

```bash
# List datasets with parsing status
curl --request GET \
     --url 'http://{address}/api/v1/datasets?include_parsing_status=true' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `page`: (*Filter parameter*)  
  Specifies the page on which the datasets will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*)  
  The number of datasets on each page. Defaults to `30`.
- `orderby`: (*Filter parameter*)  
  The field by which datasets should be sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*)  
  Indicates whether the retrieved datasets should be sorted in descending order. Defaults to `true`.
- `name`: (*Filter parameter*)  
  The name of the dataset to retrieve.
- `id`: (*Filter parameter*)  
  The ID of the dataset to retrieve.
- `include_parsing_status`: (*Filter parameter*)  
  Whether to include document parsing status counts in the response. Defaults to `false`. When set to `true`, each dataset object in the response will include the following additional fields:
  - `unstart_count`: Number of documents not yet started parsing.
  - `running_count`: Number of documents currently being parsed.
  - `cancel_count`: Number of documents whose parsing was cancelled.
  - `done_count`: Number of documents that have been successfully parsed.
  - `fail_count`: Number of documents whose parsing failed.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "avatar": "",
            "chunk_count": 59,
            "create_date": "Sat, 14 Sep 2024 01:12:37 GMT",
            "create_time": 1726276357324,
            "created_by": "69736c5e723611efb51b0242ac120007",
            "description": null,
            "document_count": 1,
            "embedding_model": "BAAI/bge-large-zh-v1.5",
            "id": "6e211ee0723611efa10a0242ac120007",
            "language": "English",
            "name": "mysql",
            "chunk_method": "naive",
            "parser_config": {
                "chunk_token_num": 8192,
                "delimiter": "\\n",
                "entity_types": [
                    "organization",
                    "person",
                    "location",
                    "event",
                    "time"
                ]
            },
            "permission": "me",
            "similarity_threshold": 0.2,
            "status": "1",
            "tenant_id": "69736c5e723611efb51b0242ac120007",
            "token_num": 12744,
            "update_date": "Thu, 10 Oct 2024 04:07:23 GMT",
            "update_time": 1728533243536,
            "vector_similarity_weight": 0.3
        }
    ],
    "total_datasets": 1
}
```

Success (with `include_parsing_status=true`):

```json
{
    "code": 0,
    "data": [
        {
            "avatar": null,
            "cancel_count": 0,
            "chunk_count": 30,
            "chunk_method": "qa",
            "create_date": "2026-03-09T18:57:13",
            "create_time": 1773053833094,
            "created_by": "928f92a210b911f1ac4cc39e0b8fa3ad",
            "description": null,
            "document_count": 1,
            "done_count": 1,
            "embedding_model": "text-embedding-v2@Tongyi-Qianwen",
            "fail_count": 0,
            "id": "ba6586c21ba611f1a3dc476f0709e75e",
            "language": "English",
            "name": "Test Dataset",
            "parser_config": {
                "graphrag": { "use_graphrag": false },
                "llm_id": "deepseek-chat@DeepSeek",
                "raptor": { "use_raptor": false }
            },
            "permission": "me",
            "running_count": 0,
            "similarity_threshold": 0.2,
            "status": "1",
            "tenant_id": "928f92a210b911f1ac4cc39e0b8fa3ad",
            "token_num": 1746,
            "unstart_count": 0,
            "update_date": "2026-03-09T18:59:32",
            "update_time": 1773053972723,
            "vector_similarity_weight": 0.3
        }
    ],
    "total_datasets": 1
}
```

Failure:

```json
{
    "code": 102,
    "message": "The dataset doesn't exist"
}
```

 ---

### Get knowledge graph

**GET** `/api/v1/datasets/{dataset_id}/knowledge_graph`

Retrieves the knowledge graph of a specified dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/knowledge_graph`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/knowledge_graph \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "graph": {
            "directed": false,
            "edges": [
                {
                    "description": "The notice is a document issued to convey risk warnings and operational alerts.<SEP>The notice is a specific instance of a notification document issued under the risk warning framework.",
                    "keywords": ["9", "8"],
                    "source": "notice",
                    "source_id": ["8a46cdfe4b5c11f0a5281a58e595aa1c"],
                    "src_id": "xxx",
                    "target": "xxx",
                    "tgt_id": "xxx",
                    "weight": 17.0
                }
            ],
            "graph": {
                "source_id": ["8a46cdfe4b5c11f0a5281a58e595aa1c", "8a7eb6424b5c11f0a5281a58e595aa1c"]
            },
            "multigraph": false,
            "nodes": [
                {
                    "description": "xxx",
                    "entity_name": "xxx",
                    "entity_type": "ORGANIZATION",
                    "id": "xxx",
                    "pagerank": 0.10804906590624092,
                    "rank": 3,
                    "source_id": ["8a7eb6424b5c11f0a5281a58e595aa1c"]
                }
            ]
        },
        "mind_map": {}
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "The dataset doesn't exist"
}
```

---

### Delete knowledge graph

**DELETE** `/api/v1/datasets/{dataset_id}/knowledge_graph`

Removes the knowledge graph of a specified dataset.

#### Request

- Method: DELETE
- URL: `/api/v1/datasets/{dataset_id}/knowledge_graph`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/knowledge_graph \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 102,
    "message": "The dataset doesn't exist"
}
```

---

### Construct knowledge graph

**POST** `/api/v1/datasets/{dataset_id}/run_graphrag`

Constructs a knowledge graph from a specified dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/run_graphrag`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/run_graphrag \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code":0,
    "data":{
      "graphrag_task_id":"e498de54bfbb11f0ba028f704583b57b"
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Invalid Dataset ID"
}
```

---

### Get knowledge graph construction status

**GET** `/api/v1/datasets/{dataset_id}/trace_graphrag`

Retrieves the knowledge graph construction status for a specified dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/trace_graphrag`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/trace_graphrag \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code":0,
    "data":{
        "begin_at":"Wed, 12 Nov 2025 19:36:56 GMT",
        "chunk_ids":"",
        "create_date":"Wed, 12 Nov 2025 19:36:56 GMT",
        "create_time":1762947416350,
        "digest":"39e43572e3dcd84f",
        "doc_id":"44661c10bde211f0bc93c164a47ffc40",
        "from_page":100000000,
        "id":"e498de54bfbb11f0ba028f704583b57b",
        "priority":0,
        "process_duration":2.45419,
        "progress":1.0,
        "progress_msg":"19:36:56 created task graphrag\n19:36:57 Task has been received.\n19:36:58 [GraphRAG] doc:083661febe2411f0bc79456921e5745f has no available chunks, skip generation.\n19:36:58 [GraphRAG] build_subgraph doc:44661c10bde211f0bc93c164a47ffc40 start (chunks=1, timeout=10000000000s)\n19:36:58 Graph already contains 44661c10bde211f0bc93c164a47ffc40\n19:36:58 [GraphRAG] build_subgraph doc:44661c10bde211f0bc93c164a47ffc40 empty\n19:36:58 [GraphRAG] kb:33137ed0bde211f0bc93c164a47ffc40 no subgraphs generated successfully, end.\n19:36:58 Knowledge Graph done (0.72s)","retry_count":1,
        "task_type":"graphrag",
        "to_page":100000000,
        "update_date":"Wed, 12 Nov 2025 19:36:58 GMT",
        "update_time":1762947418454
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Invalid Dataset ID"
}
```

---

### Construct RAPTOR

**POST** `/api/v1/datasets/{dataset_id}/run_raptor`

Construct a RAPTOR from a specified dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/run_raptor`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/run_raptor \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code":0,
    "data":{
        "raptor_task_id":"50d3c31cbfbd11f0ba028f704583b57b"
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Invalid Dataset ID"
}
```

---

### Get RAPTOR construction status

**GET** `/api/v1/datasets/{dataset_id}/trace_raptor`

Retrieves the RAPTOR construction status for a specified dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/trace_raptor`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/trace_raptor \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the target dataset.

#### Response

Success:

```json
{
    "code":0,
    "data":{
        "begin_at":"Wed, 12 Nov 2025 19:47:07 GMT",
        "chunk_ids":"",
        "create_date":"Wed, 12 Nov 2025 19:47:07 GMT",
        "create_time":1762948027427,
        "digest":"8b279a6248cb8fc6",
        "doc_id":"44661c10bde211f0bc93c164a47ffc40",
        "from_page":100000000,
        "id":"50d3c31cbfbd11f0ba028f704583b57b",
        "priority":0,
        "process_duration":0.948244,
        "progress":1.0,
        "progress_msg":"19:47:07 created task raptor\n19:47:07 Task has been received.\n19:47:07 Processing...\n19:47:07 Processing...\n19:47:07 Indexing done (0.01s).\n19:47:07 Task done (0.29s)",
        "retry_count":1,
        "task_type":"raptor",
        "to_page":100000000,
        "update_date":"Wed, 12 Nov 2025 19:47:07 GMT",
        "update_time":1762948027948
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Invalid Dataset ID"
}
```

---

## FILE MANAGEMENT WITHIN DATASET

---

### Upload documents

**POST** `/api/v1/datasets/{dataset_id}/documents`

Uploads documents to a specified dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/documents`
- Headers:
  - `'Content-Type: multipart/form-data'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Form:
  - `'file=@{FILE_PATH}'`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --form 'file=@./test1.txt' \
     --form 'file=@./test2.pdf'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the dataset to which the documents will be uploaded.
- `'file'`: (*Body parameter*)  
  A document to upload.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "chunk_method": "naive",
            "created_by": "69736c5e723611efb51b0242ac120007",
            "dataset_id": "527fa74891e811ef9c650242ac120006",
            "id": "b330ec2e91ec11efbc510242ac120004",
            "location": "1.txt",
            "name": "1.txt",
            "parser_config": {
                "chunk_token_num": 128,
                "delimiter": "\\n",
                "html4excel": false,
                "layout_recognize": true,
                "raptor": {
                    "use_raptor": false
                }
            },
            "run": "UNSTART",
            "size": 17966,
            "thumbnail": "",
            "type": "doc"
        }
    ]
}
```

Failure:

```json
{
    "code": 101,
    "message": "No file part!"
}
```

---

### Update document

**PUT** `/api/v1/datasets/{dataset_id}/documents/{document_id}`

Updates configurations for a specified document.

#### Request

- Method: PUT
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`:`string`
  - `"meta_fields"`:`object`
  - `"chunk_method"`:`string`
  - `"parser_config"`:`object`

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --header 'Content-Type: application/json' \
     --data '
     {
          "name": "manual.txt", 
          "chunk_method": "manual", 
          "parser_config": {"chunk_token_num": 128}
     }'

```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the associated dataset.
- `document_id`: (*Path parameter*)  
  The ID of the document to update.
- `"name"`: (*Body parameter*), `string`
- `"meta_fields"`: (*Body parameter*), `dict[str, Any]` The meta fields of the document.
- `"chunk_method"`: (*Body parameter*), `string`  
  The parsing method to apply to the document:  
  - `"naive"`: General
  - `"manual`: Manual
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"paper"`: Paper
  - `"book"`: Book
  - `"laws"`: Laws
  - `"presentation"`: Presentation
  - `"picture"`: Picture
  - `"one"`: One
  - `"email"`: Email
- `"parser_config"`: (*Body parameter*), `object`  
  The configuration settings for the dataset parser. The attributes in this JSON object vary with the selected `"chunk_method"`:  
  - If `"chunk_method"` is `"naive"`, the `"parser_config"` object contains the following attributes:
    - `"chunk_token_num"`: Defaults to `256`.
    - `"layout_recognize"`: Defaults to `true`.
    - `"html4excel"`: Indicates whether to convert Excel documents into HTML format. Defaults to `false`.
    - `"delimiter"`: Defaults to `"\n"`.
    - `"task_page_size"`: Defaults to `12`. For PDF only.
    - `"raptor"`: RAPTOR-specific settings. Defaults to: `{"use_raptor": false}`.
  - If `"chunk_method"` is `"qa"`, `"manuel"`, `"paper"`, `"book"`, `"laws"`, or `"presentation"`, the `"parser_config"` object contains the following attribute:
    - `"raptor"`: RAPTOR-specific settings. Defaults to: `{"use_raptor": false}`.
  - If `"chunk_method"` is `"table"`, `"picture"`, `"one"`, or `"email"`, `"parser_config"` is an empty JSON object.
- `"enabled"`: (*Body parameter*), `integer`  
  Whether the document should be **available** in the knowledge base.  
  - `1` → （available）  
  - `0` → （unavailable）  

#### Response

Success:

```json
{
  "code": 0,
  "data": {
    "id": "cd38dd72d4a611f0af9c71de94a988ef",
    "name": "large.md",
    "type": "doc",
    "suffix": "md",
    "size": 2306906,
    "location": "large.md",
    "source_type": "local",
    "status": "1",
    "run": "DONE",
    "dataset_id": "5f546a1ad4a611f0af9c71de94a988ef",

    "chunk_method": "naive",
    "chunk_count": 2,
    "token_count": 8126,

    "created_by": "eab7f446cb5a11f0ab334fbc3aa38f35",
    "create_date": "Tue, 09 Dec 2025 10:28:52 GMT",
    "create_time": 1765247332122,
    "update_date": "Wed, 17 Dec 2025 10:51:16 GMT",
    "update_time": 1765939876819,

    "process_begin_at": "Wed, 17 Dec 2025 10:33:55 GMT",
    "process_duration": 14.8615,
    "progress": 1.0,

    "progress_msg": [
      "10:33:58 Task has been received.",
      "10:33:59 Page(1~100000001): Start to parse.",
      "10:33:59 Page(1~100000001): Finish parsing.",
      "10:34:07 Page(1~100000001): Generate 2 chunks",
      "10:34:09 Page(1~100000001): Embedding chunks (2.13s)",
      "10:34:09 Page(1~100000001): Indexing done (0.31s).",
      "10:34:09 Page(1~100000001): Task done (11.68s)"
    ],

    "parser_config": {
      "chunk_token_num": 512,
      "delimiter": "\n",
      "auto_keywords": 0,
      "auto_questions": 0,
      "topn_tags": 3,

      "layout_recognize": "DeepDOC",
      "html4excel": false,
      "image_context_size": 0,
      "table_context_size": 0,

      "graphrag": {
        "use_graphrag": true,
        "method": "light",
        "entity_types": [
          "organization",
          "person",
          "geo",
          "event",
          "category"
        ]
      },

      "raptor": {
        "use_raptor": true,
        "max_cluster": 64,
        "max_token": 256,
        "threshold": 0.1,
        "random_seed": 0,
        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize."
      }
    },

    "meta_fields": {},
    "pipeline_id": "",
    "thumbnail": ""
  }
}

```

Failure:

```json
{
    "code": 102,
    "message": "The dataset does not have the document."
}
```

---

### Download document

**GET** `/api/v1/datasets/{dataset_id}/documents/{document_id}`

Downloads a document from a specified dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Output:
  - `'{PATH_TO_THE_FILE}'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --output ./ragflow.txt
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `documents_id`: (*Path parameter*)  
  The ID of the document to download.

#### Response

Success:

```json
This is a test to verify the file download feature.
```

Failure:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```

---

### List documents

**GET** `/api/v1/datasets/{dataset_id}/documents?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&keywords={keywords}&id={document_id}&name={document_name}&create_time_from={timestamp}&create_time_to={timestamp}&suffix={file_suffix}&run={run_status}&metadata_condition={json}`

Lists documents in a specified dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/documents?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&keywords={keywords}&id={document_id}&name={document_name}&create_time_from={timestamp}&create_time_to={timestamp}&suffix={file_suffix}&run={run_status}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request examples

**A basic request with pagination:**

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents?page=1&page_size=10 \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `keywords`: (*Filter parameter*), `string`  
  The keywords used to match document titles.
- `page`: (*Filter parameter*), `integer`
  Specifies the page on which the documents will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The maximum number of documents on each page. Defaults to `30`.
- `orderby`: (*Filter parameter*), `string`  
  The field by which documents should be sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved documents should be sorted in descending order. Defaults to `true`.
- `id`: (*Filter parameter*), `string`  
  The ID of the document to retrieve.
- `create_time_from`: (*Filter parameter*), `integer`  
  Unix timestamp for filtering documents created after this time. 0 means no filter. Defaults to `0`.
- `create_time_to`: (*Filter parameter*), `integer`  
  Unix timestamp for filtering documents created before this time. 0 means no filter. Defaults to `0`.
- `suffix`: (*Filter parameter*), `array[string]`  
  Filter by file suffix. Supports multiple values, e.g., `pdf`, `txt`, and `docx`. Defaults to all suffixes.
- `run`: (*Filter parameter*), `array[string]`  
  Filter by document processing status. Supports numeric, text, and mixed formats:  
  - Numeric format: `["0", "1", "2", "3", "4"]`
  - Text format: `[UNSTART, RUNNING, CANCEL, DONE, FAIL]`
  - Mixed format: `[UNSTART, 1, DONE]` (mixing numeric and text formats)
  - Status mapping:
    - `0` / `UNSTART`: Document not yet processed
    - `1` / `RUNNING`: Document is currently being processed
    - `2` / `CANCEL`: Document processing was cancelled
    - `3` / `DONE`: Document processing completed successfully
    - `4` / `FAIL`: Document processing failed  
  Defaults to all statuses.
- `metadata_condition`: (*Filter parameter*), `object` (JSON in query)
  Optional metadata filter applied to documents when `document_ids` is not provided. Uses the same structure as retrieval:
  - `logic`: `"and"` (default) or `"or"`
  - `conditions`: array of `{ "name": string, "comparison_operator": string, "value": string }`
    - `comparison_operator` supports: `is`, `not is`, `contains`, `not contains`, `in`, `not in`, `start with`, `end with`, `>`, `<`, `≥`, `≤`, `empty`, `not empty`

##### Usage examples

**A request with multiple filtering parameters**

```bash
curl --request GET \
     --url 'http://{address}/api/v1/datasets/{dataset_id}/documents?suffix=pdf&run=DONE&page=1&page_size=10' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

**Filter by metadata (query JSON):**

```bash
curl -G \
  --url "http://localhost:9222/api/v1/datasets/{{KB_ID}}/documents" \
  --header 'Authorization: Bearer <YOUR_API_KEY>' \
  --data-urlencode 'metadata_condition={"logic":"and","conditions":[{"name":"tags","comparison_operator":"is","value":"bar"},{"name":"author","comparison_operator":"is","value":"alice"}]}'
```

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "docs": [
            {
                "chunk_count": 0,
                "create_date": "Mon, 14 Oct 2024 09:11:01 GMT",
                "create_time": 1728897061948,
                "created_by": "69736c5e723611efb51b0242ac120007",
                "id": "3bcfbf8a8a0c11ef8aba0242ac120006",
                "knowledgebase_id": "7898da028a0511efbf750242ac120005",
                "location": "Test_2.txt",
                "name": "Test_2.txt",
                "parser_config": {
                    "chunk_token_count": 128,
                    "delimiter": "\n",
                    "layout_recognize": true,
                    "task_page_size": 12
                },
                "chunk_method": "naive",
                "process_begin_at": null,
                "process_duration": 0.0,
                "progress": 0.0,
                "progress_msg": "",
                "run": "UNSTART",
                "size": 7,
                "source_type": "local",
                "status": "1",
                "thumbnail": null,
                "token_count": 0,
                "type": "doc",
                "update_date": "Mon, 14 Oct 2024 09:11:01 GMT",
                "update_time": 1728897061948
            }
        ],
        "total_datasets": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "You don't own the dataset 7898da028a0511efbf750242ac1220005. "
}
```

---

### Delete documents

**DELETE** `/api/v1/datasets/{dataset_id}/documents`

Deletes documents by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/datasets/{dataset_id}/documents`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "ids": ["id_1","id_2"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "delete_all": true
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `"ids"`: (*Body parameter*), `list[string]`  
  The IDs of the documents to delete.
  - If omitted, or set to `null` or an empty array, no documents are deleted.
  - If an array of IDs is provided, only the documents matching those IDs are deleted.
- `"delete_all"`: (*Body parameter*), `boolean`  
  Whether to delete all documents in the specified dataset when `"ids"` is omitted, or set to `null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0
}.
```

Failure:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```

---

### Parse documents

**POST** `/api/v1/datasets/{dataset_id}/chunks`

Parses documents in a specified dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/chunks`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"document_ids"`: `list[string]`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/chunks \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The dataset ID.
- `"document_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the documents to parse.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

---

### Stop parsing documents

**DELETE** `/api/v1/datasets/{dataset_id}/chunks`

Stops parsing specified documents.

#### Request

- Method: DELETE
- URL: `/api/v1/datasets/{dataset_id}/chunks`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"document_ids"`: `list[string]`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/chunks \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `"document_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the documents for which the parsing should be stopped.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

---

## CHUNK MANAGEMENT WITHIN DATASET

---

### Add chunk

**POST** `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks`

Adds a chunk to a specified document in a specified dataset.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"content"`: `string`
  - `"important_keywords"`: `list[string]`
  - `"tag_kwd"`: `list[string]`
  - `"image_base64"`: `string`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "content": "<CHUNK_CONTENT_HERE>",
          "image_base64": "<BASE64_ENCODED_IMAGE>"
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)
  The associated dataset ID.
- `document_ids`: (*Path parameter*)
  The associated document ID.
- `"content"`: (*Body parameter*), `string`, *Required*
  The text content of the chunk.
- `"important_keywords`(*Body parameter*), `list[string]`
  The key terms or phrases to tag with the chunk.
- `"tag_kwd"`: (*Body parameter*), `list[string]`
  Tag keywords to associate with the chunk.
- `"questions"`(*Body parameter*), `list[string]`
  If there is a given question, the embedded chunks will be based on them
- `"image_base64"`: (*Body parameter*), `string`
  A base64-encoded image to associate with the chunk. If the chunk already has an image, the new image will be vertically concatenated below the existing one.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunk": {
            "content": "who are you",
            "create_time": "2024-12-30 16:59:55",
            "create_timestamp": 1735549195.969164,
            "dataset_id": "72f36e1ebdf411efb7250242ac120006",
            "document_id": "61d68474be0111ef98dd0242ac120006",
            "id": "12ccdc56e59837e5",
            "image_id": "",
            "important_keywords": [],
            "tag_kwd": [],
            "questions": []
        }
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`content` is required"
}
```

---

### List chunks

**GET** `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks?keywords={keywords}&page={page}&page_size={page_size}&id={id}`

Lists chunks in a specified document.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks?keywords={keywords}&page={page}&page_size={page_size}&id={chunk_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks?keywords={keywords}&page={page}&page_size={page_size}&id={chunk_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>' 
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_id`: (*Path parameter*)  
  The associated document ID.
- `keywords`(*Filter parameter*), `string`  
  The keywords used to match chunk content.
- `page`(*Filter parameter*), `integer`  
  Specifies the page on which the chunks will be displayed. Defaults to `1`.
- `page_size`(*Filter parameter*), `integer`  
  The maximum number of chunks on each page. Defaults to `1024`.
- `id`(*Filter parameter*), `string`  
  The ID of the chunk to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunks": [
            {
                "available": true,
                "content": "This is a test content.",
                "docnm_kwd": "1.txt",
                "document_id": "b330ec2e91ec11efbc510242ac120004",
                "id": "b48c170e90f70af998485c1065490726",
                "image_id": "",
                "important_keywords": "",
                "tag_kwd": [],
                "positions": [
                    ""
                ]
            }
        ],
        "doc": {
            "chunk_count": 1,
            "chunk_method": "naive",
            "create_date": "Thu, 24 Oct 2024 09:45:27 GMT",
            "create_time": 1729763127646,
            "created_by": "69736c5e723611efb51b0242ac120007",
            "dataset_id": "527fa74891e811ef9c650242ac120006",
            "id": "b330ec2e91ec11efbc510242ac120004",
            "location": "1.txt",
            "name": "1.txt",
            "parser_config": {
                "chunk_token_num": 128,
                "delimiter": "\\n",
                "html4excel": false,
                "layout_recognize": true,
                "raptor": {
                    "use_raptor": false
                }
            },
            "process_begin_at": "Thu, 24 Oct 2024 09:56:44 GMT",
            "process_duration": 0.54213,
            "progress": 0.0,
            "progress_msg": "Task dispatched...",
            "run": "2",
            "size": 17966,
            "source_type": "local",
            "status": "1",
            "thumbnail": "",
            "token_count": 8,
            "type": "doc",
            "update_date": "Thu, 24 Oct 2024 11:03:15 GMT",
            "update_time": 1729767795721
        },
        "total": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "You don't own the document 5c5999ec7be811ef9cab0242ac12000e5."
}
```

---

### Delete chunks

**DELETE** `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks`

Deletes chunks by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"chunk_ids"`: `list[string]`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "chunk_ids": ["test_1", "test_2"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "delete_all": true
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `"chunk_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the chunks to delete.
  - If omitted, or set to `null` or an empty array, no chunks are deleted.
  - If an array of IDs is provided, only the chunks matching those IDs are deleted.
- `"delete_all"`: (*Body parameter*), `boolean`  
  Whether to delete all chunks of the specified documen when `"chunk_ids"` is omitted, or set to`null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "`chunk_ids` is required"
}
```

---

### Update chunk

**PUT** `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id}`

Updates content or configurations for a specified chunk.

#### Request

- Method: PUT
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"content"`: `string`
  - `"important_keywords"`: `list[string]`
  - `"tag_kwd"`: `list[string]`
  - `"available"`: `boolean`

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {   
          "content": "ragflow123",  
          "important_keywords": []  
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `chunk_id`: (*Path parameter*)  
  The ID of the chunk to update.
- `"content"`: (*Body parameter*), `string`  
  The text content of the chunk.
- `"important_keywords"`: (*Body parameter*), `list[string]`  
  A list of key terms or phrases to tag with the chunk.
- `"tag_kwd"`: (*Body parameter*), `list[string]`  
  Updated tag keywords.
- `"available"`: (*Body parameter*) `boolean`  
  The chunk's availability status in the dataset. Value options:  
  - `true`: Available (default)
  - `false`: Unavailable

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "Can't find this chunk 29a2d9987e16ba331fb4d7d30d99b71d2"
}
```

---

### Update chunk availability

**POST** `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/switch`

Updates or switches the availability status of specified chunks, controlling whether they are available for retrieval.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/switch`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"chunk_ids"`: `list[string]` (*Required*)
  - `"available_int"`: `integer` (*Optional*)
  - `"available"`: `boolean` (*Optional*)

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/switch \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "chunk_ids": ["chunk_id_1", "chunk_id_2"],
          "available_int": 1
     }'
```

##### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the dataset.
- `document_id`: (*Path parameter*)  
  The ID of the document.
- `"chunk_ids"`: (*Body parameter*), `list[string]` (*Required*)  
  IDs of the chunks whose availability status is to be updated.
- `"available_int"`: (*Body parameter*), `integer` (*Optional*)  
  Availability status for the specified chunks. Mutually exclusive with `"available"`. You must provide either `available_int` or `available`, *not* both.
  - `1`: Available,
  - `0`: Unavailable.
- `"available"`: (*Body parameter*), `boolean` (*Optional*)  
  Availability status of the specified chunks. Mutually exclusive with `"available_int"`. You must provide either `available` or `available_int`, *not* both.  
  - `true`: Available,
  - `false`: Unavailable.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 101,
    "message": "You don't own the dataset {dataset_id}."
}
```

```json
{
    "code": 101,
    "message": "`chunk_ids` is required."
}
```

```json
{
    "code": 101,
    "message": "`available_int` or `available` is required."
}
```

```json
{
    "code": 101,
    "message": "Document not found!"
}
```

```json
{
    "code": 101,
    "message": "Index updating failure"
}
```

---

### Retrieve a metadata summary from a dataset

**GET** `/api/v1/datasets/{dataset_id}/metadata/summary`

Aggregates metadata values across all documents in a dataset.

#### Request

- Method: GET
- URL: `/api/v1/datasets/{dataset_id}/metadata/summary`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Response

Success:

```json
{
  "code": 0,
  "data": {
    "summary": {
      "tags": {
        "type": "string",
        "values": [["bar", 2], ["foo", 1], ["baz", 1]]
      },
      "author": {
        "type": "string",
        "values": [["alice", 2], ["bob", 1]]
      }
    }
  }
}
```

---

### Update or delete metadata

**POST** `/api/v1/datasets/{dataset_id}/metadata/update`

Batch update or delete document-level metadata within a specified dataset. If both `document_ids` and `metadata_condition` are omitted, all documents within that dataset are selected. When both are provided, the intersection is used.

#### Request

- Method: POST
- URL: `/api/v1/datasets/{dataset_id}/metadata/update`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `selector`: `object`
  - `updates`: `list[object]`
  - `deletes`: `list[object]`

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `"selector"`: (*Body parameter*), `object`, *optional*  
  A document selector:  
  - `"document_ids"`: `list[string]` *optional*  
    The associated document ID.  
  - `"metadata_condition"`: `object`, *optional*  
    - `"logic"`: Defines the logic relation between conditions if multiple conditions are provided. Options:
      - `"and"` (default)
      - `"or"`
    - `"conditions"`: `list[object]` *optional*  
      Each object: `{ "name": string, "comparison_operator": string, "value": string }`  
      - `"name"`: `string` The key name to search by.
      - `"comparison_operator"`: `string` Available options:
        - `"is"`
        - `"not is"`
        - `"contains"`
        - `"not contains"`
        - `"in"`
        - `"not in"`
        - `"start with"`
        - `"end with"`
        - `">"`
        - `"<"`
        - `"≥"`
        - `"≤"`
        - `"empty"`
        - `"not empty"`
      - `"value"`: `string` The key value to search by.  
- `"updates"`: (*Body parameter*), `list[object]`, *optional*  
  Replaces metadata of the retrieved documents. Each object: `{ "key": string, "match": string, "value": string }`.  
  - `"key"`: `string` The name of the key to update.
  - `"match"`: `string` *optional* The current value of the key to update. When omitted, the corresponding keys are updated to `"value"` regardless of their current values.
  - `"value"`: `string` The new value to set for the specified keys.
- `"deletes`: (*Body parameter*), `list[ojbect]`, *optional*  
  Deletes metadata of the retrieved documents. Each object: `{ "key": string, "value": string }`.  
  - `"key"`: `string` The name of the key to delete.
  - `"value"`: `string` *Optional* The value of the key to delete.
    - When provided, only keys with a matching value are deleted.
    - When omitted, all specified keys are deleted.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/datasets/{dataset_id}/metadata/update \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
       "selector": {
         "metadata_condition": {
           "logic": "and",
           "conditions": [
             {"name": "author", "comparison_operator": "is", "value": "alice"}
           ]
         }
       },
       "updates": [
         {"key": "tags", "match": "foo", "value": "foo_new"}
       ],
       "deletes": [
         {"key": "obsolete_key"},
         {"key": "author", "value": "alice"}
       ]
     }'
```

##### Response

Success:

```json
{
  "code": 0,
  "data": {
    "updated": 1,
    "matched_docs": 2
  }
}
```

---

### Retrieve chunks

**POST** `/api/v1/retrieval`

Retrieves chunks from specified datasets.

#### Request

- Method: POST
- URL: `/api/v1/retrieval`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"question"`: `string`  
  - `"dataset_ids"`: `list[string]`  
  - `"document_ids"`: `list[string]`
  - `"page"`: `integer`  
  - `"page_size"`: `integer`  
  - `"similarity_threshold"`: `float`  
  - `"vector_similarity_weight"`: `float`  
  - `"top_k"`: `integer`  
  - `"rerank_id"`: `string`  
  - `"keyword"`: `boolean`  
  - `"highlight"`: `boolean`
  - `"cross_languages"`: `list[string]`
  - `"metadata_condition"`: `object`
  - `"use_kg"`: `boolean`
  - `"toc_enhance"`: `boolean`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/retrieval \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "question": "What is advantage of ragflow?",
          "dataset_ids": ["b2a62730759d11ef987d0242ac120004"],
          "document_ids": ["77df9ef4759a11ef8bdd0242ac120004"],
          "metadata_condition": {
            "logic": "and",
            "conditions": [
              {
                "name": "author",
                "comparison_operator": "=",
                "value": "Toby"
              },
              {
                "name": "url",
                "comparison_operator": "not contains",
                "value": "amd"
              }
            ]
          }
     }'
```

##### Request parameter

- `"question"`: (*Body parameter*), `string`, *Required*  
  The user query or query keywords.
- `"dataset_ids"`: (*Body parameter*) `list[string]`  
  The IDs of the datasets to search. If you do not set this argument, ensure that you set `"document_ids"`.
- `"document_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the documents to search. Ensure that all selected documents use the same embedding model. Otherwise, an error will occur. If you do not set this argument, ensure that you set `"dataset_ids"`.
- `"page"`: (*Body parameter*), `integer`  
  Specifies the page on which the chunks will be displayed. Defaults to `1`.
- `"page_size"`: (*Body parameter*)  
  The maximum number of chunks on each page. Defaults to `30`.
- `"similarity_threshold"`: (*Body parameter*)  
  The minimum similarity score. Defaults to `0.2`.
- `"vector_similarity_weight"`: (*Body parameter*), `float`  
  The weight of vector cosine similarity. Defaults to `0.3`. If x represents the weight of vector cosine similarity, then (1 - x) is the term similarity weight.
- `"top_k"`: (*Body parameter*), `integer`  
  The number of chunks engaged in vector cosine computation. Defaults to `1024`.
- `"use_kg"`: (*Body parameter*), `boolean`  
  Whether to search chunks related to the generated knowledge graph for multi-hop queries. Defaults to `False`. Before enabling this, ensure you have successfully constructed a knowledge graph for the specified datasets. See [here](../guides/dataset/advanced/construct_knowledge_graph.md) for details.
- `"toc_enhance"`: (*Body parameter*), `boolean`  
  Whether to search chunks with extracted table of content. Defaults to `False`. Before enabling this, ensure you have enabled `TOC_Enhance` and successfully extracted table of contents for the specified datasets. See [here](https://ragflow.io/docs/dev/enable_table_of_contents) for details.
- `"rerank_id"`: (*Body parameter*), `integer`  
  The ID of the rerank model.
- `"keyword"`: (*Body parameter*), `boolean`  
  Indicates whether to enable keyword-based matching:  
  - `true`: Enable keyword-based matching.
  - `false`: Disable keyword-based matching (default).
- `"highlight"`: (*Body parameter*), `boolean`  
  Specifies whether to enable highlighting of matched terms in the results:  
  - `true`: Enable highlighting of matched terms.
  - `false`: Disable highlighting of matched terms (default).
- `"cross_languages"`: (*Body parameter*) `list[string]`  
  The languages that should be translated into, in order to achieve keywords retrievals in different languages.
- `"metadata_condition"`: (*Body parameter*), `object`  
  The metadata condition used for filtering chunks:  
  - `"logic"`: (*Body parameter*), `string`
    - `"and"`: Return only results that satisfy *every* condition (default).
    - `"or"`: Return results that satisfy *any* condition.
  - `"conditions"`: (*Body parameter*), `array`  
    A list of metadata filter conditions.  
    - `"name"`: `string` - The metadata field name to filter by, e.g., `"author"`, `"company"`, `"url"`. Ensure this parameter before use. See [Set metadata](../guides/dataset/set_metadata.md) for details.
    - `comparison_operator`: `string` - The comparison operator. Can be one of:
      - `"contains"`
      - `"not contains"`
      - `"start with"`
      - `"empty"`
      - `"not empty"`
      - `"="`
      - `"≠"`
      - `">"`
      - `"<"`
      - `"≥"`
      - `"≤"`
    - `"value"`: `string` - The value to compare.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunks": [
            {
                "content": "ragflow content",
                "content_ltks": "ragflow content",
                "document_id": "5c5999ec7be811ef9cab0242ac120005",
                "document_keyword": "1.txt",
                "highlight": "<em>ragflow</em> content",
                "id": "d78435d142bd5cf6704da62c778795c5",
                "image_id": "",
                "important_keywords": [
                    ""
                ],
                "tag_kwd": [],
                "kb_id": "c7ee74067a2c11efb21c0242ac120006",
                "positions": [
                    ""
                ],
                "similarity": 0.9669436601210759,
                "term_similarity": 1.0,
                "vector_similarity": 0.8898122004035864
            }
        ],
        "doc_aggs": [
            {
                "count": 1,
                "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                "doc_name": "1.txt"
            }
        ],
        "total": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`datasets` is required."
}
```

---

## CHAT ASSISTANT MANAGEMENT

---

### Create chat assistant

**POST** `/api/v1/chats`

Creates a chat assistant.

#### Request

- Method: POST
- URL: `/api/v1/chats`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"icon"`: `string`
  - `"dataset_ids"`: `list[string]`
  - `"llm_id"`: `string`
  - `"llm_setting"`: `object`
  - `"prompt_config"`: `object`

##### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/chats \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
    "dataset_ids": ["0b2cbc8c877f11ef89070242ac120005"],
    "name":"new_chat_1"
}'
```

##### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*  
  The name of the chat assistant.
- `"icon"`: (*Body parameter*), `string`  
  Base64 encoding of the avatar.
- `"dataset_ids"`: (*Body parameter*), `list[string]`
  The unique identifiers for the associated datasets. If omitted or set to `[]`, an empty chat assistant is created; datasets can be attached at a later time.
- `"llm_id"`: (*Body parameter*), `string`
  The identifier of the chat model. If not specified, the system defaults to the user's pre-configured chat model.
- `"llm_setting"`: (*Body parameter*), `object`
  A configuration object defining the LLM parameters for the assistant. The `llm_setting` object may contain the following attributes:
  - `"model_type"`: `string`  
    A model type specifier. Only `"chat"` and `"image2text"` are recognized; any other inputs, or when omitted, are treated as `"chat"`.
  - `"temperature"`: `float`  
    Controls the randomness of the model's predictions. A lower temperature results in more conservative responses, while a higher temperature yields more creative and diverse responses. Defaults to `0.1`.  
  - `"top_p"`: `float`  
    Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
  - `"presence_penalty"`: `float`  
    This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.4`.
  - `"frequency penalty"`: `float`  
    Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- `"prompt_config"`: (*Body parameter*), `object`  
  Instructions for the LLM to follow. A `prompt_config` object may contain the following attributes:  
  - `"system"`: `string` The prompt content.
  - `"prologue"`: `string` The opening greeting for the user.
  - `"parameters"`: `object[]` This argument lists the variables to use in the system prompt. Note that:  
    - `"knowledge"` is a reserved variable, which represents the retrieved chunks.
    - All the variables in `"system"` should be curly bracketed.
  - `"empty_response"`: `string` If nothing is retrieved in the dataset for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is found, leave this blank.
  - `"quote"`: `boolean` Whether the source of text should be displayed. Defaults to `true`.
  - `"tts"`: `boolean`
  - `"refine_multiturn"`: `boolean`
  - `"use_kg"`: `boolean`
  - `"reasoning"`: `boolean`
  - `"cross_languages"`: `list[string]`
  - `"tavily_api_key"`: `string`
  - `"toc_enhance"`: `boolean`
- `"similarity_threshold"`: (*Body parameter*), `float`
- `"vector_similarity_weight"`: (*Body parameter*), `float`
- `"top_n"`: (*Body parameter*), `int`
- `"top_k"`: (*Body parameter*), `int`
- `"rerank_id"`: (*Body parameter*), `string`

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "icon": "",
        "create_date": "Thu, 24 Oct 2024 11:18:29 GMT",
        "create_time": 1729768709023,
        "dataset_ids": [
            "527fa74891e811ef9c650242ac120006"
        ],
        "kb_names": [
            "dataset_1"
        ],
        "description": "A helpful Assistant",
        "id": "b1f2f15691f911ef81180242ac120003",
        "language": "English",
        "llm_id": "qwen-plus@Tongyi-Qianwen",
        "llm_setting": {
            "frequency_penalty": 0.7,
            "presence_penalty": 0.4,
            "temperature": 0.1,
            "top_p": 0.3
        },
        "name": "12234",
        "prompt_config": {
            "empty_response": "Sorry! No relevant content was found in the knowledge base!",
            "prologue": "Hi! I'm your assistant. What can I do for you?",
            "quote": true,
            "system": "You are an intelligent assistant...",
            "parameters": [
                {
                    "key": "knowledge",
                    "optional": false
                }
            ]
        },
        "rerank_id": "",
        "similarity_threshold": 0.2,
        "vector_similarity_weight": 0.3,
        "top_n": 6,
        "prompt_type": "simple",
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "top_k": 1024,
        "update_date": "Thu, 24 Oct 2024 11:18:29 GMT",
        "update_time": 1729768709023
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Duplicated chat name."
}
```

---

### Update chat assistant

**PUT** `/api/v1/chats/{chat_id}`

Overwrites the existing configuration for a specified chat assistant.

Use this endpoint only when providing a complete configuration. Any fields omitted from the request will be reset to their server-side default values. For partial updates, use `PATCH /api/v1/chats/{chat_id}` instead.

#### Request

- Method: PUT
- URL: `/api/v1/chats/{chat_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"icon"`: `string`
  - `"dataset_ids"`: `list[string]`
  - `"llm_id"`: `string`
  - `"llm_setting"`: `object`
  - `"prompt_config"`: `object`

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/chats/{chat_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "name":"Test",
          "icon":"",
          "dataset_ids":["0b2cbc8c877f11ef89070242ac120005"],
          "llm_id":"qwen-plus@Tongyi-Qianwen",
          "llm_setting":{"temperature":0.1,"top_p":0.3,"presence_penalty":0.4,"frequency_penalty":0.7},
          "prompt_config":{
               "system":"You are an intelligent assistant...",
               "prologue":"Hi! I'\''m your assistant. What can I do for you?",
               "parameters":[{"key":"knowledge","optional":false}],
               "empty_response":"Sorry! No relevant content was found in the knowledge base!",
               "quote":true
          },
          "similarity_threshold":0.2,
          "vector_similarity_weight":0.3,
          "top_n":6,
          "top_k":1024,
          "rerank_id":""
     }'
```

#### Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the chat assistant to update.
- `"name"`: (*Body parameter*), `string`, *Required*  
  The revised name of the chat assistant.
- `"icon"`: (*Body parameter*), `string`  
  Base64 encoding of the avatar.
- `"dataset_ids"`: (*Body parameter*), `list[string]`
  The IDs of the associated datasets.
- `"llm_id"`: (*Body parameter*), `string`  
  The chat model name. If not set, the user's default chat model is used.  
- `"llm_setting"`: (*Body parameter*), `object`  
  The LLM settings for the chat assistant. An `llm_setting` object contains the following attributes:  
  - `"model_type"`: `string`
    A model type specifier. Supported values are `"chat"` and `"image2text"`. If the field is omitted or an unrecognized value is provided, it defaults to `"chat"`.
  - `"temperature"`: `float`  
    Controls the randomness of the model's predictions. A lower temperature results in more conservative responses, while a higher temperature yields more creative and diverse responses. Defaults to `0.1`.  
  - `"top_p"`: `float`  
    Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
  - `"presence_penalty"`: `float`  
    This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.4`.
  - `"frequency penalty"`: `float`  
    Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- `"prompt_config"`: (*Body parameter*), `object`
- `"similarity_threshold"`: (*Body parameter*), `float`
- `"vector_similarity_weight"`: (*Body parameter*), `float`
- `"top_n"`: (*Body parameter*), `int`
- `"top_k"`: (*Body parameter*), `int`
- `"rerank_id"`: (*Body parameter*), `string`

For `PUT` requests, any fields omitted from the request body are reset to their server-side default values.

#### Response

Success: returns the full updated chat assistant object.

```json
{
    "code": 0,
    "data": {
        "id": "04d0d8e28d1911efa3630242ac120006",
        "name": "Test",
        "description": "A helpful Assistant",
        "icon": "",
        "dataset_ids": ["527fa74891e811ef9c650242ac120006"],
        "kb_names": ["dataset_1"],
        "llm_id": "qwen-plus@Tongyi-Qianwen",
        "llm_setting": {
            "frequency_penalty": 0.7,
            "presence_penalty": 0.4,
            "temperature": 0.1,
            "top_p": 0.3
        },
        "prompt_config": {
            "empty_response": "Sorry! No relevant content was found in the knowledge base!",
            "prologue": "Hi! I'm your assistant. What can I do for you?",
            "quote": true,
            "system": "You are an intelligent assistant...",
            "parameters": [{"key": "knowledge", "optional": false}]
        },
        "similarity_threshold": 0.2,
        "vector_similarity_weight": 0.3,
        "top_n": 6,
        "top_k": 1024,
        "rerank_id": "",
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "create_time": 1729232406637,
        "update_time": 1729232406638
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Duplicated chat name."
}
```

---

### Get chat assistant

**GET** `/api/v1/chats/{chat_id}`

Retrieves a specified chat assistant.

#### Request

- Method: GET
- URL: `/api/v1/chats/{chat_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chats/{chat_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `chat_id`: (*Path parameter*)
  The ID of the chat assistant to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "icon": "",
        "create_date": "Fri, 18 Oct 2024 06:20:06 GMT",
        "create_time": 1729232406637,
        "description": "A helpful Assistant",
        "id": "04d0d8e28d1911efa3630242ac120006",
        "dataset_ids": ["527fa74891e811ef9c650242ac120006"],
        "kb_names": ["dataset_1"],
        "language": "English",
        "llm_id": "qwen-plus@Tongyi-Qianwen",
        "llm_setting": {
            "temperature": 0.1,
            "top_p": 0.3
        },
        "name": "my_chat",
        "prompt_config": {
            "empty_response": "Sorry! No relevant content was found in the knowledge base!",
            "prologue": "Hi! I'm your assistant. What can I do for you?",
            "quote": true,
            "system": "You are an intelligent assistant...",
            "parameters": [{"key": "knowledge", "optional": false}]
        },
        "rerank_id": "",
        "similarity_threshold": 0.2,
        "vector_similarity_weight": 0.3,
        "top_n": 6,
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "update_date": "Fri, 18 Oct 2024 06:20:06 GMT",
        "update_time": 1729232406638
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "No authorization."
}
```

---

### Partially update chat assistant

**PATCH** `/api/v1/chats/{chat_id}`

Performs a partial update on a specified chat assistant.

Unspecified fields are preserved, while nested objects, such as `llm_setting` and `prompt_config`, are deep-merged with the existing configuration. This is the recommended endpoint for renaming an assistant or modifying a specific subset of settings.

#### Request

- Method: PATCH
- URL: `/api/v1/chats/{chat_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body: any subset of the fields accepted by `PUT /api/v1/chats/{chat_id}`

##### Request example

```bash
curl --request PATCH \
     --url http://{address}/api/v1/chats/{chat_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
    "llm_id": "gpt-4o",
    "llm_setting": {"temperature": 0.5}
}'
```

#### Response

Success: returns the full updated chat assistant object (same structure as `PUT /api/v1/chats/{chat_id}`).

```json
{
    "code": 0,
    "data": {
        "id": "04d0d8e28d1911efa3630242ac120006",
        "name": "Renamed assistant",
        "llm_id": "qwen-plus@Tongyi-Qianwen",
        "..."  : "..."
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "No authorization."
}
```

---

### Delete chat assistant

**DELETE** `/api/v1/chats/{chat_id}`

Deletes a chat assistant by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/chats/{chat_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats/{chat_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `chat_id`: (*Path parameter*)
  The ID of the chat assistant to delete.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 102,
    "message": "No authorization."
}
```

---

### Delete chat assistants

**DELETE** `/api/v1/chats`

Deletes chat assistants by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/chats`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "ids": ["test_1", "test_2"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "delete_all": true
     }'
```

##### Request parameters

- `"ids"`: (*Body parameter*), `list[string]`  
  The IDs of the chat assistants to delete.
  - If omitted, or set to `null` or an empty array, no chat assistants are deleted.
  - If an array of IDs is provided, only the chat assistants matching those IDs are deleted.
- `"delete_all"`: (*Body parameter*), `boolean`  
  Whether to delete all chat assistants owned by the current user when `"ids"` is omitted, or set to`null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "ids are required"
}
```

---

### List chat assistants

**GET** `/api/v1/chats?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&keywords={keywords}&owner_ids={owner_id}&name={chat_name}&id={chat_id}`

Lists chat assistants.

#### Request

- Method: GET
- URL: `/api/v1/chats?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&keywords={keywords}&owner_ids={owner_id}&name={chat_name}&id={chat_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chats?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&keywords={keywords}&owner_ids={owner_id}&name={chat_name}&id={chat_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `page`: (*Filter parameter*), `integer`
  Specifies the page on which the chat assistants will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`
  The number of chat assistants on each page. Defaults to `30`.
- `orderby`: (*Filter parameter*), `string`
  The attribute by which the results are sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`
  Indicates whether the retrieved chat assistants should be sorted in descending order. Defaults to `true`.
- `keywords`: (*Filter parameter*), `string`
  Case-insensitive fuzzy match against chat assistant names.
- `owner_ids`: (*Filter parameter*), `string` (repeatable)
  Filter by owner tenant IDs. Can be specified multiple times: `?owner_ids=id1&owner_ids=id2`.
- `id`: (*Filter parameter*), `string`
  The ID of the chat assistant to retrieve with exact match.
- `name`: (*Filter parameter*), `string`
  The name of the chat assistant to retrieve with exact match.

When `id` or `name` is provided, exact filtering takes precedence over `keywords`.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chats": [
            {
                "icon": "",
                "create_date": "Fri, 18 Oct 2024 06:20:06 GMT",
                "create_time": 1729232406637,
                "description": "A helpful Assistant",
                "id": "04d0d8e28d1911efa3630242ac120006",
                "dataset_ids": ["527fa74891e811ef9c650242ac120006"],
                "kb_names": ["dataset_1"],
                "language": "English",
                "llm_id": "qwen-plus@Tongyi-Qianwen",
                "llm_setting": {
                    "frequency_penalty": 0.7,
                    "presence_penalty": 0.4,
                    "temperature": 0.1,
                    "top_p": 0.3
                },
                "name": "13243",
                "prompt_config": {
                    "empty_response": "Sorry! No relevant content was found in the knowledge base!",
                    "prologue": "Hi! I'm your assistant. What can I do for you?",
                    "quote": true,
                    "system": "You are an intelligent assistant...",
                    "parameters": [
                        {
                            "key": "knowledge",
                            "optional": false
                        }
                    ]
                },
                "rerank_id": "",
                "similarity_threshold": 0.2,
                "vector_similarity_weight": 0.3,
                "top_n": 6,
                "prompt_type": "simple",
                "status": "1",
                "tenant_id": "69736c5e723611efb51b0242ac120007",
                "update_date": "Fri, 18 Oct 2024 06:20:06 GMT",
                "update_time": 1729232406638
            }
        ],
        "total": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "The chat doesn't exist"
}
```

---

## SESSION MANAGEMENT

---

### Create session with chat assistant

**POST** `/api/v1/chats/{chat_id}/sessions`

Creates a session with a chat assistant.

#### Request

- Method: POST
- URL: `/api/v1/chats/{chat_id}/sessions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"user_id"`: `string` (optional)

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chats/{chat_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "name": "new session"
     }'
```

##### Request parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `"name"`: (*Body parameter*), `string`  
  The name of the chat session to create.
- `"user_id"`: (*Body parameter*), `string`  
  Optional user-defined ID.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "create_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "create_time": 1728636374571,
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "messages": [
            {
                "content": "Hi! I am your assistant, can I help you?",
                "role": "assistant"
            }
        ],
        "name": "new session",
        "update_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "update_time": 1728636374571
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`name` can not be empty."
}
```

---

### Update chat assistant's session

**PATCH** `/api/v1/chats/{chat_id}/sessions/{session_id}`

Updates a session of a specified chat assistant.

#### Request

- Method: PATCH
- URL: `/api/v1/chats/{chat_id}/sessions/{session_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`

##### Request example

```bash
curl --request PATCH \
     --url http://{address}/api/v1/chats/{chat_id}/sessions/{session_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "name": "<REVISED_SESSION_NAME_HERE>"
     }'
```

##### Request Parameter

- `chat_id`: (*Path parameter*)
  The ID of the associated chat assistant.
- `session_id`: (*Path parameter*)
  The ID of the session to update.
- `"name"`: (*Body Parameter*), `string`
  The revised name of the session.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "create_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "create_time": 1728636374571,
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "messages": [
            {
                "content": "Hi! I am your assistant, can I help you?",
                "role": "assistant"
            }
        ],
        "name": "updated session name",
        "update_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "update_time": 1728636374571,
        "user_id": ""
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`name` can not be empty."
}
```

---

### List chat assistant's sessions

**GET** `/api/v1/chats/{chat_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id}&user_id={user_id}`

Lists sessions associated with a specified chat assistant.

#### Request

- Method: GET
- URL: `/api/v1/chats/{chat_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id}&user_id={user_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chats/{chat_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id}&user_id={user_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the sessions will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of sessions on each page. Defaults to `30`. If set to `0`, an empty list is returned.
- `orderby`: (*Filter parameter*), `string`  
  The field by which sessions should be sorted. Available options:  
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved sessions should be sorted in descending order. Defaults to `true`.
- `name`: (*Filter parameter*) `string`  
  The name of the chat session to retrieve.
- `id`: (*Filter parameter*), `string`  
  The ID of the chat session to retrieve.
- `user_id`: (*Filter parameter*), `string`  
  The optional user-defined ID passed in when creating session.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "chat_id": "2ca4b22e878011ef88fe0242ac120005",
            "create_date": "Fri, 11 Oct 2024 08:46:43 GMT",
            "create_time": 1728636403974,
            "id": "578d541e87ad11ef96b90242ac120006",
            "messages": [
                {
                    "content": "Hi! I am your assistant, can I help you?",
                    "role": "assistant"
                }
            ],
            "name": "new session",
            "reference": [],
            "update_date": "Fri, 11 Oct 2024 08:46:43 GMT",
            "update_time": 1728636403974,
            "user_id": ""
        }
    ]
}
```

Failure:

```json
{
    "code": 102,
    "message": "The session doesn't exist"
}
```

---

### Get chat assistant's session

**GET** `/api/v1/chats/{chat_id}/sessions/{session_id}`

Gets a specific session of a specified chat assistant, including its messages, references, and avatar.

#### Request

- Method: GET
- URL: `/api/v1/chats/{chat_id}/sessions/{session_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chats/{chat_id}/sessions/{session_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `session_id`: (*Path parameter*)  
  The ID of the session to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "name": "new session",
        "avatar": "data:image/png;base64,...",
        "messages": [
            {
                "content": "Hi! I am your assistant, can I help you?",
                "role": "assistant"
            }
        ],
        "reference": []
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Session not found!"
}
```

---

### Delete a message from a chat assistant's session

**DELETE** `/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id}`

Deletes a user message and its paired assistant reply from a specified chat assistant session.

#### Request

- Method: DELETE
- URL: `/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `session_id`: (*Path parameter*)  
  The ID of the session that owns the message.
- `msg_id`: (*Path parameter*)  
  The ID of the message to delete.

#### Response

Success: returns the updated session object.

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "messages": [],
        "reference": []
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Session not found!"
}
```

---

### Update message feedback in a chat assistant's session

**PUT** `/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id}/feedback`

Updates feedback for an assistant message in a specified chat assistant session.

#### Request

- Method: PUT
- URL: `/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id}/feedback`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"thumbup"`: `boolean`
  - `"feedback"`: `string` (optional)

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/chats/{chat_id}/sessions/{session_id}/messages/{msg_id}/feedback \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "thumbup": false,
          "feedback": "The answer missed the cited document."
     }'
```

##### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `session_id`: (*Path parameter*)  
  The ID of the session that owns the message.
- `msg_id`: (*Path parameter*)  
  The ID of the assistant message to update.
- `"thumbup"`: (*Body parameter*), `boolean`  
  Whether the assistant message is marked as positive feedback.
- `"feedback"`: (*Body parameter*), `string`  
  Optional feedback text, typically used when `"thumbup"` is `false`.

#### Response

Success: returns the updated session object.

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "messages": [
            {
                "id": "message-id",
                "role": "assistant",
                "content": "Here is the answer.",
                "thumbup": false,
                "feedback": "The answer missed the cited document."
            }
        ]
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Session not found!"
}
```

---

### Delete chat assistant's sessions

**DELETE** `/api/v1/chats/{chat_id}/sessions`

Deletes sessions of a chat assistant by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/chats/{chat_id}/sessions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats/{chat_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "ids": ["test_1", "test_2"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chats/{chat_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "delete_all": true
     }'
```

##### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `"ids"`: (*Body Parameter*), `list[string]`  
  The IDs of the sessions to delete.
  - If omitted, or set to `null` or an empty array, no sessions are deleted.
  - If an array of IDs is provided, only the sessions matching those IDs are deleted.
- `"delete_all"`: (*Body Parameter*), `boolean`  
  Whether to delete all sessions of the specified chat assistant when `"ids"` is omitted, or set to `null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "The chat doesn't own the session"
}
```

---

### Converse with chat assistant

**POST** `/api/v1/chat/completions`

Starts a chat completion request. The same endpoint supports three modes:

- No `chat_id`: talk directly with the tenant's default chat model.
- With `chat_id` but no `session_id`: use that chat's configuration and automatically create a new session.
- With both `chat_id` and `session_id`: continue an existing chat session.

:::tip NOTE

- In streaming mode, not all responses include a reference, as this depends on the system's judgement.
- In streaming mode, the last message is an empty message:

  ```json
  data:
  {
    "code": 0,
    "data": true
  }
  ```

:::

#### Request

- Method: POST
- URL: `/api/v1/chat/completions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"messages"`: `list[object]`
  - `"stream"`: `boolean`
  - `"chat_id"`: `string` (optional)
  - `"session_id"`: `string` (optional)
  - `"llm_id"`: `string` (optional)

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
     {
          "messages": [
              {
                  "role": "user",
                  "content": "Who are you?"
              }
          ]
     }'
```

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
     {
          "chat_id": "{chat_id}",
          "stream": true,
          "session_id":"9fa7691cb85c11ef9c5f0242ac120005",
          "messages": [
              {
                  "role": "assistant",
                  "content": "Hi! I'\''m your assistant. What can I do for you?"
              },
              {
                  "role": "user",
                  "content": "Who are you?"
              }
          ]
     }'
```

##### Request Parameters

- `"messages"`: (*Body Parameter*), `list[object]`, *Required*  
  The conversation messages sent to the model.
- `"stream"`: (*Body Parameter*), `boolean`  
  Indicates whether to output responses in a streaming way:
  - `true`: Enable streaming (default).
  - `false`: Disable streaming.
- `"chat_id"`: (*Body Parameter*)  
  Optional chat assistant ID. If omitted, the tenant's default chat model is used directly.
- `"session_id"`: (*Body Parameter*)  
  Optional session ID. If `chat_id` is provided but `session_id` is omitted, a new session will be generated automatically.
- `"llm_id"`: (*Body Parameter*), `string`  
  Optional model override when a specific chat model should be used for this request.

#### Response

Success without `chat_id` or `session_id`:

```json
data:{
    "code": 0,
    "message": "",
    "data": {
        "answer": "I am an assistant powered by the tenant's default chat model.",
        "reference": {},
        "audio_binary": null,
        "id": "b01eed84b85611efa0e90242ac120005",
        "session_id": ""
    }
}
data:{
    "code": 0,
    "message": "",
    "data": true
}
```

Success with `chat_id` and `session_id`:

```json
data:{
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help answer questions by summarizing content from a",
        "reference": {},
        "audio_binary": null,
        "id": "a84c5dd4-97b4-4624-8c3b-974012c8000d",
        "session_id": "82b0ab2a9c1911ef9d870242ac120006"
    }
}
data:{
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help answer questions by summarizing content from a knowledge base. My responses are based on the information available in the knowledge base and",
        "reference": {},
        "audio_binary": null,
        "id": "a84c5dd4-97b4-4624-8c3b-974012c8000d",
        "session_id": "82b0ab2a9c1911ef9d870242ac120006"
    }
}
data:{
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help answer questions by summarizing content from a knowledge base. My responses are based on the information available in the knowledge base and any relevant chat history.",
        "reference": {},
        "audio_binary": null,
        "id": "a84c5dd4-97b4-4624-8c3b-974012c8000d",
        "session_id": "82b0ab2a9c1911ef9d870242ac120006"
    }
}
data:{
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help answer questions by summarizing content from a knowledge base ##0$$. My responses are based on the information available in the knowledge base and any relevant chat history.",
        "reference": {
            "total": 1,
            "chunks": [
                {
                    "id": "faf26c791128f2d5e821f822671063bd",
                    "content": "xxxxxxxx",
                    "document_id": "dd58f58e888511ef89c90242ac120006",
                    "document_name": "1.txt",
                    "dataset_id": "8e83e57a884611ef9d760242ac120006",
                    "image_id": "",
                    "url": null,
                    "similarity": 0.7,
                    "vector_similarity": 0.0,
                    "term_similarity": 1.0,
                    "doc_type": [],
                    "positions": [
                        ""
                    ]
                }
            ],
            "doc_aggs": [
                {
                    "doc_name": "1.txt",
                    "doc_id": "dd58f58e888511ef89c90242ac120006",
                    "count": 1
                }
            ]
        },
        "prompt": "xxxxxxxxxxx",
        "created_at": 1755055623.6401553,
        "id": "a84c5dd4-97b4-4624-8c3b-974012c8000d",
        "session_id": "82b0ab2a9c1911ef9d870242ac120006"
    }
}
data:{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 102,
    "message": "Please input your question."
}
```

---

### Create session with agent

:::danger DEPRECATED
This method is deprecated and not recommended. You can still call it but be mindful that calling `Converse with agent` will automatically generate a session ID for the associated agent.
:::

**POST** `/api/v1/agents/{agent_id}/sessions`

Creates a session with an agent.

#### Request

- Method: POST
- URL: `/api/v1/agents/{agent_id}/sessions?user_id={user_id}`
- Headers:
  - `'content-Type: application/json'
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - the required parameters:`str`
  - other parameters:
    The variables specified in the **Begin** component.

##### Request example

If the **Begin** component in your agent does not take required parameters:

```bash
curl --request POST \
     --url http://{address}/api/v1/agents/{agent_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
     }'
```

##### Request parameters

- `agent_id`: (*Path parameter*)  
  The ID of the associated agent.
- `user_id`: (*Filter parameter*)  
  The optional user-defined ID for parsing docs (especially images) when creating a session while uploading files.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "agent_id": "dbb4ed366e8611f09690a55a6daec4ef",
        "dsl": {
            "components": {
                "Message:EightyJobsAsk": {
                    "downstream": [],
                    "obj": {
                        "component_name": "Message",
                        "params": {
                            "content": [
                                "{begin@var1}{begin@var2}"
                            ],
                            "debug_inputs": {},
                            "delay_after_error": 2.0,
                            "description": "",
                            "exception_default_value": null,
                            "exception_goto": null,
                            "exception_method": null,
                            "inputs": {},
                            "max_retries": 0,
                            "message_history_window_size": 22,
                            "outputs": {
                                "content": {
                                    "type": "str",
                                    "value": null
                                }
                            },
                            "stream": true
                        }
                    },
                    "upstream": [
                        "begin"
                    ]
                },
                "begin": {
                    "downstream": [
                        "Message:EightyJobsAsk"
                    ],
                    "obj": {
                        "component_name": "Begin",
                        "params": {
                            "debug_inputs": {},
                            "delay_after_error": 2.0,
                            "description": "",
                            "enablePrologue": true,
                            "enable_tips": true,
                            "exception_default_value": null,
                            "exception_goto": null,
                            "exception_method": null,
                            "inputs": {
                                "var1": {
                                    "name": "var1",
                                    "optional": false,
                                    "options": [],
                                    "type": "line",
                                    "value": null
                                },
                                "var2": {
                                    "name": "var2",
                                    "optional": false,
                                    "options": [],
                                    "type": "line",
                                    "value": null
                                }
                            },
                            "max_retries": 0,
                            "message_history_window_size": 22,
                            "mode": "conversational",
                            "outputs": {},
                            "prologue": "Hi! I'm your assistant. What can I do for you?",
                            "tips": "Please fill in the form"
                        }
                    },
                    "upstream": []
                }
            },
            "globals": {
                "sys.conversation_turns": 0,
                "sys.files": [],
                "sys.query": "",
                "sys.user_id": ""
            },
            "graph": {
                "edges": [
                    {
                        "data": {
                            "isHovered": false
                        },
                        "id": "xy-edge__beginstart-Message:EightyJobsAskend",
                        "markerEnd": "logo",
                        "source": "begin",
                        "sourceHandle": "start",
                        "style": {
                            "stroke": "rgba(151, 154, 171, 1)",
                            "strokeWidth": 1
                        },
                        "target": "Message:EightyJobsAsk",
                        "targetHandle": "end",
                        "type": "buttonEdge",
                        "zIndex": 1001
                    }
                ],
                "nodes": [
                    {
                        "data": {
                            "form": {
                                "enablePrologue": true,
                                "inputs": {
                                    "var1": {
                                        "name": "var1",
                                        "optional": false,
                                        "options": [],
                                        "type": "line"
                                    },
                                    "var2": {
                                        "name": "var2",
                                        "optional": false,
                                        "options": [],
                                        "type": "line"
                                    }
                                },
                                "mode": "conversational",
                                "prologue": "Hi! I'm your assistant. What can I do for you?"
                            },
                            "label": "Begin",
                            "name": "begin"
                        },
                        "dragging": false,
                        "id": "begin",
                        "measured": {
                            "height": 112,
                            "width": 200
                        },
                        "position": {
                            "x": 270.64098070942583,
                            "y": -56.320928437811176
                        },
                        "selected": false,
                        "sourcePosition": "left",
                        "targetPosition": "right",
                        "type": "beginNode"
                    },
                    {
                        "data": {
                            "form": {
                                "content": [
                                    "{begin@var1}{begin@var2}"
                                ]
                            },
                            "label": "Message",
                            "name": "Message_0"
                        },
                        "dragging": false,
                        "id": "Message:EightyJobsAsk",
                        "measured": {
                            "height": 57,
                            "width": 200
                        },
                        "position": {
                            "x": 279.5,
                            "y": 190
                        },
                        "selected": true,
                        "sourcePosition": "right",
                        "targetPosition": "left",
                        "type": "messageNode"
                    }
                ]
            },
            "history": [],
            "memory": [],
            "messages": [],
            "path": [],
            "retrieval": [],
            "task_id": "dbb4ed366e8611f09690a55a6daec4ef"
        },
        "id": "0b02fe80780e11f084adcfdc3ed1d902",
        "message": [
            {
                "content": "Hi! I'm your assistant. What can I do for you?",
                "role": "assistant"
            }
        ],
        "source": "agent",
        "user_id": "c3fb861af27a11efa69751e139332ced"
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Agent not found."
}
```

---

### Converse with agent

**POST** `/api/v1/agents/{agent_id}/completions`  

Asks a specified agent a question to start an AI-powered conversation.

:::tip NOTE

- In streaming mode, not all responses include a reference, as this depends on the system's judgement.
- In streaming mode, the last message is an empty message:

  ```
  [DONE]
  ```

- You can optionally return step-by-step trace logs (see `return_trace` below).

:::

#### Request

- Method: POST
- URL: `/api/v1/agents/{agent_id}/completions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"question"`: `string`
  - `"stream"`: `boolean`
  - `"session_id"`: `string` (optional)
  - `"inputs"`: `object` (optional)
  - `"user_id"`: `string` (optional)
  - `"return_trace"`: `boolean` (optional, default `false`) — whether to include execution trace logs. See the `node_finished` event.
  - `"release"`: `boolean` (optional, default `false`) - whether to visit the latest published canvas.

#### Streaming events to handle

When `stream=true`, the server sends Server-Sent Events (SSE). A client should handle these events:

- `message`: Streaming content from the **Message** components.
- `message_end`: End of a **Message** component, which may include `reference`/`attachment`.
- `node_finished`: A component finishes; `data.inputs/outputs/error/elapsed_time` describes the node result. If a component produces structured output, read it from that component's `data.outputs.structured`. If `return_trace=true`, the trace is attached inside the same `node_finished` event (`data.trace`).

The stream terminates with `[DONE]`.

:::info IMPORTANT
You can include custom parameters in the request body, but first ensure they are defined in the [Begin](../guides/agent/agent_component_reference/begin.mdx) component.
:::

##### Request example

- If the **Begin** component does not take parameters:

```bash
curl --request POST \
     --url http://{address}/api/v1/agents/{agent_id}/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
     {
        "question": "Hello",
        "stream": false,
     }'
```

- If the **Begin** component takes parameters, include their values in the body of `"inputs"` as follows:  

```bash
curl --request POST \
     --url http://{address}/api/v1/agents/{agent_id}/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
    {
        "question": "Hello",
        "stream": false,
        "inputs": {
            "line_var": {
                "type": "line",
                "value": "I am line_var"
            },
            "int_var": {
                "type": "integer",
                "value": 1
            },
            "paragraph_var": {
                "type": "paragraph",
                "value": "a\nb\nc"
            },
            "option_var": {
                "type": "options",
                "value": "option 2"
            },
            "boolean_var": {
                "type": "boolean",
                "value": true
            }
        }
    }'
```

The following code will execute the completion process

```bash
curl --request POST \
     --url http://{address}/api/v1/agents/{agent_id}/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
     {
          "question": "Hello",
          "stream": true,
          "session_id": "cb2f385cb86211efa36e0242ac120005"
     }'
```

##### Request Parameters

- `agent_id`: (*Path parameter*), `string`  
  The ID of the associated agent.
- `"question"`: (*Body Parameter*), `string`, *Required*  
  The question to start an AI-powered conversation.
- `"stream"`: (*Body Parameter*), `boolean`  
  Indicates whether to output responses in a streaming way:  
  - `true`: Enable streaming (default).
  - `false`: Disable streaming.
- `"session_id"`: (*Body Parameter*)  
  The ID of the session. If it is not provided, a new session will be generated.
- `"inputs"`: (*Body Parameter*)  
  Variables specified in the **Begin** component.  
- `"user_id"`: (*Body parameter*), `string`  
  The optional user-defined ID. Valid *only* when no `session_id` is provided.

:::tip NOTE
For now, this method does *not* support a file type input/variable. As a workaround, use the following to upload a file to an agent:  
`http://{address}/v1/canvas/upload/{agent_id}`  
*You will get a corresponding file ID from its response body.*
:::

#### Response

success without `session_id` provided and with no variables specified in the **Begin** component:

Stream:

```json
...

data: {
    "event": "message",
    "message_id": "cecdcb0e83dc11f0858253708ecb6573",
    "created_at": 1756364483,
    "task_id": "d1f79142831f11f09cc51795b9eb07c0",
    "data": {
        "content": " themes"
    },
    "session_id": "cd097ca083dc11f0858253708ecb6573"
}

data: {
    "event": "message",
    "message_id": "cecdcb0e83dc11f0858253708ecb6573",
    "created_at": 1756364483,
    "task_id": "d1f79142831f11f09cc51795b9eb07c0",
    "data": {
        "content": "."
    },
    "session_id": "cd097ca083dc11f0858253708ecb6573"
}

data: {
    "event": "message_end",
    "message_id": "cecdcb0e83dc11f0858253708ecb6573",
    "created_at": 1756364483,
    "task_id": "d1f79142831f11f09cc51795b9eb07c0",
    "data": {
        "reference": {
            "chunks": {
                "20": {
                    "id": "4b8935ac0a22deb1",
                    "content": "```cd /usr/ports/editors/neovim/ && make install```## Android[Termux](https://github.com/termux/termux-app) offers a Neovim package.",
                    "document_id": "4bdd2ff65e1511f0907f09f583941b45",
                    "document_name": "INSTALL22.md",
                    "dataset_id": "456ce60c5e1511f0907f09f583941b45",
                    "image_id": "",
                    "positions": [
                        [
                            12,
                            11,
                            11,
                            11,
                            11
                        ]
                    ],
                    "url": null,
                    "similarity": 0.5705525104787287,
                    "vector_similarity": 0.7351750337624289,
                    "term_similarity": 0.5000000005,
                    "doc_type": ""
                }
            },
            "doc_aggs": {
                "INSTALL22.md": {
                    "doc_name": "INSTALL22.md",
                    "doc_id": "4bdd2ff65e1511f0907f09f583941b45",
                    "count": 3
                },
                "INSTALL.md": {
                    "doc_name": "INSTALL.md",
                    "doc_id": "4bd7fdd85e1511f0907f09f583941b45",
                    "count": 2
                },
                "INSTALL(1).md": {
                    "doc_name": "INSTALL(1).md",
                    "doc_id": "4bdfb42e5e1511f0907f09f583941b45",
                    "count": 2
                },
                "INSTALL3.md": {
                    "doc_name": "INSTALL3.md",
                    "doc_id": "4bdab5825e1511f0907f09f583941b45",
                    "count": 1
                }
            }
        }
    },
    "session_id": "cd097ca083dc11f0858253708ecb6573"
}

data: {
    "event": "node_finished",
    "message_id": "cecdcb0e83dc11f0858253708ecb6573",
    "created_at": 1756364483,
    "task_id": "d1f79142831f11f09cc51795b9eb07c0",
    "data": {
        "inputs": {
            "sys.query": "how to install neovim?"
        },
        "outputs": {
            "content": "xxxxxxx",
            "_created_time": 15294.0382,
            "_elapsed_time": 0.00017
        },
        "component_id": "Agent:EveryHairsChew",
        "component_name": "Agent_1",
        "component_type": "Agent",
        "error": null,
        "elapsed_time": 11.2091,
        "created_at": 15294.0382,
        "trace": [
            {
                "component_id": "begin",
                "trace": [
                    {
                        "inputs": {},
                        "outputs": {
                            "_created_time": 15257.7949,
                            "_elapsed_time": 0.00070
                        },
                        "component_id": "begin",
                        "component_name": "begin",
                        "component_type": "Begin",
                        "error": null,
                        "elapsed_time": 0.00085,
                        "created_at": 15257.7949
                    }
                ]
            },
            {
                "component_id": "Agent:WeakDragonsRead",
                "trace": [
                    {
                        "inputs": {
                            "sys.query": "how to install neovim?"
                        },
                        "outputs": {
                            "content": "xxxxxxx",
                            "_created_time": 15257.7982,
                            "_elapsed_time": 36.2382
                        },
                        "component_id": "Agent:WeakDragonsRead",
                        "component_name": "Agent_0",
                        "component_type": "Agent",
                        "error": null,
                        "elapsed_time": 36.2385,
                        "created_at": 15257.7982
                    }
                ]
            },
            {
                "component_id": "Agent:EveryHairsChew",
                "trace": [
                    {
                        "inputs": {
                            "sys.query": "how to install neovim?"
                        },
                        "outputs": {
                            "content": "xxxxxxxxxxxxxxxxx",
                            "_created_time": 15294.0382,
                            "_elapsed_time": 0.00017
                        },
                        "component_id": "Agent:EveryHairsChew",
                        "component_name": "Agent_1",
                        "component_type": "Agent",
                        "error": null,
                        "elapsed_time": 11.2091,
                        "created_at": 15294.0382
                    }
                ]
            }
        ]
    },
    "session_id": "cd097ca083dc11f0858253708ecb6573"
}

data:[DONE]
```

When `extra_body.reference_metadata.include` is `true`, each reference chunk may include a `document_metadata` object.

Non-stream:

If one or more components produce structured output, ensure you set `return_trace=true` and check each component's structured output via `trace`. The top-level `data.structured` field is a shortcut aggregated by `component_id`.

```json
{
    "code": 0,
    "data": {
        "created_at": 1756363177,
        "data": {
            "content": "\nTo install Neovim, the process varies depending on your operating system:\n\n### For macOS:\nUsing Homebrew:\n```bash\nbrew install neovim\n```\n\n### For Linux (Debian/Ubuntu):\n```bash\nsudo apt update\nsudo apt install neovim\n```\n\nFor other Linux distributions, you can use their respective package managers or build from source.\n\n### For Windows:\n1. Download the latest Windows installer from the official Neovim GitHub releases page\n2. Run the installer and follow the prompts\n3. Add Neovim to your PATH if not done automatically\n\n### From source (Unix-like systems):\n```bash\ngit clone https://github.com/neovim/neovim.git\ncd neovim\nmake CMAKE_BUILD_TYPE=Release\nsudo make install\n```\n\nAfter installation, you can verify it by running `nvim --version` in your terminal.",
            "created_at": 18129.044975627,
            "elapsed_time": 10.0157331670016,
            "inputs": {
                "var1": {
                    "value": "I am var1"
                },
                "var2": {
                    "value": "I am var2"
                }
            },
            "outputs": {
                "_created_time": 18129.502422278,
                "_elapsed_time": 0.00013378599760471843,
                "content": "\nTo install Neovim, the process varies depending on your operating system:\n\n### For macOS:\nUsing Homebrew:\n```bash\nbrew install neovim\n```\n\n### For Linux (Debian/Ubuntu):\n```bash\nsudo apt update\nsudo apt install neovim\n```\n\nFor other Linux distributions, you can use their respective package managers or build from source.\n\n### For Windows:\n1. Download the latest Windows installer from the official Neovim GitHub releases page\n2. Run the installer and follow the prompts\n3. Add Neovim to your PATH if not done automatically\n\n### From source (Unix-like systems):\n```bash\ngit clone https://github.com/neovim/neovim.git\ncd neovim\nmake CMAKE_BUILD_TYPE=Release\nsudo make install\n```\n\nAfter installation, you can verify it by running `nvim --version` in your terminal."
            },
            "reference": {
                "chunks": {
                    "20": {
                        "content": "```cd /usr/ports/editors/neovim/ && make install```## Android[Termux](https://github.com/termux/termux-app) offers a Neovim package.",
                        "dataset_id": "456ce60c5e1511f0907f09f583941b45",
                        "doc_type": "",
                        "document_id": "4bdd2ff65e1511f0907f09f583941b45",
                        "document_name": "INSTALL22.md",
                        "id": "4b8935ac0a22deb1",
                        "image_id": "",
                        "positions": [
                            [
                                12,
                                11,
                                11,
                                11,
                                11
                            ]
                        ],
                        "similarity": 0.5705525104787287,
                        "term_similarity": 0.5000000005,
                        "url": null,
                        "vector_similarity": 0.7351750337624289
                    }
                },
                "doc_aggs": {
                    "INSTALL(1).md": {
                        "count": 2,
                        "doc_id": "4bdfb42e5e1511f0907f09f583941b45",
                        "doc_name": "INSTALL(1).md"
                    },
                    "INSTALL.md": {
                        "count": 2,
                        "doc_id": "4bd7fdd85e1511f0907f09f583941b45",
                        "doc_name": "INSTALL.md"
                    },
                    "INSTALL22.md": {
                        "count": 3,
                        "doc_id": "4bdd2ff65e1511f0907f09f583941b45",
                        "doc_name": "INSTALL22.md"
                    },
                    "INSTALL3.md": {
                        "count": 1,
                        "doc_id": "4bdab5825e1511f0907f09f583941b45",
                        "doc_name": "INSTALL3.md"
                    }
                }
            },
            "trace": [
                {
                    "component_id": "begin",
                    "trace": [
                        {
                            "component_id": "begin",
                            "component_name": "begin",
                            "component_type": "Begin",
                            "created_at": 15926.567517862,
                            "elapsed_time": 0.0008189299987861887,
                            "error": null,
                            "inputs": {},
                            "outputs": {
                                "_created_time": 15926.567517862,
                                "_elapsed_time": 0.0006958619997021742
                            }
                        }
                    ]
                },
                {
                    "component_id": "Agent:WeakDragonsRead",
                    "trace": [
                        {
                            "component_id": "Agent:WeakDragonsRead",
                            "component_name": "Agent_0",
                            "component_type": "Agent",
                            "created_at": 15926.569121755,
                            "elapsed_time": 53.49016142000073,
                            "error": null,
                            "inputs": {
                                "sys.query": "how to install neovim?"
                            },
                            "outputs": {
                                "_created_time": 15926.569121755,
                                "_elapsed_time": 53.489981256001556,
                                "content": "xxxxxxxxxxxxxx",
                                "use_tools": [
                                    {
                                        "arguments": {
                                            "query": "xxxx"
                                        },
                                        "name": "search_my_dateset",
                                        "results": "xxxxxxxxxxx"
                                    }
                                ]
                            }
                        }
                    ]
                },
                {
                    "component_id": "Agent:EveryHairsChew",
                    "trace": [
                        {
                            "component_id": "Agent:EveryHairsChew",
                            "component_name": "Agent_1",
                            "component_type": "Agent",
                            "created_at": 15980.060569101,
                            "elapsed_time": 23.61718057500002,
                            "error": null,
                            "inputs": {
                                "sys.query": "how to install neovim?"
                            },
                            "outputs": {
                                "_created_time": 15980.060569101,
                                "_elapsed_time": 0.0003451630000199657,
                                "content": "xxxxxxxxxxxx"
                            }
                        }
                    ]
                },
                {
                    "component_id": "Message:SlickDingosHappen",
                    "trace": [
                        {
                            "component_id": "Message:SlickDingosHappen",
                            "component_name": "Message_0",
                            "component_type": "Message",
                            "created_at": 15980.061302513,
                            "elapsed_time": 23.61655923699982,
                            "error": null,
                            "inputs": {
                                "Agent:EveryHairsChew@content": "xxxxxxxxx",
                                "Agent:WeakDragonsRead@content": "xxxxxxxxxxx"
                            },
                            "outputs": {
                                "_created_time": 15980.061302513,
                                "_elapsed_time": 0.0006695749998471001,
                                "content": "xxxxxxxxxxx"
                            }
                        }
                    ]
                }
            ]
        },
        "event": "workflow_finished",
        "message_id": "c4692a2683d911f0858253708ecb6573",
        "session_id": "c39f6f9c83d911f0858253708ecb6573",
        "task_id": "d1f79142831f11f09cc51795b9eb07c0"
    }
}
```

Success without `session_id` provided and with variables specified in the **Begin** component:

Stream:

```json
data:{
    "event": "message",
    "message_id": "0e273472783711f0806e1a6272e682d8",
    "created_at": 1755083830,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": "Hello"
    },
    "session_id": "0e0d1542783711f0806e1a6272e682d8"
}

data:{
    "event": "message",
    "message_id": "0e273472783711f0806e1a6272e682d8",
    "created_at": 1755083830,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": "!"
    },
    "session_id": "0e0d1542783711f0806e1a6272e682d8"
}

data:{
    "event": "message",
    "message_id": "0e273472783711f0806e1a6272e682d8",
    "created_at": 1755083830,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": " How"
    },
    "session_id": "0e0d1542783711f0806e1a6272e682d8"
}

...

data:[DONE]
```

Non-stream:

```json
{
    "code": 0,
    "data": {
        "created_at": 1755083779,
        "data": {
            "created_at": 547400.868004651,
            "elapsed_time": 3.5037803899031132,
            "inputs": {
                "boolean_var": {
                    "type": "boolean",
                    "value": true
                },
                "int_var": {
                    "type": "integer",
                    "value": 1
                },
                "line_var": {
                    "type": "line",
                    "value": "I am line_var"
                },
                "option_var": {
                    "type": "options",
                    "value": "option 2"
                },
                "paragraph_var": {
                    "type": "paragraph",
                    "value": "a\nb\nc"
                }
            },
            "outputs": {
                "_created_time": 547400.869271305,
                "_elapsed_time": 0.0001251999055966735,
                "content": "Hello there! How can I assist you today?"
            }
        },
        "event": "workflow_finished",
        "message_id": "effdad8c783611f089261a6272e682d8",
        "session_id": "efe523b6783611f089261a6272e682d8",
        "task_id": "99ee29d6783511f09c921a6272e682d8"
    }
}
```

Success with variables specified in the **Begin** component:

Stream:

```json
data:{
    "event": "message",
    "message_id": "5b62e790783711f0bc531a6272e682d8",
    "created_at": 1755083960,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": "Hello"
    },
    "session_id": "979e450c781d11f095cb729e3aa55728"
}

data:{
    "event": "message",
    "message_id": "5b62e790783711f0bc531a6272e682d8",
    "created_at": 1755083960,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": "!"
    },
    "session_id": "979e450c781d11f095cb729e3aa55728"
}

data:{
    "event": "message",
    "message_id": "5b62e790783711f0bc531a6272e682d8",
    "created_at": 1755083960,
    "task_id": "99ee29d6783511f09c921a6272e682d8",
    "data": {
        "content": " You"
    },
    "session_id": "979e450c781d11f095cb729e3aa55728"
}

...

data:[DONE]
```

Non-stream:

```json
{
    "code": 0,
    "data": {
        "created_at": 1755084029,
        "data": {
            "created_at": 547650.750818867,
            "elapsed_time": 1.6227330720284954,
            "inputs": {},
            "outputs": {
                "_created_time": 547650.752800839,
                "_elapsed_time": 9.628792759031057e-05,
                "content": "Hello! It appears you've sent another \"Hello\" without additional context. I'm here and ready to respond to any requests or questions you may have. Is there something specific you'd like to discuss or learn about?"
            }
        },
        "event": "workflow_finished",
        "message_id": "84eec534783711f08db41a6272e682d8",
        "session_id": "979e450c781d11f095cb729e3aa55728",
        "task_id": "99ee29d6783511f09c921a6272e682d8"
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`question` is required."
}
```

---

### List agent sessions

**GET** `/api/v1/agents/{agent_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&id={session_id}&user_id={user_id}&dsl={dsl}`

Lists sessions associated with a specified agent.

#### Request

- Method: GET
- URL: `/api/v1/agents/{agent_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&id={session_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/agents/{agent_id}/sessions?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&id={session_id}&user_id={user_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request Parameters

- `agent_id`: (*Path parameter*)  
  The ID of the associated agent.
- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the sessions will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of sessions on each page. Defaults to `30`.
- `orderby`: (*Filter parameter*), `string`  
  The field by which sessions should be sorted. Available options:  
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved sessions should be sorted in descending order. Defaults to `true`.
- `id`: (*Filter parameter*), `string`  
  The ID of the agent session to retrieve.
- `user_id`: (*Filter parameter*), `string`  
  The optional user-defined ID passed in when creating session.
- `dsl`: (*Filter parameter*), `boolean`  
  Indicates whether to include the dsl field of the sessions in the response. Defaults to `true`.

#### Response

Success:

```json
{
    "code": 0,
    "data": [{
        "agent_id": "e9e2b9c2b2f911ef801d0242ac120006",
        "dsl": {
            "answer": [],
            "components": {
                "Answer:OrangeTermsBurn": {
                    "downstream": [],
                    "obj": {
                        "component_name": "Answer",
                        "params": {}
                    },
                    "upstream": []
                },
                "Generate:SocialYearsRemain": {
                    "downstream": [],
                    "obj": {
                        "component_name": "Generate",
                        "params": {
                            "cite": true,
                            "frequency_penalty": 0.7,
                            "llm_id": "gpt-4o___OpenAI-API@OpenAI-API-Compatible",
                            "message_history_window_size": 12,
                            "parameters": [],
                            "presence_penalty": 0.4,
                            "prompt": "Please summarize the following paragraph. Pay attention to the numbers and do not make things up. The paragraph is as follows:\n{input}\nThis is what you need to summarize.",
                            "temperature": 0.1,
                            "top_p": 0.3
                        }
                    },
                    "upstream": []
                },
                "begin": {
                    "downstream": [],
                    "obj": {
                        "component_name": "Begin",
                        "params": {}
                    },
                    "upstream": []
                }
            },
            "graph": {
                "edges": [],
                "nodes": [
                    {
                        "data": {
                            "label": "Begin",
                            "name": "begin"
                        },
                        "height": 44,
                        "id": "begin",
                        "position": {
                            "x": 50,
                            "y": 200
                        },
                        "sourcePosition": "left",
                        "targetPosition": "right",
                        "type": "beginNode",
                        "width": 200
                    },
                    {
                        "data": {
                            "form": {
                                "cite": true,
                                "frequencyPenaltyEnabled": true,
                                "frequency_penalty": 0.7,
                                "llm_id": "gpt-4o___OpenAI-API@OpenAI-API-Compatible",
                                "maxTokensEnabled": true,
                                "message_history_window_size": 12,
                                "parameters": [],
                                "presencePenaltyEnabled": true,
                                "presence_penalty": 0.4,
                                "prompt": "Please summarize the following paragraph. Pay attention to the numbers and do not make things up. The paragraph is as follows:\n{input}\nThis is what you need to summarize.",
                                "temperature": 0.1,
                                "temperatureEnabled": true,
                                "topPEnabled": true,
                                "top_p": 0.3
                            },
                            "label": "Generate",
                            "name": "Generate Answer_0"
                        },
                        "dragging": false,
                        "height": 105,
                        "id": "Generate:SocialYearsRemain",
                        "position": {
                            "x": 561.3457829707513,
                            "y": 178.7211182312641
                        },
                        "positionAbsolute": {
                            "x": 561.3457829707513,
                            "y": 178.7211182312641
                        },
                        "selected": true,
                        "sourcePosition": "right",
                        "targetPosition": "left",
                        "type": "generateNode",
                        "width": 200
                    },
                    {
                        "data": {
                            "form": {},
                            "label": "Answer",
                            "name": "Dialogue_0"
                        },
                        "height": 44,
                        "id": "Answer:OrangeTermsBurn",
                        "position": {
                            "x": 317.2368194777658,
                            "y": 218.30635555445093
                        },
                        "sourcePosition": "right",
                        "targetPosition": "left",
                        "type": "logicNode",
                        "width": 200
                    }
                ]
            },
            "history": [],
            "messages": [],
            "path": [],
            "reference": []
        },
        "id": "792dde22b2fa11ef97550242ac120006",
        "message": [
            {
                "content": "Hi! I'm your smart assistant. What can I do for you?",
                "role": "assistant"
            }
        ],
        "source": "agent",
        "user_id": ""
    }]
}
```

Failure:

```json
{
    "code": 102,
    "message": "You don't own the agent ccd2f856b12311ef94ca0242ac1200052."
}
```

---

### Delete agent's sessions

**DELETE** `/api/v1/agents/{agent_id}/sessions`

Deletes sessions of an agent by ID.

#### Request

- Method: DELETE
- URL: `/api/v1/agents/{agent_id}/sessions`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]`
  - `"delete_all"`: `boolean`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/agents/{agent_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '
     {
          "ids": ["test_1", "test_2"]
     }'
```

```bash
curl --request DELETE \
     --url http://{address}/api/v1/agents/{agent_id}/sessions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "delete_all": true
     }'
```

##### Request Parameters

- `agent_id`: (*Path parameter*)  
  The ID of the associated agent.
- `"ids"`: (*Body Parameter*), `list[string]`  
  The IDs of the sessions to delete.
  - If omitted, or set to `null` or an empty array, no sessions are deleted.
  - If an array of IDs is provided, only the sessions matching those IDs are deleted.
- `"delete_all"`: (*Body Parameter*), `boolean`  
  Whether to delete all sessions of the specified agent when `"ids"` is omitted, or set to `null` or an empty array. Defaults to `false`.

#### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "The agent doesn't own the session cbd31e52f73911ef93b232903b842af6"
}
```

---

### Text-to-speech

**POST** `/api/v1/chat/audio/speech`

Converts text to speech audio using the tenant's default TTS model, returning a streaming audio response.

#### Request

- Method: POST
- URL: `/api/v1/chat/audio/speech`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_LOGIN_TOKEN>'`
- Body:
  - `"text"`: `string` *(Required)* The text to synthesize.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/audio/speech \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_LOGIN_TOKEN>' \
     --output audio.mp3 \
     --data '{"text": "Hello, how can I help you today?"}'
```

#### Response

Success: binary `audio/mpeg` stream with headers `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.

Failure:

```json
{
    "code": 102,
    "message": "No default TTS model is set"
}
```

---

### Speech-to-text

**POST** `/api/v1/chat/audio/transcription`

Transcribes an audio file using the tenant's default ASR (automatic speech recognition) model.

#### Request

- Method: POST
- URL: `/api/v1/chat/audio/transcription`
- Headers:
  - `'Authorization: Bearer <YOUR_LOGIN_TOKEN>'`
- Body (multipart/form-data):
  - `"file"`: audio file (`.wav`, `.mp3`, `.m4a`, `.aac`, `.flac`, `.ogg`, `.webm`, `.opus`, `.wma`)
  - `"stream"`: `string` `"true"` for SSE streaming, `"false"` (default) for a single JSON response.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/audio/transcription \
     --header 'Authorization: Bearer <YOUR_LOGIN_TOKEN>' \
     --form file=@recording.wav \
     --form stream=false
```

#### Response

Success (non-streaming):

```json
{
    "code": 0,
    "data": {
        "text": "Hello, how can I help you today?"
    }
}
```

Success (streaming): SSE events with `data: {"event": "partial", "text": "..."}`.

Failure:

```json
{
    "code": 102,
    "message": "Unsupported audio format: .mp4. Allowed: .aac, .flac, .m4a, .mp3, .ogg, .opus, .wav, .webm, .wma"
}
```

---

### Generate mind map

**POST** `/api/v1/chat/mindmap`

Generates a mind map from a question and a set of knowledge base IDs.

#### Request

- Method: POST
- URL: `/api/v1/chat/mindmap`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_LOGIN_TOKEN>'`
- Body:
  - `"question"`: `string` *(Required)* The central question or topic.
  - `"kb_ids"`: `list[string]` *(Required)* Knowledge base IDs to search.
  - `"search_id"`: `string` *(Optional)* ID of a saved search configuration to merge additional `kb_ids` and settings.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/mindmap \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_LOGIN_TOKEN>' \
     --data '{
         "question": "What is retrieval-augmented generation?",
         "kb_ids": ["kb-abc123"]
     }'
```

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "name": "Retrieval-Augmented Generation",
        "children": [...]
    }
}
```

Failure:

```json
{
    "code": 500,
    "message": "..."
}
```

---

### Generate related questions

**POST** `/api/v1/chat/recommandation`

Generates five to ten alternative question strings from the user's original query to retrieve more relevant search results.

This operation requires a `Bearer Login Token`, which typically expires with in 24 hours. You can find it in the Request Headers in your browser easily as shown below:

![Image](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/login_token.jpg)

:::tip NOTE
The chat model autonomously determines the number of questions to generate based on the instruction, typically between five and ten.
:::

#### Request

- Method: POST
- URL: `/api/v1/chat/recommandation`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_LOGIN_TOKEN>'`
- Body:
  - `"question"`: `string` *(Required)* The original user question.
  - `"search_id"`: `string` *(Optional)* ID of a saved search configuration to use custom LLM settings.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/recommandation \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_LOGIN_TOKEN>' \
     --data '{
          "question": "What are the key advantages of Neovim over Vim?"
     }'
```

##### Request Parameters

- `"question"`: (*Body Parameter*), `string`
  The original user question.
- `"search_id"`: (*Body Parameter*), `string`
  ID of a saved search configuration to use custom LLM settings. If provided, the LLM model and generation settings from the search configuration will be used.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        "What makes Neovim superior to Vim in terms of features?",
        "How do the benefits of Neovim compare to those of Vim?",
        "What advantages does Neovim offer that are not present in Vim?",
        "In what ways does Neovim outperform Vim in functionality?",
        "What are the most significant improvements in Neovim compared to Vim?",
        "What unique advantages does Neovim bring to the table over Vim?",
        "How does the user experience in Neovim differ from Vim in terms of benefits?",
        "What are the top reasons to switch from Vim to Neovim?",
        "What features of Neovim are considered more advanced than those in Vim?"
    ],
    "message": "success"
}
```

Failure:

```json
{
    "code": 401,
    "data": null,
    "message": "<Unauthorized '401: Unauthorized'>"
}
```

---

## AGENT MANAGEMENT

---

### List agents

**GET** `/api/v1/agents?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={agent_name}&id={agent_id}`

Lists agents.

#### Request

- Method: GET
- URL: `/api/v1/agents?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&title={agent_name}&id={agent_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/agents?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&title={agent_name}&id={agent_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the agents will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of agents on each page. Defaults to `30`.
- `orderby`: (*Filter parameter*), `string`  
  The attribute by which the results are sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved agents should be sorted in descending order. Defaults to `true`.
- `id`: (*Filter parameter*), `string`  
  The ID of the agent to retrieve.
- `title`: (*Filter parameter*), `string`  
  The name of the agent to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "avatar": null,
            "canvas_type": null,
            "create_date": "Thu, 05 Dec 2024 19:10:36 GMT",
            "create_time": 1733397036424,
            "description": null,
            "dsl": {
                "answer": [],
                "components": {
                    "begin": {
                        "downstream": [],
                        "obj": {
                            "component_name": "Begin",
                            "params": {}
                        },
                        "upstream": []
                    }
                },
                "graph": {
                    "edges": [],
                    "nodes": [
                        {
                            "data": {
                                "label": "Begin",
                                "name": "begin"
                            },
                            "height": 44,
                            "id": "begin",
                            "position": {
                                "x": 50,
                                "y": 200
                            },
                            "sourcePosition": "left",
                            "targetPosition": "right",
                            "type": "beginNode",
                            "width": 200
                        }
                    ]
                },
                "history": [],
                "messages": [],
                "path": [],
                "reference": []
            },
            "id": "8d9ca0e2b2f911ef9ca20242ac120006",
            "title": "123465",
            "update_date": "Thu, 05 Dec 2024 19:10:56 GMT",
            "update_time": 1733397056801,
            "user_id": "69736c5e723611efb51b0242ac120007"
        }
    ]
}
```

Failure:

```json
{
    "code": 102,
    "message": "The agent doesn't exist."
}
```

---

### Create agent

**POST** `/api/v1/agents`

Create an agent.

#### Request

- Method: POST
- URL: `/api/v1/agents`
- Headers:
  - `'Content-Type: application/json`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"title"`: `string`
  - `"description"`: `string`
  - `"dsl"`: `object`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/agents \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
         "title": "Test Agent",
         "description": "A test agent",
         "dsl": {
           // ... Canvas DSL here ...
         }
     }'
```

##### Request parameters

- `title`: (*Body parameter*), `string`, *Required*  
  The title of the agent.
- `description`: (*Body parameter*), `string`  
  The description of the agent. Defaults to `None`.
- `dsl`: (*Body parameter*), `object`, *Required*  
  The canvas DSL object of the agent.

#### Response

Success:

```json
{
    "code": 0,
    "data": true,
    "message": "success"
}
```

Failure:

```json
{
    "code": 102,
    "message": "Agent with title test already exists."
}
```

---

### Update agent

**PUT** `/api/v1/agents/{agent_id}`

Update an agent by id.

#### Request

- Method: PUT
- URL: `/api/v1/agents/{agent_id}`
- Headers:
  - `'Content-Type: application/json`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"title"`: `string`
  - `"description"`: `string`
  - `"dsl"`: `object`

##### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/agents/58af890a2a8911f0a71a11b922ed82d6 \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
         "title": "Test Agent",
         "description": "A test agent",
         "dsl": {
           // ... Canvas DSL here ...
         }
     }'
```

##### Request parameters

- `agent_id`: (*Path parameter*), `string`  
  The id of the agent to be updated.
- `title`: (*Body parameter*), `string`  
  The title of the agent.
- `description`: (*Body parameter*), `string`  
  The description of the agent.
- `dsl`: (*Body parameter*), `object`  
  The canvas DSL object of the agent.

Only specify the parameter you want to change in the request body. If a parameter does not exist or is `None`, it won't be updated.

#### Response

Success:

```json
{
    "code": 0,
    "data": true,
    "message": "success"
}
```

Failure:

```json
{
    "code": 103,
    "message": "Only owner of canvas authorized for this operation."
}
```

---

### Delete agent

**DELETE** `/api/v1/agents/{agent_id}`

Delete an agent by id.

#### Request

- Method: DELETE
- URL: `/api/v1/agents/{agent_id}`
- Headers:
  - `'Content-Type: application/json`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/agents/58af890a2a8911f0a71a11b922ed82d6 \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{}'
```

##### Request parameters

- `agent_id`: (*Path parameter*), `string`  
  The id of the agent to be deleted.

#### Response

Success:

```json
{
    "code": 0,
    "data": true,
    "message": "success"
}
```

Failure:

```json
{
    "code": 103,
    "message": "Only owner of canvas authorized for this operation."
}
```

---



## MEMORY MANAGEMENT

### Create Memory

**POST** `/api/v1/memories`

Create a new memory.

#### Request

- Method: POST
- URL: `/api/v1/memories`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"memory_type"`: `list[string]`
  - `"embd_id"`: `string`. 
  - `"llm_id"`: `string`

##### Request example

```bash
curl --location 'http://{address}/api/v1/memories' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--data-raw '{
    "name": "new_memory_1",
    "memory_type": ["raw", "semantic"],
    "embd_id": "BAAI/bge-large-zh-v1.5@BAAI",
    "llm_id": "glm-4-flash@ZHIPU-AI"
}'
```

##### Request parameters

- `name` : (*Body parameter*), `string`, *Required*

  The unique name of the memory to create. It must adhere to the following requirements:

  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters

- `memory_type`: (*Body parameter*), `list[enum<string>]`,  *Required*

  Specifies the types of memory to extract. Available options:

  - `raw`: The raw dialogue content between the user and the agent . *Required by default*.
  - `semantic`: General knowledge and facts about the user and world.
  - `episodic`:  Time-stamped records of specific events and experiences.
  - `procedural`: Learned skills, habits, and automated procedures.

- `embd_id`: (*Body parameter*), `string`, *Required*

  The name of the embedding model to use. For example: `"BAAI/bge-large-zh-v1.5@BAAI"`

  - Maximum 255 characters
  - Must follow `model_name@model_factory` format

- `llm_id`: (*Body parameter*), `string`, *Required*

  The name of the chat model to use. For example: `"glm-4-flash@ZHIPU-AI"`

  - Maximum 255 characters
  - Must follow `model_name@model_factory` format

#### Response

Success:

```json
{
	"code": 0,
	"data": {
	...your new memory here
	},
	"message": true
}
```

Failure:

```json
{
    "code": 101,
    "message": "Memory name cannot be empty or whitespace."
}
```



### Update Memory

**PUT** `/api/v1/memories/{memory_id}`

Updates configurations for a specified memory.

#### Request

- Method: PUT
- URL: `/api/v1/memories/{memory_id}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"permission"`: `string`
  - `"llm_id"`: `string`
  - `"description"`: `string`
  - `"memory_size"`: `int`
  - `"forgetting_policy"`: `string`
  - `"temperature"`: `float`
  - `"system_promot"`: `string`
  - `"user_prompt"`: `string`

##### Request example

```bash
curl --location --request PUT 'http://{address}/api/v1/memories/d6775d4eeada11f08ca284ba59bc53c7' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--data '{
    "name": "name_update",
}'
```

##### Request parameters

- `memory_id`: (*Path parameter*)

  The ID of the memory to update.

- `name`: (*Body parameter*), `string`, *Optional*

  The revised name of the memory.

  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters, *Optional*

- `avatar`: (*Body parameter*), `string`, *Optional*

  The updated base64 encoding of the avatar.

  - Maximum 65535 characters

- `permission`: (*Body parameter*), `enum<string>`, *Optional*

  The updated memory permission. Available options:

  - `"me"`: (Default) Only you can manage the memory.
  - `"team"`: All team members can manage the memory.

- `llm_id`: (*Body parameter*), `string`, *Optional*

  The name of the chat model to use. For example: `"glm-4-flash@ZHIPU-AI"`

  - Maximum 255 characters
  - Must follow `model_name@model_factory` format

- `description`: (*Body parameter*), `string`, *Optional*

  The description of the memory. Defaults to `None`.

- `memory_size`: (*Body parameter*), `int`, *Optional*

  Defaults to `5*1024*1024` Bytes. Accounts for each message's content + its embedding vector (≈ Content + Dimensions × 8 Bytes). Example: A 1 KB message with 1024-dim embedding uses ~9 KB. The 5 MB default limit holds ~500 such messages.

  - Maximum 10 * 1024 * 1024 Bytes

- `forgetting_policy`: (*Body parameter*), `enum<string>`, *Optional*

  Evicts existing data based on the chosen policy when the size limit is reached, freeing up space for new messages. Available options:

  - `"FIFO"`: (Default) Prioritize messages with the earliest `forget_at` time for removal. When the pool of messages that have `forget_at` set is insufficient, it falls back to selecting messages in ascending order of their `valid_at` (oldest first).

- `temperature`: (*Body parameter*), `float`, *Optional*

  Adjusts output randomness. Lower = more deterministic; higher = more creative.

  - Range [0, 1]

- `system_prompt`: (*Body parameter*), `string`, *Optional*

  Defines the system-level instructions and role for the AI assistant. It is automatically assembled based on the selected `memory_type` by `PromptAssembler` in `memory/utils/prompt_util.py`. This prompt sets the foundational behavior and context for the entire conversation.

  - Keep the `OUTPUT REQUIREMENTS` and `OUTPUT FORMAT` parts unchanged.

- `user_prompt`: (*Body parameter*), `string`, *Optional*

  Represents the user's custom setting, which is the specific question or instruction the AI needs to respond to directly. Defaults to `None`.

#### Response

Success:

```json
{
	"code": 0,
	"data": {
	...your updated memory here
	},
	"message": true
}
```

Failure:

```json
{
    "code": 101,
    "message": "Memory name cannot be empty or whitespace."
}
```



### List Memory

**GET** `/api/v1/memories?tenant_id={tenant_ids}&memory_type={memory_types}&storage_type={storage_type}&keywords={keywords}&page={page}&page_size={page_size}`

List memories.

#### Request

- Method: GET
- URL:  `/api/v1/memories?tenant_id={tenant_ids}&memory_type={memory_types}&storage_type={storage_type}&keywords={keywords}&page={page}&page_size={page_size}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/memories?keywords=&page_size=50&page=1&memory_type=semantic%2Cepisodic' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `tenant_id`: (*Filter parameter*), `string` or `list[string]`, *Optional*

  The owner's ID, supports search multiple IDs.

- `memory_type`: (*Filter parameter*), `enum<string>` or `list[enum<string>]`, *Optional*

  The type of memory (as set during creation). A memory matches if its type is **included in** the provided value(s). Available options:

  - `raw`
  - `semantic`
  - `episodic`
  - `procedural`

- `storage_type`: (*Filter parameter*), `enum<string>`, *Optional*

  The storage format of messages. Available options:

  - `table`: (Default)

- `keywords`: (*Filter parameter*), `string`, *Optional*

  The name of memory to retrieve, supports fuzzy search.

- `page`: (*Filter parameter*), `int`, *Optional*

  Specifies the page on which the memories will be displayed. Defaults to `1`.

- `page_size`: (*Filter parameter*), `int`, *Optional*

  The number of memories on each page. Defaults to `50`.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "memory_list": [
            {
                "avatar": null,
                "create_date": "Tue, 06 Jan 2026 16:36:47 GMT",
                "create_time": 1767688607040,
                "description": null,
                "id": "d6775d4eeada11f08ca284ba59bc53c7",
                "memory_type": [
                    "raw",
                    "semantic"
                ],
                "name": "new_memory_1",
                "owner_name": "Lynn",
                "permissions": "me",
                "storage_type": "table",
                "tenant_id": "55777efac9df11f09cd07f49bd527ade"
            },
            ...other 3 memories here
        ],
        "total_count": 4
    },
    "message": true
}
```

Failure:

```json
{
    "code": 500,
    "message": "Internal Server Error."
}
```



### Get Memory Config

**GET** `/api/v1/memories/{memory_id}/config`

Get the configuration of a specified memory. 

#### Request

- Method: GET
- URL: `/api/v1/memories/{memory_id}/config`
- Headers: 
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/memories/6c8983badede11f083f184ba59bc53c7/config' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory.

#### Response

Success

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "create_date": "Mon, 22 Dec 2025 10:32:13 GMT",
        "create_time": 1766370733354,
        "description": null,
        "embd_id": "BAAI/bge-large-zh-v1.5@SILICONFLOW",
        "forgetting_policy": "FIFO",
        "id": "6c8983badede11f083f184ba59bc53c7",
        "llm_id": "glm-4.5-flash@ZHIPU-AI",
        "memory_size": 5242880,
        "memory_type": [
            "raw",
            "semantic",
            "episodic",
            "procedural"
        ],
        "name": "mem1222",
        "owner_name": null,
        "permissions": "me",
        "storage_type": "table",
        "system_prompt": ...your prompt here,
        "temperature": 0.5,
        "tenant_id": "55777efac9df11f09cd07f49bd527ade",
        "update_date": null,
        "update_time": null,
        "user_prompt": null
    },
    "message": true
}
```

Failure

```json
{
    "code": 404,
    "data": null,
    "message": "Memory '{memory_id}' not found."
}
```



### Delete Memory

**DELETE** `/api/v1/memories/{memory_id}`

Delete a specified memory.

#### Request

- Method: DELETE
- URL: `/api/v1/memories/{memory_id}`
- Headers:
- Headers: 
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location --request DELETE 'http://{address}/api/v1/memories/d6775d4eeada11f08ca284ba59bc53c7' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory to delete.

#### Response

Success

```json
{
    "code": 0,
    "data": null,
    "message": true
}
```

Failure

```json
{
    "code": 404,
    "data": null,
    "message": true
}
```



### List messages of a memory

**GET** `/api/v1/memories/{memory_id}?agent_id={agent_id}&keywords={session_id}&page={page}&page_size={page_size}`

List the messages of a specified memory.

#### Request

- Method: GET
- URL: `/api/v1/memories/{memory_id}?agent_id={agent_id}&keywords={session_id}&page={page}&page_size={page_size}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/memories/6c8983badede11f083f184ba59bc53c?page=1' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory to show messages.

- `agent_id`: (*Filter parameter*), `string` or `list[string]`, *Optional*

  Filters messages by the ID of their source agent. Supports multiple values.

- `session_id`: (*Filter parameter*), `string`, *Optional*

  Filters messages by their session ID. This field supports fuzzy search.

- `page`: (*Filter parameter*), `int`, *Optional*

  Specifies the page on which the messages will be displayed. Defaults to `1`.

- `page_size`: (*Filter parameter*), `int`, *Optional*

  The number of messages on each page. Defaults to `50`.

#### Response

Success

```json
{
    "code": 0,
    "data": {
        "messages": {
            "message_list": [
                {
                    "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
                    "agent_name": "memory_agent_1223",
                    "extract": [
                        {
                            "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
                            "agent_name": "memory_agent_1223",
                            "forget_at": "None",
                            "invalid_at": "None",
                            "memory_id": "6c8983badede11f083f184ba59bc53c7",
                            "message_id": 236,
                            "message_type": "semantic",
                            "session_id": "65b89ab8e96411f08d4e84ba59bc53c7",
                            "source_id": 233,
                            "status": true,
                            "user_id": "",
                            "valid_at": "2026-01-04 19:56:46"
                        },
                        ...other extracted messages
                    ],
                    "forget_at": "None",
                    "invalid_at": "None",
                    "memory_id": "6c8983badede11f083f184ba59bc53c7",
                    "message_id": 233,
                    "message_type": "raw",
                    "session_id": "65b89ab8e96411f08d4e84ba59bc53c7",
                    "source_id": "None",
                    "status": true,
                    "task": {
                        "progress": 1.0,
                        "progress_msg": "\n2026-01-04 19:56:46 Prepared prompts and LLM.\n2026-01-04 19:57:48 Get extracted result from LLM.\n2026-01-04 19:57:48 Extracted 6 messages from raw dialogue.\n2026-01-04 19:57:48 Prepared embedding model.\n2026-01-04 19:57:48 Embedded extracted content.\n2026-01-04 19:57:48 Saved messages to storage.\n2026-01-04 19:57:48 Message saved successfully."
                    },
                    "user_id": "",
                    "valid_at": "2026-01-04 19:56:42"
                },
                {
                    "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
                    "agent_name": "memory_agent_1223",
                    "extract": [],
                    "forget_at": "None",
                    "invalid_at": "None",
                    "memory_id": "6c8983badede11f083f184ba59bc53c7",
                    "message_id": 226,
                    "message_type": "raw",
                    "session_id": "d982a8cbe96111f08a1384ba59bc53c7",
                    "source_id": "None",
                    "status": true,
                    "task": {
                        "progress": -1.0,
                        "progress_msg": "Failed to insert message into memory. Details: 6c8983badede11f083f184ba59bc53c7_228:{'type': 'document_parsing_exception', 'reason': \"[1:230] failed to parse field [valid_at] of type [date] in document with id '6c8983badede11f083f184ba59bc53c7_228'. Preview of field's value: ''\", 'caused_by': {'type': 'illegal_argument_exception', 'reason': 'cannot parse empty date'}}; 6c8983badede11f083f184ba59bc53c7_229:{'type': 'document_parsing_exception', 'reason': \"[1:230] failed to parse field [valid_at] of type [date] in document with id '6c8983badede11f083f184ba59bc53c7_229'. Preview of field's value: ''\", 'caused_by': {'type': 'illegal_argument_exception', 'reason': 'cannot parse empty date'}}; 6c8983badede11f083f184ba59bc53c7_230:{'type': 'document_parsing_exception', 'reason': \"[1:230] failed to parse field [valid_at] of type [date] in document with id '6c8983badede11f083f184ba59bc53c7_230'. Preview of field's value: ''\", 'caused_by': {'type': 'illegal_argument_exception', 'reason': 'cannot parse empty date'}}; 6c8983badede11f083f184ba59bc53c7_231:{'type': 'document_parsing_exception', 'reason': \"[1:230] failed to parse field [valid_at] of type [date] in document with id '6c8983badede11f083f184ba59bc53c7_231'. Preview of field's value: ''\", 'caused_by': {'type': 'illegal_argument_exception', 'reason': 'cannot parse empty date'}}; 6c8983badede11f083f184ba59bc53c7_232:{'type': 'document_parsing_exception', 'reason': \"[1:230] failed to parse field [valid_at] of type [date] in document with id '6c8983badede11f083f184ba59bc53c7_232'. Preview of field's value: ''\", 'caused_by': {'type': 'illegal_argument_exception', 'reason': 'cannot parse empty date'}}"
                    },
                    "user_id": "",
                    "valid_at": "2026-01-04 19:38:26"
                },
                ...other 11 messages
            ],
            "total_count": 13
        },
        "storage_type": "table"
    },
    "message": true
}
```

Failure

```
{
    "code": 404,
    "data": null,
    "message": "Memory '{memory_id}' not found."
}
```



### Add Message

**POST** `/api/v1/messages`

Add a message to specified memories.

#### Request

- Method: POST
- URL: `/api/v1/messages`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"memory_id"`: `list[string]`
  - `"agent_id"`: `string`
  - `"session_id"`: `string`
  - `"user_id"`: `string`
  - `"user_input"`: `string`
  - `"agent_response"`: `string`

##### Request example

```bash
curl --location 'http://{address}/api/v1/messages' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--data '{
    "memory_id": ["6c8983badede11f083f184ba59bc53c7", "87ebb892df1711f08d6b84ba59bc53c7"],
    "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
    "session_id": "bf0a50abeb8111f0917884ba59bc53c7",
    "user_id": "55777efac9df11f09cd07f49bd527ade",
    "user_input": "your user input here",
    "agent_response": "your agent response here"

}'
```

##### Request parameter

- `memory_id`: (*Body parameter*), `list[string]`, *Required*

  The IDs of the memories to save messages.

- `agent_id`: (*Body parameter*), `string`, *Required*

  The ID of the message's source agent.

- `session_id`: (*Body parameter*), `string`, *Required*

  The ID of the message's session.

- `user_id`: (*Body parameter*), `string`, *Optional*

  The user participating in the conversation with the agent. Defaults to `None`.

- `user_input`: (*Body parameter*), `string`, *Required*

  The text input provided by the user.

- `agent_response`: (*Body parameter*), `string`, *Required*

  The text response generated by the AI agent.

#### Response

Success

```json
{
    "code": 0,
    "data": null,
    "message": "All add to task."
}
```

Failure

```json
{
    "code": 500,
    "data": null,
    "message": "Some messages failed to add. Detail: {fail information}"
}
```



### Forget Message

**DELETE** `/api/v1/messages/{memory_id}:{message_id}`

Forget a specified message. After forgetting, this message will not be retrieved by agents, and it will also be prioritized for cleanup by the forgetting policy.

#### Request

- Method: DELETE
- URL: `/api/v1/messages/{memory_id}:{message_id}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location --request DELETE 'http://{address}/api/v1/messages/6c8983badede11f083f184ba59bc53c7:272' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory to which the specified message belongs.

- `message_id`: (*Path parameter*), `string`, *Required*

  The ID of the message to forget.

#### Response

Success

```json
{
    "code": 0,
    "data": null,
    "message": true
}
```

Failure

```json
{
    "code": 404,
    "data": null,
    "message": "Memory '{memory_id}' not found."
}
```



### Update message status

**PUT** `/api/v1/messages/{memory_id}:{message_id}`

Update message status, enable or disable a message. Once a message is disabled, it will not be retrieved by agents.

#### Request 

- Method: PUT
- URL: `/api/v1/messages/{memory_id}:{message_id}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"status"`: `bool`

##### Request example

```bash
curl --location --request PUT 'http://{address}/api/v1/messages/6c8983badede11f083f184ba59bc53c7:270' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <YOUR_API_KEY>' \
--data '{
    "status": false
}'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory to which the specified message belongs.

- `message_id`: (*Path parameter*), `string`, *Required*

  The ID of the message to enable or disable.

- `status`: (*Body parameter*), `bool`, *Required*

  The status of message. `True` = `enabled`, `False` = `disabled`.

#### Response

Success

```json
{
    "code": 0,
    "data": null,
    "message": true
}
```

Failure

```json
{
    "code": 404,
    "data": null,
    "message": "Memory '{memory_id}' not found."
}
```

### Search Message

**GET** `/api/v1/messages/search?query={question}&memory_id={memory_id}&similarity_threshold={similarity_threshold}&keywords_similarity_weight={keywords_similarity_weight}&top_n={top_n}`

Searches and retrieves messages from memory based on the provided `query` and other configuration parameters.

#### Request

- Method: GET
- URL: `/api/v1/messages/search?query={question}&memory_id={memory_id}&similarity_threshold={similarity_threshold}&keywords_similarity_weight={keywords_similarity_weight}&top_n={top_n}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/messages/search?query=%22who%20are%20you%3F%22&memory_id=6c8983badede11f083f184ba59bc53c7&similarity_threshold=0.2&keywords_similarity_weight=0.7&top_n=10' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `question`: (*Filter parameter*), `string`, *Required* 

  The search term or natural language question used to find relevant messages.

- `memory_id`: (*Filter parameter*), `string` or `list[string]`, *Required* 

  The IDs of the memories to search.  Supports multiple values.

- `agent_id`: (*Filter parameter*), `string`, *Optional*

  The ID of the message's source agent. Defaults to `None`.

- `session_id`: (*Filter parameter*), `string`, *Optional*

  The ID of the message's session. Defaults to `None`.

- `similarity_threshold`: (*Filter parameter*), `float`, *Optional*

  The minimum cosine similarity score required for a message to be considered a match. A higher value  yields more precise but fewer results. Defaults to `0.2`.

  - Range [0.0, 1.0]

- `keywords_similarity_weight` : (*Filter parameter*), `float`, *Optional*

  Controls the influence of keyword matching versus semantic (embedding-based) matching in the final relevance score. A value of 0.5 gives them equal weight. Defaults to `0.7`.

  - Range [0.0, 1.0]

- `top_n`: (*Filter parameter*), `int`, *Optional*

  The maximum number of most relevant messages to return. This limits the result set size for efficiency. Defaults to `10`.

#### Response

Success

```json
{
    "code": 0,
    "data": [
        {
            "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
            "content": "User Input: who am I?\nAgent Response: To address the question \"who am I?\", let's follow the logical steps outlined in the instructions:\n\n1. **Understand the User’s Request**: The user is asking for a clarification or identification of their own self. This is a fundamental question about personal identity.\n\n2. **Decompose the Request**: The request is quite simple and doesn't require complex decomposition. The core task is to provide an answer that identifies the user in some capacity.\n\n3. **Execute the Subtask**:\n   - **Identify the nature of the question**: The user is seeking to understand their own existence or their sense of self.\n   - **Assess the context**: The context is not explicitly given, so the response will be general.\n   - **Provide a response**: The answer should acknowledge the user's inquiry into their identity.\n\n4. **Validate Accuracy and Consistency**: The response should be consistent with the general understanding of the question. Since the user has not provided specific details about their identity, the response should be broad and open-ended.\n\n5. **Summarize the Final Result**: The user is asking \"who am I?\" which is an inquiry into their own identity. The answer is that the user is the individual who is asking the question. Without more specific information, a detailed description of their identity cannot be provided.\n\nSo, the final summary would be:\n\nThe user is asking the question \"who am I?\" to seek an understanding of their own identity. The response to this question is that the user is the individual who is posing the question. Without additional context or details, a more comprehensive description of the user's identity cannot be given.",
            "forget_at": "None",
            "invalid_at": "None",
            "memory_id": "6c8983badede11f083f184ba59bc53c7",
            "message_id": 61,
            "message_type": "raw",
            "session_id": "ebf8025de52211f0b56684ba59bc53c7",
            "source_id": "None",
            "status": true,
            "user_id": "",
            "valid_at": "2025-12-30 09:57:49"
        },
        ...other 2 matched messages here
    ],
    "message": true
}
```

Failure

```json
{
    "code": 500,
    "message": "Internal Server Error."
}
```



### Get Recent Messages

**GET** `/api/v1/messages?memory_id={memory_id}&agent_id={agent_id}&session_id={session_id}&limit={limit}`

Retrieves the most recent messages from specified memories. Typically accepts a `limit` parameter to control the number of messages returned.

#### Request

- Method: GET
- URL: `/api/v1/messages?memory_id={memory_id}&agent_id={agent_id}&session_id={session_id}&limit={limit}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/messages?memory_id=6c8983badede11f083f184ba59bc53c7&limit=10' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Filter parameter*), `string` or `list[string]`, *Required* 

  The IDs of the memories to search.  Supports multiple values.

- `agent_id`: (*Filter parameter*), `string`, *Optional*

  The ID of the message's source agent. Defaults to `None`.

- `session_id`: (*Filter parameter*), `string`, *Optional*

  The ID of the message's session. Defaults to `None`.

- `limit`: (*Filter parameter*), `int`, *Optional*

  Control the number of messages returned. Defaults to `10`.

#### Response

Success

```json
{
    "code": 0,
    "data": [
        {
            "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
            "content": "User Input: what is pineapple?\nAgent Response: A pineapple is a tropical fruit known for its sweet, tangy flavor and distinctive, spiky appearance. Here are the key facts:\nScientific Name: Ananas comosus\nPhysical Description: It has a tough, spiky, diamond-patterned outer skin (rind) that is usually green, yellow, or brownish. Inside, the juicy yellow flesh surrounds a fibrous core.\nGrowth: Unlike most fruits, pineapples do not grow on trees. They grow from a central stem as a composite fruit, meaning they are formed from many individual berries that fuse together around the core. They grow on a short, leafy plant close to the ground.\nUses: Pineapples are eaten fresh, cooked, grilled, juiced, or canned. They are a popular ingredient in desserts, fruit salads, savory dishes (like pizzas or ham glazes), smoothies, and cocktails.\nNutrition: They are a good source of Vitamin C, manganese, and contain an enzyme called bromelain, which aids in digestion and can tenderize meat.\nSymbolism: The pineapple is a traditional symbol of hospitality and welcome in many cultures.\nAre you asking about the fruit itself, or its use in a specific context?",
            "forget_at": "None",
            "invalid_at": "None",
            "memory_id": "6c8983badede11f083f184ba59bc53c7",
            "message_id": 269,
            "message_type": "raw",
            "session_id": "bf0a50abeb8111f0917884ba59bc53c7",
            "source_id": "None",
            "status": true,
            "user_id": "",
            "valid_at": "2026-01-07 16:49:12"
        },
        ...other 9 messages here
    ],
    "message": true
}
```

Failure

```json
{
    "code": 500,
    "message": "Internal Server Error."
}
```



### Get Message Content

**GET** `/api/v1/messages/{memory_id}:{message_id}/content`

Retrieves the full content and embed vector of a specific message using its unique message ID.

#### Request

- Method: GET
- URL: `/api/v1/messages/{memory_id}:{message_id}/content`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --location 'http://{address}/api/v1/messages/6c8983badede11f083f184ba59bc53c7:270/content' \
--header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `memory_id`: (*Path parameter*), `string`, *Required*

  The ID of the memory to which the specified message belongs.

- `message_id`: (*Path parameter*), `string`, *Required*

  The ID of the message.

#### Response

Success

```json
{
    "code": 0,
    "data": {
        "agent_id": "8db9c8eddfcc11f0b5da84ba59bc53c7",
        "content": "Pineapples are tropical fruits known for their sweet, tangy flavor and distinctive, spiky appearance",
        "content_embed": [
        	0.03641991,
            ...embed vector here
        ],
        "forget_at": null,
        "id": "6c8983badede11f083f184ba59bc53c7_270",
        "invalid_at": null,
        "memory_id": "6c8983badede11f083f184ba59bc53c7",
        "message_id": 270,
        "message_type": "semantic",
        "session_id": "bf0a50abeb8111f0917884ba59bc53c7",
        "source_id": 269,
        "status": false,
        "user_id": "",
        "valid_at": "2026-01-07 16:48:37",
        "zone_id": 0
    },
    "message": true
}
```

Failure

```json
{
    "code": 404,
    "data": null,
    "message": "Memory '{memory_id}' not found."
}
```



---

## System

---

### Check system health

**GET** `/api/v1/system/healthz`

Check the health status of RAGFlow’s dependencies (database, Redis, document engine, object storage).

#### Request

- Method: GET
- URL: `/api/v1/system/healthz`
- Headers:
  - 'Content-Type: application/json'
  (no Authorization required)

##### Request example

```bash
curl --request GET
     --url http://{address}/api/v1/system/healthz
     --header 'Content-Type: application/json'
```

##### Request parameters

- `address`: (*Path parameter*), string  
  The host and port of the backend service (e.g., `localhost:7897`).

---

#### Responses

- **200 OK** – All services healthy

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "db": "ok",
  "redis": "ok",
  "doc_engine": "ok",
  "storage": "ok",
  "status": "ok"
}
```

- **500 Internal Server Error** – At least one service unhealthy

```http
HTTP/1.1 500 INTERNAL SERVER ERROR
Content-Type: application/json

{
  "db": "ok",
  "redis": "nok",
  "doc_engine": "ok",
  "storage": "ok",
  "status": "nok",
  "_meta": {
    "redis": {
      "elapsed": "5.2",
      "error": "Lost connection!"
    }
  }
}
```

Explanation:  

- Each service is reported as "ok" or "nok".  
- The top-level `status` reflects overall health.  
- If any service is "nok", detailed error info appears in `_meta`.  

---

## FILE MANAGEMENT

---

### Upload file

**POST** `/api/v1/files`

Uploads one or multiple files to the system.

#### Request

- Method: POST
- URL: `/api/v1/files`
- Headers:
  - `'Content-Type: multipart/form-data'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Form:
  - `'file=@{FILE_PATH}'`
  - `'parent_id'`: `string` (optional)

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/files \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --form 'file=@./test1.txt' \
     --form 'file=@./test2.pdf' \
     --form 'parent_id={folder_id}'
```

##### Request parameters

- `'file'`: (*Form parameter*), `file`, *Required*  
  The file(s) to upload. Multiple files can be uploaded in a single request.
- `'parent_id'`: (*Form parameter*), `string`  
  The parent folder ID where the file will be uploaded. If not specified, files will be uploaded to the root folder.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "id": "b330ec2e91ec11efbc510242ac120004",
            "name": "test1.txt",
            "size": 17966,
            "type": "doc",
            "parent_id": "527fa74891e811ef9c650242ac120006",
            "location": "test1.txt",
            "create_time": 1729763127646
        }
    ]
}
```

Failure:

```json
{
    "code": 400,
    "message": "No file part!"
}
```

---

### Upload document

**POST** `/v1/document/upload_info`

Uploads a file and creates the respective document.

#### Request

- Method: POST
- URL: `/v1/document/upload_info`
- Headers:
  - `'Content-Type: multipart/form-data'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Form:
  - `'file=@{FILE_PATH}'` (mutually exclusive with `url`)
- Query:
  - `url`: URL to crawl and convert to a runtime attachment (mutually exclusive with `file`).

##### Request example

Upload a local file:

```bash
curl --request POST \
     --url http://{address}/v1/document/upload_info \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --form 'file=@./test1.pdf'
```

Crawl a URL:

```bash
curl --request POST \
     --url 'http://{address}/v1/document/upload_info?url=https://example.com/page' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `'file'`: (*Form parameter*), `file`, *Optional*  
  The file to upload. Mutually exclusive with `url`; either `file` or `url` must be provided.
- `url`: (*Query parameter*), `string`, *Optional*  
  A URL to crawl and store as an attachment. Mutually exclusive with `file`; either `url` or `file` must be provided.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
      "created_at": 1772451421.7924063,
      "created_by": "be951084066611f18f5f00155d2f98f4",
      "extension": "pdf",
      "id": "2143a03d162c11f1b80f00155d334d02",
      "mime_type": "application/pdf",
      "name": "test1.pdf",
      "preview_url": null,
      "size": 49705
    },
    "message": "success"
}
```

Failure:

```json
{
    "code": 400,
    "message": "Provide either multipart file(s) or ?url=...!"
}
```

---

### Download attachment

**GET** `/v1/document/download/{attachment_id}`

Downloads a runtime attachment previously uploaded via the [Upload document](#upload-document) method.

#### Request

- Method: GET
- URL: `/v1/document/download/{attachment_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Query parameter:
  - `ext`: `string` (Optional)

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/v1/document/download/{attachment_id}?ext=pdf' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --output ./downloaded_attachment.pdf
```

##### Request parameters

- `attachment_id`: (*Path parameter*), `string`, *Required*  
  The `id` value returned by the [Upload document](#upload-document) method.
- `ext`: (*Query parameter*), `string`, *Optional*  
  A file extension hint specifying the response's Content-Type. Defaults to `"markdown"`. Available values:  
  - `"markdown"`
  - `"html"`
  - `"pdf"`
  - `"docx"`
  - `"xlsx"`
  - `"csv"`

#### Response

Success:

Returns the file content as a binary stream with the relevant Content-Type header.

Failure:

```json
{
    "code": 500,
    "message": "Internal server error"
}
```

---

### Create file or folder

**POST** `/api/v1/files`

Creates a new file or folder in the system.

#### Request

- Method: POST
- URL: `/api/v1/files`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"name"`: `string`
  - `"parent_id"`: `string` (optional)
  - `"type"`: `string`

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/files \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "name": "New Folder",
          "type": "folder",
          "parent_id": "{folder_id}"
     }'
```

##### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*  
  The name of the file or folder to create.
- `"parent_id"`: (*Body parameter*), `string`  
  The parent folder ID. If not specified, the file/folder will be created in the root folder.
- `"type"`: (*Body parameter*), `string`  
  The type of the file to create. Available options:
  - `"folder"`: Create a folder
  - `"virtual"`: Create a virtual file

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "id": "b330ec2e91ec11efbc510242ac120004",
        "name": "New Folder",
        "type": "folder",
        "parent_id": "527fa74891e811ef9c650242ac120006",
        "size": 0,
        "create_time": 1729763127646
    }
}
```

Failure:

```json
{
    "code": 409,
    "message": "Duplicated folder name in the same folder."
}
```

---

### List files

**GET** `/api/v1/files?parent_id={parent_id}&keywords={keywords}&page={page}&page_size={page_size}&orderby={orderby}&desc={desc}`

Lists files and folders under a specific folder.

#### Request

- Method: GET
- URL: `/api/v1/files?parent_id={parent_id}&keywords={keywords}&page={page}&page_size={page_size}&orderby={orderby}&desc={desc}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/api/v1/files?parent_id={folder_id}&page=1&page_size=15' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `parent_id`: (*Filter parameter*), `string`  
  The folder ID to list files from. If not specified, the root folder is used by default.
- `keywords`: (*Filter parameter*), `string`  
  Search keyword to filter files by name.
- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the files will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of files on each page. Defaults to `15`.
- `orderby`: (*Filter parameter*), `string`  
  The field by which files should be sorted. Available options:
  - `create_time` (default)
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved files should be sorted in descending order. Defaults to `true`.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "total": 10,
        "files": [
            {
                "id": "b330ec2e91ec11efbc510242ac120004",
                "name": "test1.txt",
                "type": "doc",
                "size": 17966,
                "parent_id": "527fa74891e811ef9c650242ac120006",
                "create_time": 1729763127646
            }
        ],
        "parent_folder": {
            "id": "527fa74891e811ef9c650242ac120006",
            "name": "Parent Folder"
        }
    }
}
```

Failure:

```json
{
    "code": 404,
    "message": "Folder not found!"
}
```

---

### Get parent folder

**GET** `/api/v1/files/{file_id}/parent`

Retrieves the immediate parent folder information of a specified file.

#### Request

- Method: GET
- URL: `/api/v1/files/{file_id}/parent`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/api/v1/files/{file_id}/parent' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `file_id`: (*Path parameter*), `string`, *Required*  
  The ID of the file whose immediate parent folder to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "parent_folder": {
            "id": "527fa74891e811ef9c650242ac120006",
            "name": "Parent Folder"
        }
    }
}
```

Failure:

```json
{
    "code": 404,
    "message": "Folder not found!"
}
```

---

### Get all parent folders

**GET** `/api/v1/files/{file_id}/ancestors`

Retrieves all parent folders of a specified file in the folder hierarchy.

#### Request

- Method: GET
- URL: `/api/v1/files/{file_id}/ancestors`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/api/v1/files/{file_id}/ancestors' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `file_id`: (*Path parameter*), `string`, *Required*  
  The ID of the file whose parent folders to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "parent_folders": [
            {
                "id": "527fa74891e811ef9c650242ac120006",
                "name": "Parent Folder 1"
            },
            {
                "id": "627fa74891e811ef9c650242ac120007",
                "name": "Parent Folder 2"
            }
        ]
    }
}
```

Failure:

```json
{
    "code": 404,
    "message": "Folder not found!"
}
```

---

### Delete files

**DELETE** `/api/v1/files`

Deletes one or multiple files or folders.

#### Request

- Method: DELETE
- URL: `/api/v1/files`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"ids"`: `list[string]`

##### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/files \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "ids": ["file_id_1", "file_id_2"]
     }'
```

##### Request parameters

- `"ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the files or folders to delete.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 404,
    "message": "File or Folder not found!"
}
```

---

### Download file

**GET** `/api/v1/files/{file_id}`

Downloads a file from the system.

#### Request

- Method: GET
- URL: `/api/v1/files/{file_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/files/{file_id} \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --output ./downloaded_file.txt
```

##### Request parameters

- `file_id`: (*Path parameter*), `string`, *Required*  
  The ID of the file to download.

#### Response

Success:

Returns the file content as a binary stream with appropriate Content-Type headers.

Failure:

```json
{
    "code": 404,
    "message": "Document not found!"
}
```

---

### Move or rename files

**POST** `/api/v1/files/move`

Moves and/or renames files or folders. Follows Linux `mv` semantics: at least one of `dest_file_id` or `new_name` must be provided.

- `dest_file_id` only: move files to a new folder, names unchanged.
- `new_name` only: rename a single file or folder in place, no storage operation.
- Both: move and rename simultaneously.

#### Request

- Method: POST
- URL: `/api/v1/files/move`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"src_file_ids"`: `list[string]`, *Required*
  - `"dest_file_id"`: `string`, *Optional*
  - `"new_name"`: `string`, *Optional*

##### Request examples

Move files to a folder:

```bash
curl --request POST \
     --url http://{address}/api/v1/files/move \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "src_file_ids": ["file_id_1", "file_id_2"],
          "dest_file_id": "{destination_folder_id}"
     }'
```

Rename a file in place:

```bash
curl --request POST \
     --url http://{address}/api/v1/files/move \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "src_file_ids": ["{file_id}"],
          "new_name": "new_name.txt"
     }'
```

##### Request parameters

- `"src_file_ids"`: (*Body parameter*), `list[string]`, *Required*
  The IDs of the files or folders to move or rename.
- `"dest_file_id"`: (*Body parameter*), `string`, *Optional*
  The ID of the destination folder. Omit to rename in place.
- `"new_name"`: (*Body parameter*), `string`, *Optional*
  New name for the file or folder. Only valid when `src_file_ids` contains a single entry. Note: Changing file extensions is *not* supported.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 404,
    "message": "File or Folder not found!"
}
```

or

```json
{
    "code": 404,
    "message": "Parent folder not found!"
}
```

or

```json
{
    "code": 400,
    "message": "The extension of file can't be changed"
}
```

---

### Convert files to documents and link them to datasets

**POST** `/v1/file2document/convert`

Converts files to documents and links them to specified datasets.

#### Request

- Method: POST
- URL: `/v1/file2document/convert`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:
  - `"file_ids"`: `list[string]`
  - `"kb_ids"`: `list[string]`

##### Request example

```bash
curl --request POST \
     --url http://{address}/v1/file2document/convert \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data '{
          "file_ids": ["file_id_1", "file_id_2"],
          "kb_ids": ["dataset_id_1", "dataset_id_2"]
     }'
```

##### Request parameters

- `"file_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the files to convert. If a folder ID is provided, all files within that folder will be converted.
- `"kb_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the target datasets.

#### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "id": "file2doc_id_1",
            "file_id": "file_id_1",
            "document_id": "document_id_1"
        }
    ]
}
```

Failure:

```json
{
    "code": 404,
    "message": "File not found!"
}
```

or

```json
{
    "code": 404,
    "message": "Can't find this dataset!"
}
```

---

## SEARCH APP MANAGEMENT

### Create search app

**POST** `/api/v1/searches`

Creates a search app.

#### Request

- Method: POST
- URL: `/api/v1/searches`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:

```json
{
    "name": "my_search_app",
    "description": "optional description"
}
```

##### Request example

```bash
curl --request POST \
     --url 'http://{address}/api/v1/searches' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --header 'Content-Type: application/json' \
     --data '{
         "name": "my_search_app",
         "description": "My first search app"
     }'
```

##### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*
  The name of the search app. Must be unique and no longer than 255 characters.
- `"description"`: (*Body parameter*), `string`
  A brief description of the search app.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "search_id": "b330ec2e91ec11efbc510242ac120006"
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Search name can't be empty."
}
```

---

### List search apps

**GET** `/api/v1/searches?keywords={keywords}&page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&owner_ids={owner_ids}`

Lists search apps for the current user.

#### Request

- Method: GET
- URL: `/api/v1/searches`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/api/v1/searches?page=1&page_size=20' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `keywords`: (*Filter parameter*), `string`
  Search keyword to filter search apps by name.
- `page`: (*Filter parameter*), `integer`
  Specifies the page number. Defaults to `0` (no pagination).
- `page_size`: (*Filter parameter*), `integer`
  The number of items per page. Defaults to `0` (no pagination).
- `orderby`: (*Filter parameter*), `string`
  The field to sort by. Defaults to `create_time`.
- `desc`: (*Filter parameter*), `boolean`
  Whether to sort in descending order. Defaults to `true`.
- `owner_ids`: (*Filter parameter*), `string` (repeatable)
  Filter by owner tenant IDs. Can be specified multiple times: `?owner_ids=id1&owner_ids=id2`.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "total": 2,
        "search_apps": [
            {
                "id": "b330ec2e91ec11efbc510242ac120006",
                "name": "my_search_app",
                "description": "My first search app",
                "tenant_id": "7c8983badede11f083f184ba59bc53c7",
                "create_time": 1729763127646
            }
        ]
    }
}
```

---

### Get search app

**GET** `/api/v1/searches/{search_id}`

Gets the details of a search app.

#### Request

- Method: GET
- URL: `/api/v1/searches/{search_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request GET \
     --url 'http://{address}/api/v1/searches/b330ec2e91ec11efbc510242ac120006' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `search_id`: (*Path parameter*), `string`, *Required*
  The ID of the search app to retrieve.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "id": "b330ec2e91ec11efbc510242ac120006",
        "name": "my_search_app",
        "description": "My first search app",
        "tenant_id": "7c8983badede11f083f184ba59bc53c7",
        "search_config": {},
        "create_time": 1729763127646
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Can't find this Search App!"
}
```

---

### Update search app

**PUT** `/api/v1/searches/{search_id}`

Updates a search app.

#### Request

- Method: PUT
- URL: `/api/v1/searches/{search_id}`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_API_KEY>'`
- Body:

```json
{
    "name": "updated_name",
    "search_config": {"top_k": 5}
}
```

##### Request example

```bash
curl --request PUT \
     --url 'http://{address}/api/v1/searches/b330ec2e91ec11efbc510242ac120006' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --header 'Content-Type: application/json' \
     --data '{
         "name": "updated_name",
         "search_config": {"top_k": 5}
     }'
```

##### Request parameters

- `search_id`: (*Path parameter*), `string`, *Required*
  The ID of the search app to update.
- `"name"`: (*Body parameter*), `string`, *Required*
  The new name of the search app.
- `"search_config"`: (*Body parameter*), `object`, *Required*
  Configuration fields to update. Merged with the existing config.

#### Response

Success:

```json
{
    "code": 0,
    "data": {
        "id": "b330ec2e91ec11efbc510242ac120006",
        "name": "updated_name",
        "search_config": {"top_k": 5},
        "create_time": 1729763127646
    }
}
```

Failure:

```json
{
    "code": 109,
    "message": "No authorization."
}
```

---

### Delete search app

**DELETE** `/api/v1/searches/{search_id}`

Deletes a search app.

#### Request

- Method: DELETE
- URL: `/api/v1/searches/{search_id}`
- Headers:
  - `'Authorization: Bearer <YOUR_API_KEY>'`

##### Request example

```bash
curl --request DELETE \
     --url 'http://{address}/api/v1/searches/b330ec2e91ec11efbc510242ac120006' \
     --header 'Authorization: Bearer <YOUR_API_KEY>'
```

##### Request parameters

- `search_id`: (*Path parameter*), `string`, *Required*
  The ID of the search app to delete.

#### Response

Success:

```json
{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 109,
    "message": "No authorization."
}
```

---

### Search completion

**POST** `/api/v1/searches/{search_id}/completion`

Generates an answer using the saved search app configuration and returns the result as a Server-Sent Events stream.

#### Request

- Method: POST
- URL: `/api/v1/searches/{search_id}/completion`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer <YOUR_LOGIN_TOKEN>'`
- Body:
  - `"question"`: `string` *(Required)* The user question.
  - `"kb_ids"`: `list[string]` *(Optional)* Fallback dataset IDs. Used only when the search app config does not already define `kb_ids`.

##### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/searches/{search_id}/completion \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_LOGIN_TOKEN>' \
     --data '{
         "question": "What is retrieval-augmented generation?"
     }'
```

##### Request parameters

- `search_id`: (*Path parameter*), `string`, *Required*  
  The ID of the search app.
- `"question"`: (*Body parameter*), `string`, *Required*  
  The user question.
- `"kb_ids"`: (*Body parameter*), `list[string]`  
  Optional fallback dataset IDs when the search app config does not define them.

#### Response

Success (streaming):

```text
data: {"code": 0, "message": "", "data": {"answer": "...", "reference": {...}}}

data: {"code": 0, "message": "", "data": true}
```

Failure:

```json
{
    "code": 109,
    "message": "No authorization."
}
```
