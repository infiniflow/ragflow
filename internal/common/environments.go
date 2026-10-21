//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package common

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func GetEnv(key string) string {
	return os.Getenv(key)
}

// GetEnvInt reads a whole-number setting, falling back to def when the variable
// is unset or not a number. A typo degrades to the default rather than to zero:
// zero is a meaningful value for several of these knobs (a disabled ceiling), so
// treating a malformed value as zero would silently turn a limit off.
func GetEnvInt(key string, def int) int {
	raw := strings.TrimSpace(GetEnv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func GetEnvSmall(key string) string {
	return strings.ToLower(GetEnv(key))
}

// SandboxArtifactBucket is the object-storage bucket that holds
// code-exec sandbox artifacts, served back through
// /api/v1/documents/artifact/<name>.
func SandboxArtifactBucket() string {
	if bucket := GetEnv(EnvSandboxArtifactBucket); bucket != "" {
		return bucket
	}
	return "sandbox-artifacts"
}

// SandboxArtifactContentTypes maps the sandbox-artifact file extensions
// the /api/v1/documents/artifact route serves to response content
// types. Artifact publication derives storage-name extensions from the
// same table so every published URL resolves to a servable type.
var SandboxArtifactContentTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".svg":  "image/svg+xml",
	".pdf":  "application/pdf",
	".csv":  "text/csv",
	".json": "application/json",
	".html": "text/html",
}

func IsLLMDebugEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(GetEnv(EnvLLMDebug)))
	return err == nil && enabled
}

