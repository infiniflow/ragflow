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
	"strings"
)

func GetEnv(key string) string {
	return os.Getenv(key)
}

func GetEnvSmall(key string) string {
	return strings.ToLower(GetEnv(key))
}

// environment variables
const (
	EnvTensorrtDLAServer                 = "TENSORRT_DLA_SVR"
	EnvRAGFlowTTSCacheTTLSeconds         = "RAGFLOW_TTS_CACHE_TTL_SECONDS"
	EnvComponentExecTimeout              = "COMPONENT_EXEC_TIMEOUT"
	EnvDocEngine                         = "DOC_ENGINE"
	EnvMaxFileNumPerUser                 = "MAX_FILE_NUM_PER_USER"
	EnvMaxContentLength                  = "MAX_CONTENT_LENGTH"
	EnvRAGFlowDictPath                   = "RAGFLOW_DICT_PATH"
	EnvDefaultSuperuserEmail             = "DEFAULT_SUPERUSER_EMAIL"
	EnvDefaultSuperuserNickname          = "DEFAULT_SUPERUSER_NICKNAME"
	EnvDefaultSuperuserPassword          = "DEFAULT_SUPERUSER_PASSWORD"
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
	EnvStageHandExtractSchemaJSON        = "STAGEHAND_EXTRACT_SCHEMA_JSON"
	EnvSandboxProviderType               = "SANDBOX_PROVIDER_TYPE"
	EnvSandboxExecutorManagerURL         = "SANDBOX_EXECUTOR_MANAGER_URL"
	EnvSandboxExecutorManagerTimeout     = "SANDBOX_EXECUTOR_MANAGER_TIMEOUT"
	EnvSandboxExecutorManagerPoolSize    = "SANDBOX_EXECUTOR_MANAGER_POOL_SIZE"
	EnvSandboxExecutorManagerMaxRetries  = "SANDBOX_EXECUTOR_MANAGER_MAX_RETRIES"
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
	EnvMineruAPIServer                   = "MINERU_APISERVER"
	EnvMineruAPIKey                      = "MINERU_API_KEY"
	EnvMineruBackend                     = "MINERU_BACKEND"
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

	// EnvDeepDocModelDir points the in-process (Go) DeepDoc backend at the
	// model snapshot (see common.DeepDocModelFiles); mirrors
	// deepdoc_server.py's --model-dir (default rag/res/deepdoc).
	EnvDeepDocModelDir = "DEEPDOC_MODEL_DIR"
	// EnvDeepDocDropScore overrides the confidence threshold below which the
	// in-process (Go) DeepDoc backend blanks recognized text while preserving
	// the real score. It MUST match the Python inference service's
	// Recognizer.drop_score (deepdoc/vision/ocr.py, default 0.5) so both
	// backends apply the same text-blanking contract.
	EnvDeepDocDropScore = "DEEPDOC_DROP_SCORE"
)

// DeepDocModelFiles is the single source of truth for the weights the
// in-process (Go) DeepDoc backend and the Python DeepDoc service both
// require. cmd/ resolves the model directory against it; infnative
// validates file presence against it. Order is insignificant (callers do
// set-membership checks); keep it stable so logs and diffs stay readable.
//
// External consumers that re-list these names must stay in sync:
//   - .github/workflows/deepdoc-drift.yml  (MODEL_FILES)
//   - deepdoc/server/download_deps.py      (FILES)
var DeepDocModelFiles = []string{
	"det.onnx",
	"layout.onnx",
	"tsr.onnx",
	"rec.onnx",
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
// DeepDoc backend is built and tested against (e.g. "1.23.2"). It is ONE OF
// THREE raw version declarations that must stay equal (the other two are
// ORT_VERSION in ragflow_deps/download_go_deps.py and ragflow_deps/download_deps.py)
// — NOT a single source of truth. The download URL and extracted dir name are
// built from those ORT_VERSION constants, not from this one. The Go binding
// (github.com/infiniflow/onnxruntime_go, the org mirror of yalue/onnxruntime_go)
// and the pip onnxruntime== pin must
// track this MINOR version: the binding uses its own release numbering
// (v1.23.0 <-> ORT 1.23.x) but is ABI-compatible with this native release on
// the same minor line. ONNX Runtime is linked statically (libonnxruntime.a),
// so there is no .so / SONAME at runtime.
//
// To bump ORT, ALL of the following must change together (drift breaks the
// static link or the runtime OrtGetApiBase lookup):
//   - DeepDocORTVersion (here, Go) AND ORT_VERSION in BOTH
//     ragflow_deps/download_go_deps.py and ragflow_deps/download_deps.py;
//   - the onnxruntime== pin in pyproject.toml and the onnxruntime /
//     onnxruntime-gpu pins in .github/workflows/deepdoc-drift.yml;
//   - the onnxruntime_go binding minor in go.mod.
const DeepDocORTVersion = "1.23.2"
