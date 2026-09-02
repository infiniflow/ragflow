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

package config

import (
	"fmt"
	"net/mail"
	"ragflow/internal/common"
	"strconv"
	"strings"
	"time"
)

type Environments struct {
	Language string `mapstructure:"language"`

	SecretKey string `mapstructure:"secret_key"`

	EnableRegister       *bool `mapstructure:"enable_register"`
	DisablePasswordLogin *bool `mapstructure:"disable_password_login"`

	DocumentEngineType string `mapstructure:"document_engine_type"`
	DatabaseType       string `mapstructure:"database_type"`

	StorageType string `mapstructure:"storage_type"`
	MinioHost   string `mapstructure:"minio_host"`
	MinioRegion string `mapstructure:"minio_region"`

	DefaultSuperUser DefaultSuperUser `mapstructure:"default_super_user"`

	DeviceType string `mapstructure:"device_type"`

	InfinityURL        string `mapstructure:"infinity_url"`         // INFINITY_URI
	RAGFLowConfPath    string `mapstructure:"ragflow_conf_path"`    // RAGFLOW_CONF_DIR
	RAGFlowProjectBase string `mapstructure:"ragflow_project_base"` // RAG_PROJECT_BASE
	RAGFlowDeployBase  string `mapstructure:"ragflow_deploy_base"`  // RAG_DEPLOY_BASE
	SPACYModelPath     string `mapstructure:"spacy_model_path"`     // SPACY_MODEL_DIR

	DictionaryPath         string        `mapstructure:"ragflow_dict_path"`
	TTSCacheTTLSeconds     time.Duration `mapstructure:"ttscache_ttl_seconds"`
	MAXFileNumberPerUser   int           `mapstructure:"max_file_number_per_user"`
	MAXContentLength       int           `mapstructure:"max_content_length"`
	HomePath               string        `mapstructure:"home_path"`
	UserProfile            string        `mapstructure:"user_profile"`
	HTTPProxy              string        `mapstructure:"http_proxy"`
	HTTPSProxy             string        `mapstructure:"https_proxy"`
	LOGAPITiming           bool          `mapstructure:"log_api_timing"`           // RAGFLOW_API_TIMING
	ChunkFeedbackEnabled   bool          `mapstructure:"chunk_feedback_enabled"`   // CHUNK_FEEDBACK_ENABLED
	ChunkFeedbackWeighting string        `mapstructure:"chunk_feedback_weighting"` // CHUNK_FEEDBACK_WEIGHTING

	BoxWebOauthRedirectURL         string `mapstructure:"box_web_oauth_redirect_url"`          // BOX_WEB_OAUTH_REDIRECT_URI
	GmailWebOAuthRedirectURL       string `mapstructure:"gmail_web_oauth_redirect_url"`        // GMAIL_WEB_OAUTH_REDIRECT_URI
	GoogleDriveWebOAuthRedirectURL string `mapstructure:"google_drive_web_oauth_redirect_url"` // GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI

	DeepDocURL                        string `mapstructure:"deep_doc_url"`
	TensorRTDLAServer                 string `mapstructure:"tensorrt_dla_server"`
	OSSDeepDocURL                     string `mapstructure:"oss_deep_doc_url"` // open source deepdoc URL
	DeepDocBatchSingle                string `mapstructure:"deep_doc_batch_single"`
	DeepDocBatchCount                 int    `mapstructure:"deep_doc_batch_count"`
	DeepDocBatchLogLevel              string `mapstructure:"deep_doc_batch_log_level"`
	DeepDocCompareOnly                bool   `mapstructure:"deep_doc_compare_only"`
	DeepDocCompareFilter              string `mapstructure:"deep_doc_compare_filter"`
	DeepDocCompareCSV                 string `mapstructure:"deep_doc_compare_csv"`
	DeepDocBatchCSV                   string `mapstructure:"deep_doc_batch_csv"`
	OpenDataLoaderAPIServer           string `mapstructure:"open_dataloader_api_server"`        // OPENDATALOADER_APISERVER
	OpenDataLoaderAPIKey              string `mapstructure:"open_dataloader_api_key"`           // OPENDATALOADER_API_KEY
	PaddleOCRBaseURL                  string `mapstructure:"paddle_ocr_base_url"`               // PADDLEOCR_BASE_URL
	PaddleOCRAPIURL                   string `mapstructure:"paddle_ocr_api_url"`                // PADDLEOCR_API_URL
	PaddleOCRAccessToken              string `mapstructure:"paddle_ocr_access_token"`           // PADDLEOCR_ACCESS_TOKEN
	PaddleOCRAlgorithm                string `mapstructure:"paddle_ocr_algorithm"`              // PADDLEOCR_ALGORITHM
	SOMarkBaseURL                     string `mapstructure:"somark_base_url"`                   // SOMARK_BASE_URL
	SOMarkAPIKey                      string `mapstructure:"somark_api_key"`                    // SOMARK_API_KEY
	SOMarkImageFormat                 string `mapstructure:"somark_image_format"`               // SOMARK_IMAGE_FORMAT
	SOMarkFormulaFormat               string `mapstructure:"somark_formula_format"`             // SOMARK_FORMULA_FORMAT
	SOMarkTableFormat                 string `mapstructure:"somark_table_format"`               // SOMARK_TABLE_FORMAT
	SOMarkCSFormat                    string `mapstructure:"somark_cs_format"`                  // SOMARK_CS_FORMAT
	SOMarkEnableTextCrossPage         bool   `mapstructure:"somark_enable_text_cross_page"`     // SOMARK_ENABLE_TEXT_CROSS_PAGE
	SOMarkEnableTableCrossPage        bool   `mapstructure:"somark_enable_table_cross_page"`    // SOMARK_ENABLE_TABLE_CROSS_PAGE
	SOMarkEnableTitleLevelRecognition bool   `mapstructure:"somark_title_level_recognition"`    // SOMARK_TITLE_LEVEL_RECOGNITION
	SOMarkEnableInlineImage           bool   `mapstructure:"somark_enable_inline_image"`        // SOMARK_ENABLE_INLINE_IMAGE
	SOMarkEnableTableImage            bool   `mapstructure:"somark_enable_table_image"`         // SOMARK_ENABLE_TABLE_IMAGE
	SOMarkEnableImageUnderstanding    bool   `mapstructure:"somark_enable_image_understanding"` // SOMARK_ENABLE_IMAGE_UNDERSTANDING
	SOMarkKeepHeaderFooter            bool   `mapstructure:"somark_keep_header_footer"`         // SOMARK_KEEP_HEADER_FOOTER
	TCADPAPIServerURL                 string `mapstructure:"tcad_api_server"`                   // TCAD_API_SERVER
	TCADPAPIKey                       string `mapstructure:"tcad_api_key"`                      // TCAD_API_KEY
	TEIModel                          string `mapstructure:"tei_model"`                         // TEI_MODEL
	TEIComposeProfiles                string `mapstructure:"tei_compose_profiles"`              // EnvComposeProfiles
	TEIBaseURL                        string `mapstructure:"tei_base_url"`                      // TEI_BASE_URL
	DoclingServerURL                  string `mapstructure:"docling_server_url"`                // DOCLING_SERVER_URI
	DoclingAPIKey                     string `mapstructure:"docling_api_key"`                   // DOCLING_API_KEY
	MinerUAPIServer                   string `mapstructure:"miner_userver"`                     // MINERU_APISERVER
	MinerUAPIKey                      string `mapstructure:"mineru_api_key"`                    // MINERU_API_KEY
	MinerUBackend                     string `mapstructure:"mineru_backend"`                    // MINERU_BACKEND
	MonkeyOCRAPIServer                string `mapstructure:"monkeyocr_apiserver"`               // MONKEYOCR_APISERVER
	MonkeyOCROutputDir                string `mapstructure:"monkeyocr_output_dir"`              // MONKEYOCR_OUTPUT_DIR
	MonkeyOCRServerURL                string `mapstructure:"monkeyocr_server_url"`              // MONKEYOCR_SERVER_URL
	MonkeyOCRDeleteOutput             string `mapstructure:"monkeyocr_delete_output"`           // MONKEYOCR_DELETE_OUTPUT
	TavilyAPIKey                      string `mapstructure:"tavily_api_key"`
	QueritAPIKey                      string `mapstructure:"querit_api_key"`
	KeenableAPIURL                    string `mapstructure:"keenable_api_url"` // KEENABLE_API_URL
}