// environment variables
const (
	EnvTensorrtDLAServer                 = "TENSORRT_DLA_SVR"
	EnvRAGFlowDevMode                    = "RAGFLOW_DEV_MODE"
	EnvRAGFlowTTSCacheTTLSeconds         = "RAGFLOW_TTS_CACHE_TTL_SECONDS"
	EnvRerankTokenLimitMode              = "RERANK_TOKEN_LIMIT_MODE"
	EnvComponentExecTimeout              = "COMPONENT_EXEC_TIMEOUT"
	EnvMCPAllowPrivateHosts              = "RAGFLOW_MCP_ALLOW_PRIVATE_HOSTS"
	EnvDocEngine                         = "DOC_ENGINE"
	EnvMaxFileNumPerUser                 = "MAX_FILE_NUM_PER_USER"
	EnvMaxContentLength                  = "MAX_CONTENT_LENGTH"
	EnvRAGFlowDictPath                   = "RAGFLOW_DICT_PATH"
	EnvDefaultSuperuserEmail             = "DEFAULT_SUPERUSER_EMAIL"
	EnvDefaultSuperuserNickname          = "DEFAULT_SUPERUSER_NICKNAME"
	EnvDefaultSuperuserPassword          = "DEFAULT_SUPERUSER_PASSWORD"
	EnvAdminDefaultPassword              = "ADMIN_DEFAULT_PASSWORD" // docker/.env alias of DEFAULT_SUPERUSER_PASSWORD
	EnvDBType                            = "DB_TYPE"
	EnvDevice                            = "DEVICE"
	EnvStorageImpl                       = "STORAGE_IMPL"
	EnvStageHandCacheCap                 = "STAGEHAND_CACHE_CAP"
	EnvStageHandCacheTTLSeconds          = "STAGEHAND_CACHE_TTL_SECONDS"
	EnvStageHandCacheSweepInterval       = "STAGEHAND_CACHE_SWEEP_INTERVAL"
	EnvStageHandExtractResultFile        = "STAGEHAND_EXTRACT_RESULT_FILE"
	EnvAgentRunAccessKeyID               = "AGENTRUN_ACCESS_KEY_ID"
	EnvAgentRunAccessKeySecret           = "AGENTRUN_ACCESS_KEY_SECRET"
	EnvAgentRunAccountID                 = "AGENTRUN_ACCOUNT_ID"
	EnvAgentRunRegion                    = "AGENTRUN_REGION"
	EnvAgentRunTemplateName              = "AGENTRUN_TEMPLATE_NAME"
	EnvAgentRunExecuteHost               = "AGENTRUN_EXECUTE_HOST"
	EnvAgentRunTimeout                   = "AGENTRUN_TIMEOUT"
	EnvE2BTemplate                       = "E2B_TEMPLATE"
	EnvE2BTemplateName                   = "E2B_TEMPLATE_NAME"
	EnvE2BTimeout                        = "E2B_TIMEOUT"
	EnvE2BAPIURL                         = "E2B_API_URL"
	EnvE2BAPIKey                         = "E2B_API_KEY"
	EnvE2BAccessToken                    = "E2B_ACCESS_TOKEN"
	EnvE2BDomain                         = "E2B_DOMAIN"
	EnvTenkiAPIKey                       = "TENKI_API_KEY"
	EnvTenkiAPIURL                       = "TENKI_API_URL"
	EnvTenkiImage                        = "TENKI_IMAGE"
	EnvTenkiTimeout                      = "TENKI_TIMEOUT"
	EnvTenkiAllowOutbound                = "TENKI_ALLOW_OUTBOUND"
	EnvUCloudSandboxAPIKey               = "UCLOUD_SANDBOX_API_KEY"
	EnvUCloudSandboxRegion               = "UCLOUD_SANDBOX_REGION"
	EnvUCloudSandboxDomain               = "UCLOUD_SANDBOX_DOMAIN"
	EnvUCloudSandboxAPIURL               = "UCLOUD_SANDBOX_API_URL"
	EnvUCloudSandboxTemplate             = "UCLOUD_SANDBOX_TEMPLATE"
	EnvUCloudSandboxAllowInternetAccess  = "UCLOUD_SANDBOX_ALLOW_INTERNET_ACCESS"
	EnvUCloudSandboxInsecureHTTP         = "UCLOUD_SANDBOX_INSECURE_HTTP"
	EnvUCloudSandboxExecutionTimeout     = "UCLOUD_SANDBOX_EXECUTION_TIMEOUT"
	EnvUCloudSandboxTimeout              = "UCLOUD_SANDBOX_TIMEOUT"
	EnvUCloudSandboxMaxOutputBytes       = "UCLOUD_SANDBOX_MAX_OUTPUT_BYTES"
	EnvUCloudSandboxMaxArtifacts         = "UCLOUD_SANDBOX_MAX_ARTIFACTS"
	EnvUCloudSandboxMaxArtifactBytes     = "UCLOUD_SANDBOX_MAX_ARTIFACT_BYTES"
	EnvLocalPythonBin                    = "LOCAL_PYTHON_BIN"
	EnvLocalNodeBin                      = "LOCAL_NODE_BIN"
	EnvLocalWorkDir                      = "LOCAL_WORK_DIR"
	EnvLocalTimeout                      = "LOCAL_TIMEOUT"
	EnvLocalMaxMemoryMB                  = "LOCAL_MAX_MEMORY_MB"
	EnvLocalMaxOutputBytes               = "LOCAL_MAX_OUTPUT_BYTES"
	EnvLocalMaxArtifacts                 = "LOCAL_MAX_ARTIFACTS"
	EnvLocalMaxArtifactBytes             = "LOCAL_MAX_ARTIFACT_BYTES"
	EnvPath                              = "PATH"
	EnvOMPNumThreads                     = "OMP_NUM_THREADS"
	EnvOpenBLASNumThreads                = "OPENBLAS_NUM_THREADS"
	EnvMKLNumThreads                     = "MKL_NUM_THREADS"
	EnvVECLIBMaximumThreads              = "VECLIB_MAXIMUM_THREADS"
	EnvNumEXPRNumThreads                 = "NUMEXPR_NUM_THREADS"
	EnvXDGCacheHome                      = "XDG_CACHE_HOME"
	EnvOpenAIAPIKey                      = "OPENAI_API_KEY"
	EnvOpenAIBaseURL                     = "OPENAI_BASE_URL"
	EnvOpenAIModel                       = "OPENAI_MODEL"
	EnvLLMDebug                          = "LLM_DEBUG"
	EnvStageHandExtractSchemaJSON        = "STAGEHAND_EXTRACT_SCHEMA_JSON"
	EnvSandboxProviderType               = "SANDBOX_PROVIDER_TYPE"
	EnvSandboxExecutorManagerURL         = "SANDBOX_EXECUTOR_MANAGER_URL"
	EnvSandboxExecutorManagerTimeout     = "SANDBOX_EXECUTOR_MANAGER_TIMEOUT"
	EnvSandboxExecutorManagerAPIToken    = "SANDBOX_EXECUTOR_MANAGER_API_TOKEN"
	EnvSandboxBasePythonImage            = "SANDBOX_BASE_PYTHON_IMAGE"
	EnvSandboxBaseNodeJSImage            = "SANDBOX_BASE_NODEJS_IMAGE"
	EnvSandboxArtifactBucket             = "SANDBOX_ARTIFACT_BUCKET"
	EnvSSHHost                           = "SSH_HOST"
	EnvSSHPort                           = "SSH_PORT"
	EnvSSHUsername                       = "SSH_USERNAME"
	EnvSSHPassword                       = "SSH_PASSWORD"
	EnvSSHPrivateKey                     = "SSH_PRIVATE_KEY"
	EnvSSHPrivateKeyPath                 = "SSH_PRIVATE_KEY_PATH"
	EnvSSHPassphrase                     = "SSH_PASSPHRASE"
	EnvSSHPythonBin                      = "SSH_PYTHON_BIN"
	EnvSSHNodeBin                        = "SSH_NODE_BIN"
	EnvSSHWorkDir                        = "SSH_WORK_DIR"
	EnvSSHTimeout                        = "SSH_TIMEOUT"
	EnvSSHMaxOutputBytes                 = "SSH_MAX_OUTPUT_BYTES"
	EnvSSHMaxArtifacts                   = "SSH_MAX_ARTIFACTS"
	EnvSSHMaxArtifactBytes               = "SSH_MAX_ARTIFACT_BYTES"
	EnvSSHKnownHosts                     = "SSH_KNOWN_HOSTS"
	EnvTrinoUseTls                       = "TRINO_USE_TLS"
	EnvSSHEnableAPIURL                   = "SSH_ENABLE_API_URL"
	EnvAllowAnyHost                      = "ALLOW_ANY_HOST"
	EnvTavilyAPIKey                      = "TAVILY_API_KEY"
	EnvQueritAPIKey                      = "QUERIT_API_KEY"
	EnvHome                              = "HOME"
	EnvUserProfile                       = "USERPROFILE"
	EnvHTTPProxy                         = "http_proxy"
	EnvHTTPSProxy                        = "https_proxy"
	EnvBatchSingle                       = "BATCH_SINGLE"
	EnvBatchCount                        = "BATCH_COUNT"
	EnvBatchLogLevel                     = "BATCH_LOG_LEVEL"
	EnvBatchCompareOnly                  = "BATCH_COMPARE_ONLY"
	EnvBatchCompareFilter                = "BATCH_COMPARE_FILTER"
	EnvBatchCompareCSV                   = "BATCH_COMPARE_CSV"
	EnvPYOCRSuffix                       = "PY_OCR_SUFFIX"
	EnvUpdateGolden                      = "UPDATE_GOLDEN"
	EnvBatchParityFilter                 = "BATCH_PARITY_FILTER"
	EnvBatchParityVariant                = "BATCH_PARITY_VARIANT"
	EnvBatchParityDataRoot               = "BATCH_PARITY_DATA_ROOT"
	EnvDumpCount                         = "DUMP_COUNT"
	EnvBatchCSV                          = "BATCH_CSV"
	EnvESTest                            = "ES_TEST"
	EnvESHost                            = "ES_HOST"
	EnvESUsername                        = "ES_USERNAME"
	EnvESPassword                        = "ES_PASSWORD"
	EnvESIndexPrefix                     = "ES_INDEX_PREFIX"
	EnvGiteeListModelsIntegration        = "GITEE_LIST_MODELS_INTEGRATION"
	EnvGiteeBaseUrl                      = "GITEE_BASE_URL"
	EnvGiteeAPIKey                       = "GITEE_API_KEY"
	EnvRAGFlowAPITiming                  = "RAGFLOW_API_TIMING"
	EnvInfinityURI                       = "INFINITY_URI"
	EnvDoclingServerURL                  = "DOCLING_SERVER_URL"
	EnvDoclingAPIKey                     = "DOCLING_API_KEY"
	EnvDoclingDoOCR                      = "DOCLING_DO_OCR"
	EnvDoclingPDFBackend                 = "DOCLING_PDF_BACKEND"
	EnvMineruAPIServer                   = "MINERU_APISERVER"
	EnvMineruAPIKey                      = "MINERU_API_KEY"
	EnvMineruBackend                     = "MINERU_BACKEND"
	EnvMineruServerURL                   = "MINERU_SERVER_URL"
	EnvMonkeyOCRv2ServerURL              = "MONKEYOCRV2_SERVER_URL"
	EnvMonkeyOCRv2Timeout                = "MONKEYOCRV2_TIMEOUT"
	EnvMonkeyOCRAPIServer                = "MONKEYOCR_APISERVER"
	EnvMonkeyOCROutputDir                = "MONKEYOCR_OUTPUT_DIR"
	EnvMonkeyOCRServerURL                = "MONKEYOCR_SERVER_URL"
	EnvMonkeyOCRBackend                  = "MONKEYOCR_BACKEND"
	EnvMonkeyOCRDeleteOutput             = "MONKEYOCR_DELETE_OUTPUT"
	EnvOpenDataLoaderAPIServer           = "OPENDATALOADER_APISERVER"
	EnvOpenDataLoaderAPIKey              = "OPENDATALOADER_API_KEY"
	EnvPaddleOCRBaseUrl                  = "PADDLEOCR_BASE_URL"
	EnvPaddleOCRAPIURL                   = "PADDLEOCR_API_URL"
	EnvPaddleOCRAccessToken              = "PADDLEOCR_ACCESS_TOKEN"
	EnvPaddleOCRAlgorithm                = "PADDLEOCR_ALGORITHM"
	EnvSOMarkBaseUrl                     = "SOMARK_BASE_URL"
	EnvSOMarkAPIKey                      = "SOMARK_API_KEY"
	EnvSOMarkImageFormat                 = "SOMARK_IMAGE_FORMAT"
	EnvSOMarkFormulaFormat               = "SOMARK_FORMULA_FORMAT"
	EnvSOMarkTableFormat                 = "SOMARK_TABLE_FORMAT"
	EnvSOMarkCSFormat                    = "SOMARK_CS_FORMAT"
	EnvSOMarkEnableTextCrossPage         = "SOMARK_ENABLE_TEXT_CROSS_PAGE"
	EnvSOMarkEnableTableCrossPage        = "SOMARK_ENABLE_TABLE_CROSS_PAGE"
	EnvSOMarkEnableTitleLevelRecognition = "SOMARK_ENABLE_TITLE_LEVEL_RECOGNITION"
	EnvSOMarkEnableInlineImage           = "SOMARK_ENABLE_INLINE_IMAGE"
	EnvSOMarkEnableTableImage            = "SOMARK_ENABLE_TABLE_IMAGE"
	EnvSOMarkEnableImageUnderstanding    = "SOMARK_ENABLE_IMAGE_UNDERSTANDING"
	EnvSOMarkKeepHeaderFooter            = "SOMARK_KEEP_HEADER_FOOTER"
	EnvTCADPAPIServerURL                 = "TCADP_APISERVER_URL"
	EnvTCADPAPIKey                       = "TCADP_API_KEY"
	EnvFirecrawlAPIKey                   = "FIRECRAWL_API_KEY"
	EnvFirecrawlAPIURL                   = "FIRECRAWL_API_URL"
	EnvFirecrawlMaxRetries               = "FIRECRAWL_MAX_RETRIES"
	EnvFirecrawlTimeout                  = "FIRECRAWL_TIMEOUT"
	EnvFirecrawlRelayTimeout             = "FIRECRAWL_RELAY_TIMEOUT"
	EnvRAGFlowSecretKey                  = "RAGFLOW_SECRET_KEY"
	EnvEnableRegister                    = "ENABLE_REGISTER"
	EnvDisablePasswordLogin              = "DISABLE_PASSWORD_LOGIN"
	EnvMinioHost                         = "MINIO_HOST"
	EnvMinioRegion                       = "MINIO_REGION"
	EnvLang                              = "LANG"
	EnvLanguage                          = "LANGUAGE"
	EnvChunkFeedbackEnabled              = "CHUNK_FEEDBACK_ENABLED"
	EnvChunkFeedbackWeighting            = "CHUNK_FEEDBACK_WEIGHTING"
	EnvComposeProfiles                   = "COMPOSE_PROFILES"
	EnvTEIModel                          = "TEI_MODEL"
	EnvTEIHost                           = "TEI_HOST"
	EnvTEIBaseURL                        = "TEI_BASE_URL"
	EnvRAGFlowConfDir                    = "RAGFLOW_CONF_DIR"
	EnvRAGProjectBase                    = "RAG_PROJECT_BASE"
	EnvRAGDeployBase                     = "RAG_DEPLOY_BASE"
	EnvRAGFlowTestEnvIntOrUnset          = "RAGFLOW_TEST_ENVINTOR_UNSET"
	EnvRAGFlowTestEnvIntOr               = "RAGFLOW_TEST_ENVINTOR"
	EnvRAGFlowTestEnvOr                  = "RAGFLOW_TEST_ENVOR"
	EnvRAGFlowTestEnvOrUnset             = "RAGFLOW_TEST_ENVOR_UNSET"
	EnvKeenableAPIURL                    = "KEENABLE_API_URL"
	EnvBoxWebOAuthRedirectURI            = "BOX_WEB_OAUTH_REDIRECT_URI"
	EnvGmailWebOAuthRedirectURI          = "GMAIL_WEB_OAUTH_REDIRECT_URI"
	EnvGoogleDriveWebOAuthRedirectURI    = "GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI"
	EnvSpacyModelDir                     = "SPACY_MODEL_DIR"

	// EnvExtractorChatTimeout sets the Extractor LLM call timeout in seconds,
	// including retries and backoff. It does not apply to tag extraction.
	EnvExtractorChatTimeout = "EXTRACTOR_CHAT_TIMEOUT"

	// EnvDeepDocModelDir points the in-process (Go) DeepDoc backend at the
	// model snapshot (see common.DeepDocModelFiles); mirrors
	// the RAGFlow default model dir (internal/rag/res/deepdoc).
	EnvDeepDocModelDir = "DEEPDOC_MODEL_DIR"
	// EnvDeepDocDropScore overrides the confidence threshold below which the
	// in-process (Go) DeepDoc backend blanks recognized text while preserving
	// the real score. It defaults to 0.5, matching the historical DeepDoc
	// recognizer drop_score, so recognized text is blanked consistently.
	EnvDeepDocDropScore = "DEEPDOC_DROP_SCORE"
	// EnvDeepDocInferenceConcurrency bounds how many DeepDoc ONNX inference
	// Runs may be in flight at once. Each Run opens with max(1, N/K) intra-op
	// threads (N = EnvDeepDocInferenceCPUCores budget, K = this value), so the
	// total cores inference may occupy is at most N. It overrides the
	// ingestor.inference_concurrency config key and is itself overridden by the
	// --deepdoc-inference-concurrency CLI flag.
	EnvDeepDocInferenceConcurrency = "RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY"
	// EnvIngestorMaxConcurrentWorkers bounds how many ingestion tasks the
	// ingestor runs in parallel (the NATS consumer worker count). It overrides
	// the ingestor.max_concurrent_workers config key and is itself overridden
	// by the --ingestor-max-concurrent-workers CLI flag.
	EnvIngestorMaxConcurrentWorkers = "RAGFLOW_INGESTOR_MAX_CONCURRENT_WORKERS"
	// EnvIngestorPageConcurrency bounds how many pages of a single document are
	// parsed concurrently inside one ingestor worker. It overrides the
	// ingestor.page_concurrency config key and is itself overridden by the
	// --ingestor-page-concurrency CLI flag.
	EnvIngestorPageConcurrency = "RAGFLOW_INGESTOR_PAGE_CONCURRENCY"
	// EnvDeepDocInferenceCPUCores bounds the CPU-core budget N for DeepDoc
	// in-process inference (0 = all cores). It overrides the
	// ingestor.inference_cpu_cores config key and is itself overridden by the
	// --deepdoc-inference-cpu-cores CLI flag.
	EnvDeepDocInferenceCPUCores = "RAGFLOW_DEEPDOC_INFERENCE_CPU_CORES"

	// Typed-decision model. Unset RAGFLOW_DECISION_DIR leaves the feature off:
	// every caller degrades to unfiltered behaviour, so a deployment without the
	// model directory is indistinguishable from one that never had the feature.
	EnvDecisionDir            = "RAGFLOW_DECISION_DIR"
	EnvDecisionSegmentTokens  = "RAGFLOW_DECISION_SEGMENT_TOKENS"
	EnvDecisionMaxSegments    = "RAGFLOW_DECISION_MAX_SEGMENTS"
	EnvDecisionMinProbability = "RAGFLOW_DECISION_HEAD_MIN_PROB"
	EnvDecisionBudgetMS       = "RAGFLOW_DECISION_HEAD_BUDGET_MS"
	// EnvDecisionHeadURL points at a local decision-head inference sidecar. Set it
	// when the graph runs
	// out of process; the ONNX Runtime linked into this binary is an ORT-format
	// minimal build that cannot load the exported .onnx at all.
	EnvDecisionHeadURL = "RAGFLOW_DECISION_HEAD_URL"
	// Which model-visible exits the sufficiency filter runs on. Deep read is the
	// original (and default) target; the snippet exit is opt-in because dropping
	// a locate hit costs the model an anchor it may have needed to deep-read at
	// all. Both default to their historical behaviour so a deployment that sets
	// only RAGFLOW_DECISION_DIR keeps filtering deep reads and nothing else.
	EnvDecisionFilterDeepRead = "RAGFLOW_DECISION_FILTER_DEEP_READ"
	EnvDecisionFilterSnippet  = "RAGFLOW_DECISION_FILTER_SNIPPET"
	// EnvDecisionSnippetTools replaces the snippet filter's TOOL scope (comma
	// separated). Unset means the corpus-passage locate tools only;
	// search_metadata is never in scope because it returns no passages.
	EnvDecisionSnippetTools = "RAGFLOW_DECISION_FILTER_SNIPPET_TOOLS"
	// EnvDecisionWholeChunk switches the filter's judging window from judged
	// sub-segments to the whole passage, truncated at EnvDecisionWholeChunkTokens.
	// Off by default: the window is a calibration choice, so a deployment that
	// only sets RAGFLOW_DECISION_DIR keeps the window it was measured with.
	EnvDecisionWholeChunk       = "RAGFLOW_DECISION_WHOLE_CHUNK"
	EnvDecisionWholeChunkTokens = "RAGFLOW_DECISION_WHOLE_CHUNK_TOKENS"
	// EnvDecisionQuestionStyle selects what the filter ASKS the model: the contract's
	// trained sufficiency phrasing ("sufficiency", default) or the relevance
	// phrasing ("relevance"). The relevance set is out of the checkpoint's
	// training distribution, so it is an experiment knob, not a quality setting.
	EnvDecisionQuestionStyle = "RAGFLOW_DECISION_QUESTION_STYLE"

	// EnvDecisionKeepTopN exempts the first N passages of every result set from the
	// filter, so the model only shortens the TAIL of a ranking instead of
	// gating each passage on its own. 0 keeps the all-or-nothing behaviour.
	EnvDecisionKeepTopN = "RAGFLOW_DECISION_KEEP_TOP_N"
	// The decision-model backend. It replaces the local decision head with the
	// hosted decision API the head was distilled from, which is judged on the
	// SAME question shape (noul) and measured on the same 68 labelled passages:
	// relevance@0.15 gives precision 0.80 / recall 0.71 against the head's
	// 0.62 / 0.36, and unlike the head it reacts to a passage that says what is
	// missing (0.01 vs 0.94). See plans/20261016-decision-model-judge-task.md §9.
	//
	// EnvDecisionBackend selects the judge: "decision-api" replaces the head
	// everywhere the filter asks for a verdict. Empty keeps the default wording.
	EnvDecisionBackend = "RAGFLOW_DECISION_BACKEND"
	// EnvDecisionURL is the /v1/systemone endpoint.
	EnvDecisionURL = "RAGFLOW_DECISION_URL"
	// EnvDecisionKey is the API key, sent as "Authorization: Bearer <key>".
	EnvDecisionKey = "RAGFLOW_DECISION_KEY"
	// EnvDecisionModel is the model name the endpoint expects.
	EnvDecisionModel = "RAGFLOW_DECISION_MODEL"
	// EnvDecisionMinProb is the P(relevant) at or above which a passage is
	// kept. The default is the point measured on the labelled set, not a round
	// number: the API is deterministic (five identical calls returned the same
	// value to three decimals), so a fitted threshold is a stable one.
	EnvDecisionMinProb = "RAGFLOW_DECISION_MIN_PROB"
	// EnvDecisionMaxChars caps the passage text sent for judgement. The
	// calibration judged passages at 6000 characters.
	EnvDecisionMaxChars = "RAGFLOW_DECISION_MAX_CHARS"
	// EnvDecisionTimeoutMS bounds one judgement. On timeout the passage is
	// KEPT, like every other degraded path: a filter that cannot run must not
	// cost the caller its evidence.
	EnvDecisionTimeoutMS = "RAGFLOW_DECISION_TIMEOUT_MS"
	// EnvDecisionSnippetText selects what the locate exit judges: the chunk
	// BODY (default) or the rendered "snippet" window. The body is the unit the
	// threshold was fitted on and the unit the deep-read exit already uses; the
	// window is what the model is served, but for a truncated grep hit it is one
	// paragraph and for a BM25 preview hit it is the chunk's opening, so judging
	// it asks about an excerpt that may not hold the evidence.
	EnvDecisionSnippetText = "RAGFLOW_DECISION_SNIPPET_TEXT"
	// EnvDecisionConcurrency bounds how many judgements of one result set may be
	// in flight at once. The verdicts are independent — each one asks about a
	// different passage — so the only reason to serialise them is the upstream's
	// rate limit, and serialising them costs the whole sum of their latencies in
	// wall time. Set to 1 to get the old strictly-serial behaviour.
	EnvDecisionConcurrency = "RAGFLOW_DECISION_CONCURRENCY"
	// EnvDecisionBatch is how many passages go into ONE request. The endpoint
	// caps a request at 16 questions and each passage spends one question per
	// phrasing, so the effective value is min(this, 16/phrasings). Batching is
	// what the endpoint offers — one request answering several passages — not
	// client-side concurrency.
	EnvDecisionBatch = "RAGFLOW_DECISION_BATCH"
	// EnvDecisionPhrasings is how many wordings of the same relevance question
	// are asked per passage and averaged. Four is the calibrated setting and
	// what the 0.15 threshold was fitted on; lowering it admits phrasing noise
	// into the decision and requires re-fitting the threshold.
	EnvDecisionPhrasings = "RAGFLOW_DECISION_PHRASINGS"
	// EnvDecisionBreakerFailures is how many judgements may fail in a row before
	// the gate is switched OFF for a cooldown. A failed judgement keeps its
	// passage, so an unreachable judge cannot cost evidence — it costs the
	// timeout per passage, and a run that keeps paying it is the ungated
	// baseline with a much longer wall. 0 or 1 opens on the first failure.
	EnvDecisionBreakerFailures = "RAGFLOW_DECISION_BREAKER_FAILURES"
	// EnvDecisionBreakerCooldownMS is how long the gate stays off before one
	// probe is let through. A probe that succeeds closes the breaker; one that
	// fails re-opens it and restarts the cooldown.
	EnvDecisionBreakerCooldownMS = "RAGFLOW_DECISION_BREAKER_COOLDOWN_MS"
	// EnvAgenticServeOnce makes a passage's BODY reach the model once per run:
	// a later fetch of the same text emits the passage's identity and handle
	// only. Off by default — the model must be able to tell a stub from a loss.
	EnvAgenticServeOnce = "RAGFLOW_AGENTIC_SERVE_ONCE"
	// EnvAgenticMaxIterations caps the ReAct loop per run. The compiled default
	// is 120, which no measured run ever reached (the worst observed was 30
	// turns), so it never bounded anything: measured questions spread from 3 to
	// 30 turns and the tail dominates the run's tokens. Reaching the cap is not
	// an error — the run falls back to synthesizing an answer from the evidence
	// already gathered — so lowering it truncates a still-productive search and
	// trades answer quality for a bounded tail. 0 keeps the compiled default.
	EnvAgenticMaxIterations = "RAGFLOW_AGENTIC_MAX_ITERATIONS"
	// EnvAgenticTokenBudget is the per-run token ceiling: once a run's model
	// calls have spent this many tokens, the run stops and answers from the
	// evidence it already has. 0 disables it.
	//
	// The default is 500k because that is the gap the measurements show: across
	// the recorded runs an ordinary question spends 84-229k, while a runaway one
	// spends 1.4-2.2M for the same single answer. 500k sits above every ordinary
	// question and below every runaway one, so it bounds the tail without
	// touching the body of the distribution.
	EnvAgenticTokenBudget = "RAGFLOW_AGENTIC_TOKEN_BUDGET"
)

