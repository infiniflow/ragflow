# RAGFlow CLI User Command Reference

This document describes the user commands available in RAGFlow CLI. All commands must end with a semicolon (`;`).

## Command List

### ping_server

**Description**  
Tests the connection status to the server.

**Usage**  
```
PING;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> PING;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### show_current_user

**Description**  
Displays information about the currently logged-in user.

**Usage**  
```
SHOW CURRENT USER;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> SHOW CURRENT USER;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### create_model_provider

**Description**  
Creates a new model provider.

**Usage**  
```
CREATE MODEL PROVIDER <provider_name> <provider_key>;
```

**Parameters**  
- `provider_name`: Provider name, quoted string.
- `provider_key`: Provider key, quoted string.

**Example**  
```
ragflow> CREATE MODEL PROVIDER 'openai' 'sk-...';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### drop_model_provider

**Description**  
Deletes a model provider.

**Usage**  
```
DROP MODEL PROVIDER <provider_name>;
```

**Parameters**  
- `provider_name`: Name of the provider to delete, quoted string.

**Example**  
```
ragflow> DROP MODEL PROVIDER 'openai';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_llm

**Description**  
Sets the default LLM (Large Language Model).

**Usage**  
```
SET DEFAULT LLM <llm_id>;
```

**Parameters**  
- `llm_id`: LLM identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT LLM 'gpt-4';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_vlm

**Description**  
Sets the default VLM (Vision Language Model).

**Usage**  
```
SET DEFAULT VLM <vlm_id>;
```

**Parameters**  
- `vlm_id`: VLM identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT VLM 'clip-vit-large';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_embedding

**Description**  
Sets the default embedding model.

**Usage**  
```
SET DEFAULT EMBEDDING <embedding_id>;
```

**Parameters**  
- `embedding_id`: Embedding model identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT EMBEDDING 'text-embedding-ada-002';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_reranker

**Description**  
Sets the default reranker model.

**Usage**  
```
SET DEFAULT RERANKER <reranker_id>;
```

**Parameters**  
- `reranker_id`: Reranker model identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT RERANKER 'bge-reranker-large';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_asr

**Description**  
Sets the default ASR (Automatic Speech Recognition) model.

**Usage**  
```
SET DEFAULT ASR <asr_id>;
```

**Parameters**  
- `asr_id`: ASR model identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT ASR 'whisper-large';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### set_default_tts

**Description**  
Sets the default TTS (Text-to-Speech) model.

**Usage**  
```
SET DEFAULT TTS <tts_id>;
```

**Parameters**  
- `tts_id`: TTS model identifier, quoted string.

**Example**  
```
ragflow> SET DEFAULT TTS 'tts-1';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_llm

**Description**  
Resets the default LLM to system default.

**Usage**  
```
RESET DEFAULT LLM;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT LLM;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_vlm

**Description**  
Resets the default VLM to system default.

**Usage**  
```
RESET DEFAULT VLM;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT VLM;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_embedding

**Description**  
Resets the default embedding model to system default.

**Usage**  
```
RESET DEFAULT EMBEDDING;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT EMBEDDING;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_reranker

**Description**  
Resets the default reranker model to system default.

**Usage**  
```
RESET DEFAULT RERANKER;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT RERANKER;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_asr

**Description**  
Resets the default ASR model to system default.

**Usage**  
```
RESET DEFAULT ASR;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT ASR;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### reset_default_tts

**Description**  
Resets the default TTS model to system default.

**Usage**  
```
RESET DEFAULT TTS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> RESET DEFAULT TTS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### create_user_dataset_with_parser

**Description**  
Creates a user dataset with the specified parser.

**Usage**  
```
CREATE DATASET <dataset_name> WITH EMBEDDING <embedding> PARSER <parser_type>;
```

**Parameters**  
- `dataset_name`: Dataset name, quoted string.
- `embedding`: Embedding model name, quoted string.
- `parser_type`: Parser type, quoted string.

**Example**  
```
ragflow> CREATE DATASET 'my_dataset' WITH EMBEDDING 'text-embedding-ada-002' PARSER 'pdf';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### create_user_dataset_with_pipeline

**Description**  
Creates a user dataset with the specified pipeline.

**Usage**  
```
CREATE DATASET <dataset_name> WITH EMBEDDING <embedding> PIPELINE <pipeline>;
```

**Parameters**  
- `dataset_name`: Dataset name, quoted string.
- `embedding`: Embedding model name, quoted string.
- `pipeline`: Pipeline name, quoted string.

**Example**  
```
ragflow> CREATE DATASET 'my_dataset' WITH EMBEDDING 'text-embedding-ada-002' PIPELINE 'standard';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### drop_user_dataset

**Description**  
Deletes a user dataset.

**Usage**  
```
DROP DATASET <dataset_name>;
```

**Parameters**  
- `dataset_name`: Name of the dataset to delete, quoted string.

**Example**  
```
ragflow> DROP DATASET 'my_dataset';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_datasets