type DefaultSuperUser struct {
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
	Nickname string `mapstructure:"nickname"`
}

func (c *Config) GetEnvironments() error {

	// Language
	c.environments.Language = GetLanguage()

	// Secret key
	if envVal := common.GetEnv(common.EnvRAGFlowSecretKey); envVal != "" {
		c.environments.SecretKey = envVal
	}

	if envVal := common.GetEnv(common.EnvEnableRegister); envVal != "" {
		if c.environments.EnableRegister == nil {
			c.environments.EnableRegister = new(bool)
		}
		str := envVal
		if str == "true" || str == "1" || str == "yes" {
			*c.environments.EnableRegister = true
		} else {
			*c.environments.EnableRegister = false
		}
	}

	if envVal := common.GetEnv(common.EnvDisablePasswordLogin); envVal != "" {
		if c.environments.DisablePasswordLogin == nil {
			c.environments.DisablePasswordLogin = new(bool)
		}
		str := envVal
		if str == "true" || str == "1" || str == "yes" {
			*c.environments.DisablePasswordLogin = true
		} else {
			*c.environments.DisablePasswordLogin = false
		}
	}

	// Doc engine
	docEngine := common.GetEnvSmall(common.EnvDocEngine)
	if docEngine != "" {
		switch docEngine {
		case "infinity", "elasticsearch", "oceanbase", "seekdb":
			c.environments.DocumentEngineType = docEngine
		case "opensearch":
			return fmt.Errorf("not implemented: %s", docEngine)
		default:
			return fmt.Errorf("invalid doc engine: %s", docEngine)
		}
	}

	// Meta database
	databaseType := common.GetEnvSmall(common.EnvDBType)
	if databaseType != "" {
		switch databaseType {
		case "mysql", "oceanbase":
			c.environments.DatabaseType = databaseType
		default:
			return fmt.Errorf("invalid database type: %s", databaseType)
		}
	}

	// Storage
	storageType := common.GetEnvSmall(common.EnvStorageImpl)
	if storageType != "" {
		switch storageType {
		case "minio", "s3", "oss", "gcs":
			c.environments.StorageType = storageType
		default:
			return fmt.Errorf("invalid storage type: %s", storageType)
		}
	}

	// Minio
	minioHost := common.GetEnv(common.EnvMinioHost)
	if minioHost != "" {
		c.environments.MinioHost = minioHost
	}

	minioRegion := common.GetEnv(common.EnvMinioRegion)
	if minioRegion != "" {
		c.environments.MinioRegion = minioRegion
	}

	// Default super user email
	superUserEmail := common.GetEnv(common.EnvDefaultSuperuserEmail)
	if superUserEmail != "" {
		_, err := mail.ParseAddress(superUserEmail)
		if err != nil {
			return fmt.Errorf("invalid super user email: %s", superUserEmail)
		}
		c.environments.DefaultSuperUser.Email = superUserEmail
	}

	superUserPassword := common.GetEnv(common.EnvDefaultSuperuserPassword)
	if superUserPassword != "" {
		c.environments.DefaultSuperUser.Password = superUserPassword
	}

	superUserNickname := common.GetEnv(common.EnvDefaultSuperuserNickname)
	if superUserNickname != "" {
		c.environments.DefaultSuperUser.Nickname = superUserNickname
	}

	deviceType := common.GetEnv(common.EnvDevice)
	if deviceType != "" {
		c.environments.DeviceType = deviceType
	}

	infinityURL := common.GetEnv(common.EnvInfinityURI)
	if infinityURL != "" {
		c.environments.InfinityURL = infinityURL
	}

	ragflowConfPath := common.GetEnv(common.EnvRAGFlowConfDir)
	if ragflowConfPath != "" {
		c.environments.RAGFLowConfPath = ragflowConfPath
	}

	ragflowProjectBase := common.GetEnv(common.EnvRAGProjectBase)
	if ragflowProjectBase != "" {
		c.environments.RAGFlowProjectBase = ragflowProjectBase
	}

	ragflowDeployBase := common.GetEnv(common.EnvRAGDeployBase)
	if ragflowDeployBase != "" {
		c.environments.RAGFlowDeployBase = ragflowDeployBase
	}

	spacyModelPATH := common.GetEnv(common.EnvSpacyModelDir)
	if spacyModelPATH != "" {
		c.environments.SPACYModelPath = spacyModelPATH
	}

	dictPath := common.GetEnv(common.EnvRAGFlowDictPath)
	if dictPath != "" {
		c.environments.DictionaryPath = dictPath
	}

	ttsCacheTTLSecondsStr := common.GetEnv(common.EnvRAGFlowTTSCacheTTLSeconds)
	if ttsCacheTTLSecondsStr == "" {
		c.environments.TTSCacheTTLSeconds = 7 * 24 * time.Hour
	} else {
		n, err := strconv.Atoi(ttsCacheTTLSecondsStr)
		if err != nil || n <= 0 {
			c.environments.TTSCacheTTLSeconds = 7 * 24 * time.Hour
		} else {
			c.environments.TTSCacheTTLSeconds = time.Duration(n) * time.Second
		}
	}

	maxFileNumberPerUserStr := common.GetEnv(common.EnvMaxFileNumPerUser)
	if maxFileNumberPerUserStr != "" {
		maxFileNumberPerUser, err := strconv.Atoi(maxFileNumberPerUserStr)
		if err != nil {
			return err
		}
		c.environments.MAXFileNumberPerUser = maxFileNumberPerUser
	}

	maxContentLengthStr := common.GetEnv(common.EnvMaxContentLength)
	if maxContentLengthStr != "" {
		maxContentLength, err := strconv.Atoi(maxContentLengthStr)
		if err != nil {
			return err
		}
		c.environments.MAXContentLength = maxContentLength
	}

	homePath := common.GetEnv(common.EnvHome)
	if homePath != "" {
		c.environments.HomePath = homePath
	}

	userProfile := common.GetEnv(common.EnvUserProfile)
	if userProfile != "" {
		c.environments.UserProfile = userProfile
	}

	httpProxy := common.GetEnv(common.EnvHTTPProxy)
	if httpProxy != "" {
		c.environments.HTTPProxy = httpProxy
	}

	httpsProxy := common.GetEnv(common.EnvHTTPSProxy)
	if httpsProxy != "" {
		c.environments.HTTPSProxy = httpsProxy
	}

	logAPITimingStr := common.GetEnv(common.EnvRAGFlowAPITiming)
	if logAPITimingStr != "" {
		logAPITiming := strings.ToLower(logAPITimingStr) == "true"
		c.environments.LOGAPITiming = logAPITiming
	}

	chunkFeedbackEnabledStr := common.GetEnv(common.EnvChunkFeedbackEnabled)
	if chunkFeedbackEnabledStr != "" {
		chunkFeedbackEnabled := strings.ToLower(chunkFeedbackEnabledStr) == "true"
		c.environments.ChunkFeedbackEnabled = chunkFeedbackEnabled
	}

	chunkFeedbackWeightingStr := common.GetEnv(common.EnvChunkFeedbackWeighting)
	if chunkFeedbackWeightingStr != "" {
		c.environments.ChunkFeedbackWeighting = chunkFeedbackWeightingStr
	}

	boxWebOauthRedirectURLStr := common.GetEnv(common.EnvBoxWebOAuthRedirectURI)
	if boxWebOauthRedirectURLStr != "" {
		c.environments.BoxWebOauthRedirectURL = boxWebOauthRedirectURLStr
	}

	gmailWebOAuthRedirectURLStr := common.GetEnv(common.EnvGmailWebOAuthRedirectURI)
	if gmailWebOAuthRedirectURLStr != "" {
		c.environments.GmailWebOAuthRedirectURL = gmailWebOAuthRedirectURLStr
	}

	GoogleDriveWebOAuthRedirectURLStr := common.GetEnv(common.EnvGoogleDriveWebOAuthRedirectURI)
	if GoogleDriveWebOAuthRedirectURLStr != "" {
		c.environments.GoogleDriveWebOAuthRedirectURL = GoogleDriveWebOAuthRedirectURLStr
	}

	deepDocURLStr := common.GetEnv(common.EnvDeepDocURL)
	if deepDocURLStr != "" {
		c.environments.DeepDocURL = deepDocURLStr
	}

	tensorRTDLAServerStr := common.GetEnv(common.EnvTensorrtDLAServer)
	if tensorRTDLAServerStr != "" {
		c.environments.TensorRTDLAServer = tensorRTDLAServerStr
	}

	OSSDeepDocURLStr := common.GetEnv(common.EnvOSSDeepDocURL)
	if OSSDeepDocURLStr != "" {
		c.environments.OSSDeepDocURL = OSSDeepDocURLStr
	}

	DeepDocBatchSingleStr := common.GetEnv(common.EnvBatchSingle)
	if DeepDocBatchSingleStr != "" {
		c.environments.DeepDocBatchSingle = DeepDocBatchSingleStr
	}

	DeepDocBatchCountStr := common.GetEnv(common.EnvBatchCount)
	if DeepDocBatchCountStr != "" {
		DeepDocBatchCount, err := strconv.Atoi(DeepDocBatchCountStr)
		if err != nil {
			return err
		}
		c.environments.DeepDocBatchCount = DeepDocBatchCount
	}

	DeepDocBatchLogLevelStr := common.GetEnv(common.EnvBatchLogLevel)
	if DeepDocBatchLogLevelStr != "" {
		c.environments.DeepDocBatchLogLevel = DeepDocBatchLogLevelStr
	}

	DeepDocBatchCompareOnlyStr := common.GetEnv(common.EnvBatchCompareOnly)
	if DeepDocBatchCompareOnlyStr != "" {
		DeepDocBatchCompareOnly := strings.ToLower(DeepDocBatchCompareOnlyStr) == "true"
		c.environments.DeepDocCompareOnly = DeepDocBatchCompareOnly
	}

	DeepDocBatchCompareFilterStr := common.GetEnv(common.EnvBatchCompareFilter)
	if DeepDocBatchCompareFilterStr != "" {
		c.environments.DeepDocCompareFilter = DeepDocBatchCompareFilterStr
	}

	DeepDocBatchCompareCSVStr := common.GetEnv(common.EnvBatchCompareCSV)
	if DeepDocBatchCompareCSVStr != "" {
		c.environments.DeepDocCompareCSV = DeepDocBatchCompareCSVStr
	}

	DeepDocBatchCSVStr := common.GetEnv(common.EnvBatchCSV)
	if DeepDocBatchCSVStr != "" {
		c.environments.DeepDocBatchCSV = DeepDocBatchCSVStr
	}

	OpenDataLoaderAPIServerStr := common.GetEnv(common.EnvOpenDataLoaderAPIServer)
	if OpenDataLoaderAPIServerStr != "" {
		c.environments.OpenDataLoaderAPIServer = OpenDataLoaderAPIServerStr
	}

	OpenDataLoaderAPIKeyStr := common.GetEnv(common.EnvOpenDataLoaderAPIKey)
	if OpenDataLoaderAPIKeyStr != "" {
		c.environments.OpenDataLoaderAPIKey = OpenDataLoaderAPIKeyStr
	}

	PaddleOCRBaseURLStr := common.GetEnv(common.EnvPaddleOCRBaseUrl)
	if PaddleOCRBaseURLStr != "" {
		c.environments.PaddleOCRBaseURL = PaddleOCRBaseURLStr
	}

	PaddleOCRAPIURLStr := common.GetEnv(common.EnvPaddleOCRAPIURL)
	if PaddleOCRAPIURLStr != "" {
		c.environments.PaddleOCRAPIURL = PaddleOCRAPIURLStr
	}

	PaddleOCRAccessTokenStr := common.GetEnv(common.EnvPaddleOCRAccessToken)
	if PaddleOCRAccessTokenStr != "" {
		c.environments.PaddleOCRAccessToken = PaddleOCRAccessTokenStr
	}

	PaddleOCRAlgorithmStr := common.GetEnv(common.EnvPaddleOCRAlgorithm)
	if PaddleOCRAlgorithmStr != "" {
		c.environments.PaddleOCRAlgorithm = PaddleOCRAlgorithmStr
	}

	SOMarkBaseURLStr := common.GetEnv(common.EnvSOMarkBaseUrl)
	if SOMarkBaseURLStr != "" {
		c.environments.SOMarkBaseURL = SOMarkBaseURLStr
	}

	SOMarkAPIKeyStr := common.GetEnv(common.EnvSOMarkAPIKey)
	if SOMarkAPIKeyStr != "" {
		c.environments.SOMarkAPIKey = SOMarkAPIKeyStr
	}

	SOMarkImageFormatStr := common.GetEnv(common.EnvSOMarkImageFormat)
	if SOMarkImageFormatStr != "" {
		c.environments.SOMarkImageFormat = SOMarkImageFormatStr
	}

	SOMarkFormulaFormatStr := common.GetEnv(common.EnvSOMarkFormulaFormat)
	if SOMarkFormulaFormatStr != "" {
		c.environments.SOMarkFormulaFormat = SOMarkFormulaFormatStr
	}

	SOMarkTableFormatStr := common.GetEnv(common.EnvSOMarkTableFormat)
	if SOMarkTableFormatStr != "" {
		c.environments.SOMarkTableFormat = SOMarkTableFormatStr
	}

	SOMarkCSFormatStr := common.GetEnv(common.EnvSOMarkCSFormat)
	if SOMarkCSFormatStr != "" {
		c.environments.SOMarkCSFormat = SOMarkCSFormatStr
	}

	SOMarkEnableTextCrossPageStr := common.GetEnv(common.EnvSOMarkEnableTextCrossPage)
	if SOMarkEnableTextCrossPageStr != "" {
		c.environments.SOMarkEnableTextCrossPage = strings.ToLower(SOMarkEnableTextCrossPageStr) == "true"
	}

	SOMarkEnableTableCrossPageStr := common.GetEnv(common.EnvSOMarkEnableTableCrossPage)
	if SOMarkEnableTableCrossPageStr != "" {
		c.environments.SOMarkEnableTableCrossPage = strings.ToLower(SOMarkEnableTableCrossPageStr) == "true"
	}

	SOMarkTitleLevelRecognitionStr := common.GetEnv(common.EnvSOMarkEnableTitleLevelRecognition)
	if SOMarkTitleLevelRecognitionStr != "" {
		c.environments.SOMarkEnableTitleLevelRecognition = strings.ToLower(SOMarkTitleLevelRecognitionStr) == "true"
	}

	SOMarkEnableInlineImageStr := common.GetEnv(common.EnvSOMarkEnableInlineImage)
	if SOMarkEnableInlineImageStr != "" {
		c.environments.SOMarkEnableInlineImage = strings.ToLower(SOMarkEnableInlineImageStr) == "true"
	}

	SOMarkEnableTableImageStr := common.GetEnv(common.EnvSOMarkEnableTableImage)
	if SOMarkEnableTableImageStr != "" {
		c.environments.SOMarkEnableTableImage = strings.ToLower(SOMarkEnableTableImageStr) == "true"
	}

	SOMarkEnableImageUnderstandingStr := common.GetEnv(common.EnvSOMarkEnableImageUnderstanding)
	if SOMarkEnableImageUnderstandingStr != "" {
		c.environments.SOMarkEnableImageUnderstanding = strings.ToLower(SOMarkEnableImageUnderstandingStr) == "true"
	}

	SOMarkKeepHeaderFooterStr := common.GetEnv(common.EnvSOMarkKeepHeaderFooter)
	if SOMarkKeepHeaderFooterStr != "" {
		c.environments.SOMarkKeepHeaderFooter = strings.ToLower(SOMarkKeepHeaderFooterStr) == "true"
	}

	TCADPAPIServerURLStr := common.GetEnv(common.EnvTCADPAPIServerURL)
	if TCADPAPIServerURLStr != "" {
		c.environments.TCADPAPIServerURL = TCADPAPIServerURLStr
	}

	TCADPAPIKeyStr := common.GetEnv(common.EnvTCADPAPIKey)
	if TCADPAPIKeyStr != "" {
		c.environments.TCADPAPIKey = TCADPAPIKeyStr
	}

	TEIModelStr := common.GetEnv(common.EnvTEIModel)
	if TEIModelStr != "" {
		c.environments.TEIModel = TEIModelStr
	}

	TEIComposeProfilesStr := common.GetEnv(common.EnvComposeProfiles)
	if TEIComposeProfilesStr != "" {
		c.environments.TEIComposeProfiles = TEIComposeProfilesStr
	}

	TEIBaseURLStr := common.GetEnv(common.EnvTEIBaseURL)
	if TEIBaseURLStr != "" {
		c.environments.TEIBaseURL = TEIBaseURLStr
	}

	DoclingServerURLStr := common.GetEnv(common.EnvDoclingServerURL)
	if DoclingServerURLStr != "" {
		c.environments.DoclingServerURL = DoclingServerURLStr
	}

	DoclingAPIKeyStr := common.GetEnv(common.EnvDoclingAPIKey)
	if DoclingAPIKeyStr != "" {
		c.environments.DoclingAPIKey = DoclingAPIKeyStr
	}

	MinerUAPIServerStr := common.GetEnv(common.EnvMineruAPIServer)
	if MinerUAPIServerStr != "" {
		c.environments.MinerUAPIServer = MinerUAPIServerStr
	}

	MinerUAPIKeyStr := common.GetEnv(common.EnvMineruAPIKey)
	if MinerUAPIKeyStr != "" {
		c.environments.MinerUAPIKey = MinerUAPIKeyStr
	}

	MinerUBackendStr := common.GetEnv(common.EnvMineruBackend)
	if MinerUBackendStr != "" {
		c.environments.MinerUBackend = MinerUBackendStr
	}

	MonkeyOCRAPIServerStr := common.GetEnv(common.EnvMonkeyOCRAPIServer)
	if MonkeyOCRAPIServerStr != "" {
		c.environments.MonkeyOCRAPIServer = MonkeyOCRAPIServerStr
	}

	MonkeyOCROutputDirStr := common.GetEnv(common.EnvMonkeyOCROutputDir)
	if MonkeyOCROutputDirStr != "" {
		c.environments.MonkeyOCROutputDir = MonkeyOCROutputDirStr
	}

	MonkeyOCRServerURLStr := common.GetEnv(common.EnvMonkeyOCRServerURL)
	if MonkeyOCRServerURLStr != "" {
		c.environments.MonkeyOCRServerURL = MonkeyOCRServerURLStr
	}

	MonkeyOCRDeleteOutputStr := common.GetEnv(common.EnvMonkeyOCRDeleteOutput)
	if MonkeyOCRDeleteOutputStr != "" {
		c.environments.MonkeyOCRDeleteOutput = MonkeyOCRDeleteOutputStr
	}

	TavilyAPIKeyStr := common.GetEnv(common.EnvTavilyAPIKey)
	if TavilyAPIKeyStr != "" {
		c.environments.TavilyAPIKey = TavilyAPIKeyStr
	}

	QueritAPIKeyStr := common.GetEnv(common.EnvQueritAPIKey)
	if QueritAPIKeyStr != "" {
		c.environments.QueritAPIKey = QueritAPIKeyStr
	}

	KeenableAPIURLStr := common.GetEnv(common.EnvKeenableAPIURL)
	if KeenableAPIURLStr != "" {
		c.environments.KeenableAPIURL = KeenableAPIURLStr
	}

	return nil
}