// DeepDocModelFiles is the single source of truth for the weights the
// in-process (Go) DeepDoc backend requires to serve. The Go backend consumes
// the FlatBuffer (.ort) serialization — the static ONNX Runtime build linked
// into the Go binary supports .ort only, not the protobuf .onnx format, so
// this slice must NOT re-add the .onnx names; it is the Go presence check,
// not a shared list.
// cmd/ resolves the model directory against it; the native analyzer validates
// file presence against it via HasModelFiles. Order is insignificant (callers
// do set-membership checks); keep it stable so logs and diffs stay readable.
//
// External consumers that re-list these names must stay in sync:
//   - ragflow_deps/download_deps.py re-lists them as DEEPDOC_MODEL_FILES
//     (it fetches the files one by one, so it MUST be edited by hand when this
//     slice changes);
var DeepDocModelFiles = []string{
	"det.ort",
	"layout.ort",
	"tsr.ort",
	"rec.ort",
	"ocr.res",
}

// HasModelFiles reports whether dir contains every file listed in
// DeepDocModelFiles. It is the single presence check shared by the server
// (cmd/ragflow_server.go) and the in-process analyzer (infnative:
// NewAnalyzer / canServe); those call sites must not re-roll this loop.
func HasModelFiles(dir string) bool {
	for _, f := range DeepDocModelFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// DeepDocORTVersion is the onnxruntime native release the in-process (Go)
// DeepDoc backend is built and tested against (e.g. "1.29.0"). It is ONE OF
// TWO raw version declarations that must stay equal (the other is
// ARG ORT_VERSION in Dockerfile) — NOT a single source of truth. The
// download URL and extracted dir name are built from those ORT_VERSION
// constants, not from this one. The Go binding
// (github.com/infiniflow/onnxruntime_go, the org mirror of yalue/onnxruntime_go)
// and the pip onnxruntime== pin must
// track this MINOR version: the binding uses its own release numbering
// (v1.29.0 <-> ORT 1.29.x) and is ABI-compatible with this native release on
// the same minor line. ONNX Runtime is linked statically (libonnxruntime.a),
// so there is no .so / SONAME at runtime.
//
// To bump ORT, the two Go-side native pins above must change together
// (drift breaks the static link or the runtime OrtGetApiBase lookup):
//   - DeepDocORTVersion (here, Go)
//   - ARG ORT_VERSION in Dockerfile
//
// The dev downloader tools/scripts/download_dev_deps.py carries its own copy of
// ORT_VERSION used only to fetch the matching infiniflow/ragflow-build release;
// build.sh no longer greps it, so the Docker build needs no extra COPY of the
// script. Keep all three (this, Dockerfile ARG, the downloader) on the same
// version when bumping.
//
// Separately, keep these on the same ORT minor line but version them
// independently of the Go native lib (see pyproject.toml / development.md):
//   - the onnxruntime== / onnxruntime-gpu== pins in pyproject.toml (Python side)
//   - the onnxruntime_go binding minor in go.mod.
const DeepDocORTVersion = "1.29.0"