**Description**  
Lists all datasets for the current user.

**Usage**  
```
LIST DATASETS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> LIST DATASETS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_dataset_files

**Description**  
Lists all files in the specified dataset.

**Usage**  
```
LIST FILES OF DATASET <dataset_name>;
```

**Parameters**  
- `dataset_name`: Dataset name, quoted string.

**Example**  
```
ragflow> LIST FILES OF DATASET 'my_dataset';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_agents

**Description**  
Lists all agents for the current user.

**Usage**  
```
LIST AGENTS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> LIST AGENTS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_chats

**Description**  
Lists all chat sessions for the current user.

**Usage**  
```
LIST CHATS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> LIST CHATS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### create_user_chat

**Description**  
Creates a new chat session.

**Usage**  
```
CREATE CHAT <chat_name>;
```

**Parameters**  
- `chat_name`: Chat session name, quoted string.

**Example**  
```
ragflow> CREATE CHAT 'my_chat';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### drop_user_chat

**Description**  
Deletes a chat session.

**Usage**  
```
DROP CHAT <chat_name>;
```

**Parameters**  
- `chat_name`: Name of the chat session to delete, quoted string.

**Example**  
```
ragflow> DROP CHAT 'my_chat';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_model_providers

**Description**  
Lists all model providers for the current user.

**Usage**  
```
LIST MODEL PROVIDERS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> LIST MODEL PROVIDERS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### list_user_default_models

**Description**  
Lists all default model settings for the current user.

**Usage**  
```
LIST DEFAULT MODELS;
```

**Parameters**  
No parameters.

**Example**  
```
ragflow> LIST DEFAULT MODELS;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### import_docs_into_dataset

**Description**  
Imports documents into the specified dataset.

**Usage**  
```
IMPORT <document_list> INTO DATASET <dataset_name>;
```

**Parameters**  
- `document_list`: List of document paths, multiple paths can be separated by commas, or as a space-separated quoted string.
- `dataset_name`: Target dataset name, quoted string.

**Example**  
```
ragflow> IMPORT '/path/to/doc1.pdf,/path/to/doc2.pdf' INTO DATASET 'my_dataset';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### search_on_datasets

**Description**  
Searches in one or more specified datasets.

**Usage**  
```
SEARCH <question> ON DATASETS <dataset_list>;
```

**Parameters**  
- `question`: Search question, quoted string.
- `dataset_list`: List of dataset names, multiple names can be separated by commas, or as a space-separated quoted string.

**Example**  
```
ragflow> SEARCH 'What is RAG?' ON DATASETS 'dataset1,dataset2';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### parse_dataset_docs

**Description**  
Parses specified documents in a dataset.

**Usage**  
```
PARSE <document_names> OF DATASET <dataset_name>;
```

**Parameters**  
- `document_names`: List of document names, multiple names can be separated by commas, or as a space-separated quoted string.
- `dataset_name`: Dataset name, quoted string.

**Example**  
```
ragflow> PARSE 'doc1.pdf,doc2.pdf' OF DATASET 'my_dataset';
```

**Display Effect**  
(Sample output will be provided by the user)

---

### parse_dataset_sync

**Description**  
Synchronously parses the entire dataset.

**Usage**  
```
PARSE DATASET <dataset_name> SYNC;
```

**Parameters**  
- `dataset_name`: Dataset name, quoted string.

**Example**  
```
ragflow> PARSE DATASET 'my_dataset' SYNC;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### parse_dataset_async

**Description**  
Asynchronously parses the entire dataset.

**Usage**  
```
PARSE DATASET <dataset_name> ASYNC;
```

**Parameters**  
- `dataset_name`: Dataset name, quoted string.

**Example**  
```
ragflow> PARSE DATASET 'my_dataset' ASYNC;
```

**Display Effect**  
(Sample output will be provided by the user)

---

### benchmark

**Description**  
Performs performance benchmark testing on the specified user command.

**Usage**  
```
BENCHMARK <concurrency> <iterations> <user_command>;
```

**Parameters**  
- `concurrency`: Concurrency number, positive integer.
- `iterations`: Number of iterations, positive integer.
- `user_command`: User command to test (must be a valid user command, such as `PING;`).

**Example**  
```
ragflow> BENCHMARK 5 10 PING;
```

**Display Effect**  
(Sample output will be provided by the user)

---

**Notes**  
- All string parameters (such as names, IDs, paths) must be enclosed in single quotes (`'`) or double quotes (`"`).
- Commands must end with a semicolon (`;`).
- The prompt is `ragflow>`.