func GetLanguage() string {
	lang := common.GetEnv(common.EnvLang)
	if lang == "" {
		lang = common.GetEnv(common.EnvLanguage)
	}

	if strings.Contains(lang, "zh_") ||
		strings.Contains(lang, "zh-") ||
		strings.HasPrefix(lang, "zh") {
		return "Chinese"
	}

	return "English"
}

func (c *Config) GetSecretKey() string {
	return c.environments.SecretKey
}

func (c *Config) GetEnvLanguage() string {
	return c.environments.Language
}

func (c *Config) GetEnvEnableRegister() bool {
	return *c.environments.EnableRegister
}

func (c *Config) GetEnvDisablePasswordLogin() bool {
	return *c.environments.DisablePasswordLogin
}

func (c *Config) GetEnvDocumentEngineType() string {
	return c.environments.DocumentEngineType
}

func (c *Config) GetEnvDatabaseType() string {
	return c.environments.DatabaseType
}

func (c *Config) GetEnvStorageType() string {
	return c.environments.StorageType
}

func (c *Config) GetEnvMinioHost() string {
	return c.environments.MinioHost
}

func (c *Config) GetEnvMinioRegion() string {
	return c.environments.MinioRegion
}

func (c *Config) GetEnvDefaultSuperUserEmail() string {
	return c.environments.DefaultSuperUser.Email
}

