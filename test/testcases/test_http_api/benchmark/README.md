# RAGFlow HTTP Benchmark CLI

Run (from repo root):
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark [global flags] <chat|retrieval> [command flags]
  Global flags can be placed before or after the command.

If you run from another directory:
  PYTHONPATH=/Directory_name/ragflow/test uv run -m testcases.test_http_api.benchmark [global flags] <chat|retrieval> [command flags]

JSON args:
  For --dataset-payload, --chat-payload, --messages-json, --extra-body, --payload
  - Pass inline JSON: '{"key": "value"}'
  - Or use a file: '@/path/to/file.json'

Global flags
  --base-url
    Base server URL.
    Env: RAGFLOW_BASE_URL or HOST_ADDRESS
  --api-version
    API version string (default: v1).
    Env: RAGFLOW_API_VERSION
  --api-key
    API key for Authorization: Bearer <token>.
  --connect-timeout
    Connect timeout seconds (default: 5.0).
  --read-timeout
    Read timeout seconds (default: 60.0).
  --no-verify-ssl
    Disable SSL verification.
  --iterations
    Iterations per benchmark (default: 1).
  --concurrency
    Concurrency (forced to 1). Any other value is ignored with a warning.
  --json
    Output JSON report (plain stdout).
  --print-response
    Print response content per iteration (stdout). With --json, responses are included in the JSON output.
  --response-max-chars
    Truncate printed responses to N chars (0 = no limit).

Auth and bootstrap flags (used when --api-key is not provided)
  --login-email
    Login email.
    Env: RAGFLOW_EMAIL
  --login-nickname
    Nickname for registration. If omitted, defaults to email prefix when registering.
    Env: RAGFLOW_NICKNAME
  --login-password
    Login password (encrypted client-side). Requires pycryptodomex in the test group.
  --allow-register
    Attempt /user/register before login (best effort).
  --token-name
    Optional API token name for /system/new_token.
  --bootstrap-llm
    Ensure LLM factory API key is configured via /llm/set_api_key.
  --llm-factory
    LLM factory name for bootstrap.
    Env: RAGFLOW_LLM_FACTORY
  --llm-api-key
    LLM API key for bootstrap.
    Env: ZHIPU_AI_API_KEY
  --llm-api-base
    Optional LLM API base URL.
    Env: RAGFLOW_LLM_API_BASE
  --set-tenant-info
    Set tenant defaults via /user/set_tenant_info.
  --tenant-llm-id
    Tenant chat model ID.
    Env: RAGFLOW_TENANT_LLM_ID
  --tenant-embd-id
    Tenant embedding model ID.
    Env: RAGFLOW_TENANT_EMBD_ID
  --tenant-img2txt-id
    Tenant image2text model ID.
    Env: RAGFLOW_TENANT_IMG2TXT_ID
  --tenant-asr-id
    Tenant ASR model ID (default empty).
    Env: RAGFLOW_TENANT_ASR_ID
  --tenant-tts-id
    Tenant TTS model ID.
    Env: RAGFLOW_TENANT_TTS_ID

Dataset/document flags (shared by chat and retrieval)
  --dataset-id
    Existing dataset ID.
  --dataset-ids
    Comma-separated dataset IDs.
  --dataset-name
    Dataset name when creating a new dataset.
    Env: RAGFLOW_DATASET_NAME
  --dataset-payload
    JSON body for dataset creation (see API docs).
  --document-path
    Document path to upload (repeatable).
  --document-paths-file
    File containing document paths, one per line.
  --parse-timeout
    Document parse timeout seconds (default: 120.0).
  --parse-interval
    Document parse poll interval seconds (default: 1.0).
  --teardown
    Delete created resources after run.

Chat command flags
  --chat-id
    Existing chat ID. If omitted, a chat is created.
  --chat-name
    Chat name when creating a new chat.
    Env: RAGFLOW_CHAT_NAME
  --chat-payload
    JSON body for chat creation (see API docs).
  --model
    Model field for OpenAI-compatible completion request.
    Env: RAGFLOW_CHAT_MODEL
  --message
    Single user message (required unless --messages-json is provided).
  --messages-json
    JSON list of OpenAI-format messages (required unless --message is provided).
  --extra-body
    JSON extra_body for OpenAI-compatible request.