func (c *Config) GetEnvDefaultSuperUserNick() string {
	return c.environments.DefaultSuperUser.Email
}

func (c *Config) GetEnvDictionaryPath() string {
	return c.environments.DictionaryPath
}

func (c *Config) GetEnvDeviceType() string {
	return c.environments.DeviceType
}

func (c *Config) GetInfinityURL() string {
	return c.environments.InfinityURL
}

func (c *Config) GetConfigPath() string {
	return c.environments.RAGFLowConfPath
}

func (c *Config) GetProjectBase() string {
	return c.environments.RAGFlowProjectBase
}

func (c *Config) GetDeployBase() string {
	return c.environments.RAGFlowDeployBase
}

func (c *Config) GetSPACYModelPath() string {
	return c.environments.SPACYModelPath
}

func (c *Config) GetDictionaryPath() string { return c.environments.DictionaryPath }

func (c *Config) GetTTSCacheTTLSeconds() time.Duration { return c.environments.TTSCacheTTLSeconds }

func (c *Config) GetMaxFileNumPerUser() int { return c.environments.MAXFileNumberPerUser }

func (c *Config) GetMaxContentLength() int { return c.environments.MAXContentLength }

func (c *Config) GetHomePath() string { return c.environments.HomePath }

func (c *Config) GetUserProfile() string { return c.environments.UserProfile }

func (c *Config) GetHTTPProxy() string { return c.environments.HTTPProxy }

func (c *Config) GetHTTPSProxy() string { return c.environments.HTTPSProxy }

func (c *Config) LogAPITiming() bool { return c.environments.LOGAPITiming }

func (c *Config) ChunkFeedbackEnabled() bool { return c.environments.ChunkFeedbackEnabled }

func (c *Config) GetChunkFeedbackWeighting() string { return c.environments.ChunkFeedbackWeighting }

func (c *Config) GetBoxWebOauthRedirectURL() string { return c.environments.BoxWebOauthRedirectURL }

func (c *Config) GetGmailWebOAuthRedirectURL() string { return c.environments.GmailWebOAuthRedirectURL }

func (c *Config) GetGoogleDriveWebOAuthRedirectURL() string {
	return c.environments.GoogleDriveWebOAuthRedirectURL
}

func (c *Config) GetDeepDocURL() string {
	return c.environments.DeepDocURL
}

func (c *Config) GetTensorRTDLAServer() string {
	return c.environments.TensorRTDLAServer
}

func (c *Config) GetOSSDeepDocURL() string {
	return c.environments.OSSDeepDocURL
}

func (c *Config) GetDeepDocBatchSingle() string {
	return c.environments.DeepDocBatchSingle
}