Retrieval command flags
  --question
    Retrieval question (required unless provided in --payload).
  --payload
    JSON body for /api/v1/retrieval (see API docs).
  --document-ids
    Comma-separated document IDs for retrieval.

Model selection guidance
  - Embedding model is tied to the dataset.
    Set during dataset creation using --dataset-payload:
      {"name": "...", "embedding_model": "<model_name>@<provider>"}
    Or set tenant defaults via --set-tenant-info with --tenant-embd-id.
  - Chat model is tied to the chat assistant.
    Set during chat creation using --chat-payload:
      {"name": "...", "llm": {"model_name": "<model_name>@<provider>"}}
    Or set tenant defaults via --set-tenant-info with --tenant-llm-id.
  - --model is required by the OpenAI-compatible endpoint but does not override
    the chat assistant's configured model on the server.

What this CLI can do
  - This is a benchmark CLI. It always runs either a chat or retrieval benchmark
    and prints a report.
  - It can create datasets, upload documents, trigger parsing, and create chats
    as part of a benchmark run (setup for the benchmark).
  - It is not a general admin CLI; there are no standalone "create-only" or
    "manage" commands. Use the reports to capture created IDs for reuse.

Do I need the dataset ID?
  - If the CLI creates a dataset, it uses the returned dataset ID internally.
    You do not need to supply it for that same run.
  - The report prints "Created Dataset ID" so you can reuse it later with
    --dataset-id or --dataset-ids.
  - Dataset name is only used at creation time. Selection is always by ID.

Examples

Example: chat benchmark creating dataset + upload + parse + chat (login + register)
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark chat \
    --base-url http://127.0.0.1:9380 \
    --allow-register \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --bootstrap-llm \
    --llm-factory ZHIPU-AI \
    --llm-api-key $ZHIPU_AI_API_KEY \
    --dataset-name "bench_dataset" \
    --dataset-payload '{"name":"bench_dataset","embedding_model":"embedding-2@ZHIPU-AI"}' \
    --document-path /path/to/doc1.pdf \
    --document-path /path/to/doc2.pdf \
    --chat-name "bench_chat" \
    --chat-payload '{"name":"bench_chat","llm":{"model_name":"glm-4-flash@ZHIPU-AI"}}' \
    --message "Say this is a test!" \
    --model "glm-4-flash@ZHIPU-AI"

Example: chat benchmark with existing dataset + chat id (no creation)
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark chat \
    --base-url http://127.0.0.1:9380 \
    --chat-id <existing_chat_id> \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --message "Say this is a test!" \
    --model "glm-4-flash@ZHIPU-AI"

Example: retrieval benchmark creating dataset + upload + parse
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark retrieval \
    --base-url http://127.0.0.1:9380 \
    --allow-register \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --bootstrap-llm \
    --llm-factory ZHIPU-AI \
    --llm-api-key $ZHIPU_AI_API_KEY \
    --dataset-name "bench_dataset" \
    --dataset-payload '{"name":"bench_dataset","embedding_model":"embedding-2@ZHIPU-AI"}' \
    --document-path /path/to/file/Doc1.pdf \
    --document-path /path/to/file/Doc2.pdf \
    --question "What is the longest lasting empire"

Example: retrieval benchmark with existing dataset IDs
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark retrieval \
    --base-url http://127.0.0.1:9380 \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --dataset-ids "<dataset_id_1>,<dataset_id_2>" \
    --question "What is advantage of ragflow?"

Example: retrieval benchmark with existing dataset IDs and document IDs
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark retrieval \
    --base-url http://127.0.0.1:9380 \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --dataset-id "<dataset_id>" \
    --document-ids "<doc_id_1>,<doc_id_2>" \
    --question "What is advantage of ragflow?"

Example: using a document list file (multi-file selection)
  PYTHONPATH=./test uv run -m testcases.test_http_api.benchmark retrieval \
    --base-url http://127.0.0.1:9380 \
    --login-email "<login_email>" \
    --login-password "<password>" \
    --dataset-name "bench_dataset" \
    --dataset-payload '{"name":"bench_dataset","embedding_model":"embedding-2@ZHIPU-AI"}' \
    --document-paths-file /path/to/document_paths.txt \
    --question "What is advantage of ragflow?"