func (c *Config) GetDeepDocBatchCount() int {
	return c.environments.DeepDocBatchCount
}

func (c *Config) GetDeepDocBatchLogLevel() string {
	return c.environments.DeepDocBatchLogLevel
}

func (c *Config) GetDeepDocCompareOnly() bool {
	return c.environments.DeepDocCompareOnly
}

func (c *Config) GetDeepDocCompareFilter() string {
	return c.environments.DeepDocCompareFilter
}

func (c *Config) GetDeepDocCompareCSV() string {
	return c.environments.DeepDocCompareCSV
}

func (c *Config) GetDeepDocBatchCSV() string {
	return c.environments.DeepDocBatchCSV
}

func (c *Config) GetOpenDataLoaderAPIServer() string {
	return c.environments.OpenDataLoaderAPIServer
}

func (c *Config) GetOpenDataLoaderAPIKey() string {
	return c.environments.OpenDataLoaderAPIKey
}

func (c *Config) GetPaddleOCRBaseURL() string {
	return c.environments.PaddleOCRBaseURL
}

func (c *Config) GetPaddleOCRAPIURL() string {
	return c.environments.PaddleOCRAPIURL
}

func (c *Config) GetPaddleOCRAccessToken() string {
	return c.environments.PaddleOCRAccessToken
}

func (c *Config) GetPaddleOCRAlgorithm() string {
	return c.environments.PaddleOCRAlgorithm
}

func (c *Config) GetSOMarkBaseURL() string {
	return c.environments.SOMarkBaseURL
}

func (c *Config) GetSOMarkAPIKey() string {
	return c.environments.SOMarkAPIKey
}

func (c *Config) GetSOMarkImageFormat() string {
	return c.environments.SOMarkImageFormat
}

func (c *Config) GetSOMarkFormulaFormat() string {
	return c.environments.SOMarkFormulaFormat
}

func (c *Config) GetSOMarkTableFormat() string {
	return c.environments.SOMarkTableFormat
}

func (c *Config) GetSOMarkCSFormatStr() string {
	return c.environments.SOMarkCSFormat
}

func (c *Config) GetSOMarkEnableTextCrossPage() bool {
	return c.environments.SOMarkEnableTextCrossPage
}

func (c *Config) GetSOMarkEnableTableCrossPage() bool {
	return c.environments.SOMarkEnableTableCrossPage
}

func (c *Config) GetSOMarkEnableInlineImage() bool {
	return c.environments.SOMarkEnableInlineImage
}

func (c *Config) GetSOMarkEnableTableImage() bool {
	return c.environments.SOMarkEnableTableImage
}

func (c *Config) GetSOMarkEnableImageUnderstanding() bool {
	return c.environments.SOMarkEnableImageUnderstanding
}

func (c *Config) GetSOMarkKeepHeaderFooter() bool {
	return c.environments.SOMarkKeepHeaderFooter
}

func (c *Config) GetTCADPAPIServerURL() string {
	return c.environments.TCADPAPIServerURL
}

func (c *Config) GetTCADPAPIKey() string {
	return c.environments.TCADPAPIKey
}

func (c *Config) GetTEIModel() string {
	return c.environments.TEIModel
}

func (c *Config) GetTEIComposeProfiles() string {
	return c.environments.TEIComposeProfiles
}

func (c *Config) GetTEIBaseURL() string {
	return c.environments.TEIBaseURL
}

func (c *Config) GetDoclingServerURL() string {
	return c.environments.DoclingServerURL
}

func (c *Config) GetDoclingAPIKey() string {
	return c.environments.DoclingAPIKey
}

func (c *Config) GetMinerUAPIServer() string {
	return c.environments.MinerUAPIServer
}

func (c *Config) GetMinerUAPIKey() string {
	return c.environments.MinerUAPIKey
}

func (c *Config) GetMinerUBackend() string {
	return c.environments.MinerUBackend
}

func (c *Config) GetMonkeyOCRAPIServer() string {
	return c.environments.MonkeyOCRAPIServer
}

func (c *Config) GetMonkeyOCROutputDir() string {
	return c.environments.MonkeyOCROutputDir
}

func (c *Config) GetMonkeyOCRServerURL() string {
	return c.environments.MonkeyOCRServerURL
}

func (c *Config) GetMonkeyOCRDeleteOutput() string {
	return c.environments.MonkeyOCRDeleteOutput
}

func (c *Config) GetQueritAPIKey() string {
	return c.environments.QueritAPIKey
}

func (c *Config) GetKeenableAPIURL() string {
	return c.environments.KeenableAPIURL
}
