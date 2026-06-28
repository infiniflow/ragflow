import { FormFieldType } from '@/components/dynamic-form';
import { LLMFactory } from '@/constants/llm';
import type { ProviderConfig } from '../types';
import { buildModelInfoFromValues } from './utils';

/**
 * Factory configuration mapping table
 * key: LLMFactory value
 * value: ProviderConfig
 */
export const ProviderConfigMap: Record<string, ProviderConfig> = {
  // ============ Azure OpenAI ============
  [LLMFactory.AzureOpenAI]: {
    llmFactory: LLMFactory.AzureOpenAI,
    title: 'Azure OpenAI',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [
          { label: 'Chat', value: 'chat' },
          { label: 'Embedding', value: 'embedding' },
          { label: 'Image2Text', value: 'image2text' },
        ],
        defaultValue: ['embedding'],
      },
      {
        name: 'api_base',
        label: 'addLlmBaseUrl',
        type: 'inputSelect',
        required: true,
        placeholder: 'baseUrlNameMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'baseUrlNameMessage' },
      },
      {
        name: 'api_key',
        label: 'apiKey',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'apiKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'apiKeyMessage' },
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'modelNameMessage',
        defaultValue: 'gpt-3.5-turbo',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'api_version',
        label: 'apiVersion',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'apiVersionMessage',
        defaultValue: '2024-02-01',
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0, message: 'maxTokensMessage' },
      },
      {
        name: 'vision',
        label: 'vision',
        type: FormFieldType.Switch,
        defaultValue: false,
        shouldRender: 'modelTypeIncludesChat',
      },
    ],
    verifyTransform: (values) => ({
      apiKey: values.api_key,
      baseUrl: values.api_base,
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.AzureOpenAI,
      api_base: values.api_base,
      api_key: values.api_key,
      api_version: values.api_version,
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ VolcEngine ============
  [LLMFactory.VolcEngine]: {
    llmFactory: LLMFactory.VolcEngine,
    title: 'VolcEngine',
    docLink: 'https://www.volcengine.com/docs/82379/1302008',
    docLinkI18nKey: 'ollamaLink',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [
          { label: 'Chat', value: 'chat' },
          { label: 'Embedding', value: 'embedding' },
          { label: 'Image2Text', value: 'image2text' },
        ],
        defaultValue: ['chat'],
      },
      // {
      //   name: 'model_name',
      //   label: 'modelName',
      //   type: 'text',
      //   required: true,
      //   placeholder: 'volcModelNameMessage',
      //   validation: { message: 'volcModelNameMessage' },
      // },
      {
        name: 'model_name',
        label: 'addEndpointID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'endpointIDMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'endpointIDMessage' },
      },
      {
        name: 'api_key',
        label: 'addArkApiKey',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'ArkApiKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'ArkApiKeyMessage' },
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0 },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: values.api_key,
      endpoint_id: values.endpoint_id,
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.VolcEngine,
      endpoint_id: values.endpoint_id,
      api_key: values.api_key,
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ Google Cloud ============
  [LLMFactory.GoogleCloud]: {
    llmFactory: LLMFactory.GoogleCloud,
    title: 'Google Cloud',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [
          { label: 'Chat', value: 'chat' },
          { label: 'Image2Text', value: 'image2text' },
        ],
        defaultValue: ['chat'],
      },
      {
        name: 'model_name',
        label: 'modelID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'GoogleModelIDMessage',
        validation: { message: 'GoogleModelIDMessage' },
      },
      {
        name: 'google_project_id',
        label: 'addGoogleProjectID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'GoogleProjectIDMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'GoogleProjectIDMessage' },
      },
      {
        name: 'google_region',
        label: 'addGoogleRegion',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'GoogleRegionMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'GoogleRegionMessage' },
      },
      {
        name: 'google_service_account_key',
        label: 'addGoogleServiceAccountKey',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'GoogleServiceAccountKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'GoogleServiceAccountKeyMessage' },
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0, message: 'maxTokensMinMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        google_project_id: values.google_project_id,
        google_region: values.google_region,
        google_service_account_key: values.google_service_account_key,
      },
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.GoogleCloud,
      google_project_id: values.google_project_id,
      google_region: values.google_region,
      google_service_account_key: values.google_service_account_key,
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ Tencent Cloud ============
  [LLMFactory.TencentCloud]: {
    llmFactory: LLMFactory.TencentCloud,
    title: 'Tencent Cloud',
    docLink: 'https://cloud.tencent.com/document/api/1093/37823',
    docLinkI18nKey: 'TencentCloudLink',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [{ label: 'Speech2Text', value: 'speech2text' }],
        defaultValue: ['speech2text'],
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Select,
        required: true,
        options: [
          { label: '16k_zh', value: '16k_zh' },
          { label: '16k_zh_large', value: '16k_zh_large' },
          { label: '16k_multi_lang', value: '16k_multi_lang' },
          { label: '16k_zh_dialect', value: '16k_zh_dialect' },
          { label: '16k_en', value: '16k_en' },
          { label: '16k_yue', value: '16k_yue' },
          { label: '16k_zh-PY', value: '16k_zh-PY' },
          { label: '16k_ja', value: '16k_ja' },
          { label: '16k_ko', value: '16k_ko' },
          { label: '16k_vi', value: '16k_vi' },
          { label: '16k_ms', value: '16k_ms' },
          { label: '16k_id', value: '16k_id' },
          { label: '16k_fil', value: '16k_fil' },
          { label: '16k_th', value: '16k_th' },
          { label: '16k_pt', value: '16k_pt' },
          { label: '16k_tr', value: '16k_tr' },
          { label: '16k_ar', value: '16k_ar' },
          { label: '16k_es', value: '16k_es' },
          { label: '16k_hi', value: '16k_hi' },
          { label: '16k_fr', value: '16k_fr' },
          { label: '16k_zh_medical', value: '16k_zh_medical' },
          { label: '16k_de', value: '16k_de' },
        ],
        defaultValue: '16k_zh',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'TencentCloud_sid',
        label: 'addTencentCloudSID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'TencentCloudSIDMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'TencentCloudSIDMessage' },
      },
      {
        name: 'TencentCloud_sk',
        label: 'addTencentCloudSK',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'TencentCloudSKMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'TencentCloudSKMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        TencentCloud_sid: values.TencentCloud_sid,
        TencentCloud_sk: values.TencentCloud_sk,
      },
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.TencentCloud,
      TencentCloud_sid: values.TencentCloud_sid,
      TencentCloud_sk: values.TencentCloud_sk,
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ XunFei Spark ============
  [LLMFactory.XunFeiSpark]: {
    llmFactory: LLMFactory.XunFeiSpark,
    title: 'XunFei Spark',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [
          { label: 'Chat', value: 'chat' },
          { label: 'TTS', value: 'tts' },
        ],
        defaultValue: ['chat'],
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'modelNameMessage',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'spark_api_password',
        label: 'addSparkAPIPassword',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPIPasswordMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'SparkAPIPasswordMessage' },
      },
      {
        name: 'spark_app_id',
        label: 'addSparkAPPID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPPIDMessage',
        shouldRender: 'modelTypeIncludesTtsAndNotExists',
        validation: { message: 'SparkAPPIDMessage' },
      },
      {
        name: 'spark_api_secret',
        label: 'addSparkAPISecret',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPISecretMessage',
        shouldRender: 'modelTypeIncludesTtsAndNotExists',
        validation: { message: 'SparkAPISecretMessage' },
      },
      {
        name: 'spark_api_key',
        label: 'addSparkAPIKey',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPIKeyMessage',
        shouldRender: 'modelTypeIncludesTtsAndNotExists',
        validation: { message: 'SparkAPIKeyMessage' },
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0, message: 'maxTokensInvalidMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        spark_api_password: values.spark_api_password,
        spark_app_id: values.spark_app_id,
        spark_api_secret: values.spark_api_secret,
        spark_api_key: values.spark_api_key,
      },
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.XunFeiSpark,
      api_key: {
        spark_api_password: values.spark_api_password,
        spark_app_id: values.spark_app_id,
        spark_api_secret: values.spark_api_secret,
        spark_api_key: values.spark_api_key,
      },
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ Baidu YiYan ============
  [LLMFactory.BaiduYiYan]: {
    llmFactory: LLMFactory.BaiduYiYan,
    title: 'Baidu YiYan',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [
          { label: 'Chat', value: 'chat' },
          { label: 'Embedding', value: 'embedding' },
          { label: 'Rerank', value: 'rerank' },
        ],
        defaultValue: ['chat'],
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'yiyanModelNameMessage',
        validation: { message: 'yiyanModelNameMessage' },
      },
      {
        name: 'yiyan_ak',
        label: 'addyiyanAK',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'yiyanAKMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'yiyanAKMessage' },
      },
      {
        name: 'yiyan_sk',
        label: 'addyiyanSK',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'yiyanSKMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'yiyanSKMessage' },
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0 },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.BaiduYiYan,
      api_key: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ Fish Audio ============
  [LLMFactory.FishAudio]: {
    llmFactory: LLMFactory.FishAudio,
    title: 'Fish Audio',
    docLink: 'https://fish.audio',
    docLinkI18nKey: 'FishAudioLink',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_type',
        label: 'modelType',
        type: FormFieldType.MultiSelect,
        required: true,
        options: [{ label: 'TTS', value: 'tts' }],
        defaultValue: ['tts'],
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'FishAudioModelNameMessage',
        validation: { message: 'FishAudioModelNameMessage' },
      },
      {
        name: 'fish_audio_ak',
        label: 'addFishAudioAK',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'FishAudioAKMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'FishAudioAKMessage' },
      },
      {
        name: 'fish_audio_refid',
        label: 'addFishAudioRefID',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'FishAudioRefIDMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'FishAudioRefIDMessage' },
      },
      {
        name: 'max_tokens',
        label: 'maxTokens',
        type: FormFieldType.Number,
        required: true,
        placeholder: 'maxTokensTip',
        defaultValue: 8192,
        validation: { min: 0, message: 'maxTokensInvalidMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        fish_audio_ak: values.fish_audio_ak,
        fish_audio_refid: values.fish_audio_refid,
      },
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.FishAudio,
      fish_audio_ak: values.fish_audio_ak,
      fish_audio_refid: values.fish_audio_refid,
      model_info: buildModelInfoFromValues(values),
    }),
  },

  // ============ OpenDataLoader ============
  [LLMFactory.OpenDataLoader]: {
    llmFactory: LLMFactory.OpenDataLoader,
    title: 'OpenDataLoader',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'modelNameMessage',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'opendataloader_apiserver',
        label: 'baseUrl',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'opendataloaderApiserverPlaceholder',
        validation: { message: 'opendataloaderApiserverMessage' },
      },
      {
        name: 'opendataloader_api_key',
        label: 'apiKey',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'apiKeyPlaceholder',
      },
    ],
    verifyTransform: (values) => {
      const cfg: Record<string, any> = {};
      if (values.opendataloader_apiserver) {
        cfg.opendataloader_apiserver = values.opendataloader_apiserver;
      }
      if (values.opendataloader_api_key) {
        cfg.opendataloader_api_key = values.opendataloader_api_key;
      }
      cfg.llm_name = values.model_name;
      return {
        apiKey: cfg,
        baseUrl: values.opendataloader_apiserver,
        modelInfo: [
          {
            model_name: values.model_name,
            model_type: ['ocr'],
            max_tokens: 0,
          },
        ],
      };
    },
    submitTransform: (values) => {
      const cfg: Record<string, any> = {};
      if (values.opendataloader_apiserver) {
        cfg.opendataloader_apiserver = values.opendataloader_apiserver;
      }
      if (values.opendataloader_api_key) {
        cfg.opendataloader_api_key = values.opendataloader_api_key;
      }
      cfg.llm_name = values.model_name;
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.OpenDataLoader,
        api_key: cfg,
        api_base: '',
        model_info: [
          {
            model_name: values.model_name,
            model_type: ['ocr'],
            max_tokens: 0,
          },
        ],
      };
    },
  },

  // ============ PaddleOCR ============
  [LLMFactory.PaddleOCR]: {
    llmFactory: LLMFactory.PaddleOCR,
    title: 'PaddleOCR',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'modelNameMessage',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'paddleocr_api_url',
        label: 'paddleocrApiUrl',
        type: 'inputSelect',
        required: true,
        placeholder: 'paddleocrApiUrlPlaceholder',
        validation: { message: 'paddleocrApiUrlMessage' },
      },
      {
        name: 'paddleocr_access_token',
        label: 'paddleocrAccessToken',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'paddleocrAccessTokenPlaceholder',
        validation: { message: 'paddleocrAccessTokenMessage' },
      },
      {
        name: 'paddleocr_algorithm',
        label: 'paddleocrAlgorithm',
        type: FormFieldType.Select,
        required: false,
        defaultValue: 'PaddleOCR-VL',
        placeholder: 'paddleocrSelectAlgorithm',
        options: [
          { label: 'PaddleOCR-VL-1.6', value: 'PaddleOCR-VL-1.6' },
          { label: 'PaddleOCR-VL-1.5', value: 'PaddleOCR-VL-1.5' },
          { label: 'PaddleOCR-VL', value: 'PaddleOCR-VL' },
          { label: 'PP-OCRv6', value: 'PP-OCRv6' },
          { label: 'PP-OCRv5', value: 'PP-OCRv5' },
          { label: 'PP-StructureV3', value: 'PP-StructureV3' },
        ],
      },
    ],
    verifyTransform: (values) => {
      const cfg: Record<string, any> = {};
      if (values.paddleocr_api_url)
        cfg.paddleocr_api_url = values.paddleocr_api_url;
      if (values.paddleocr_access_token)
        cfg.paddleocr_access_token = values.paddleocr_access_token;
      if (values.paddleocr_algorithm)
        cfg.paddleocr_algorithm = values.paddleocr_algorithm;
      return {
        apiKey: cfg,
        baseUrl: values.paddleocr_api_url,
        modelInfo: buildModelInfoFromValues({
          ...values,
          model_type: ['ocr'],
        }),
      };
    },
    submitTransform: (values) => {
      const cfg: Record<string, any> = {};
      if (values.paddleocr_api_url)
        cfg.paddleocr_api_url = values.paddleocr_api_url;
      if (values.paddleocr_access_token)
        cfg.paddleocr_access_token = values.paddleocr_access_token;
      if (values.paddleocr_algorithm)
        cfg.paddleocr_algorithm = values.paddleocr_algorithm;
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.PaddleOCR,
        api_key: cfg,
        api_base: '',
        model_info: buildModelInfoFromValues({
          ...values,
          model_type: ['ocr'],
        }),
      };
    },
  },

  // ============ MinerU ============
  [LLMFactory.MinerU]: {
    llmFactory: LLMFactory.MinerU,
    title: 'MinerU',
    fields: [
      {
        name: 'instance_name',
        label: 'instanceName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'instanceNameMessage',
        tooltip: 'instanceNameTip',
        validation: { message: 'instanceNameMessage' },
      },
      {
        name: 'model_name',
        label: 'modelName',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'modelNameMessage',
        validation: { message: 'modelNameMessage' },
      },
      {
        name: 'mineru_apiserver',
        label: 'mineruApiserver',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'mineruApiserverPlaceholder',
        validation: { message: 'mineruApiserverMessage' },
      },
      {
        name: 'mineru_output_dir',
        label: 'mineruOutputDir',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'mineruOutputDirPlaceholder',
      },
      {
        name: 'mineru_backend',
        label: 'mineruBackend',
        type: FormFieldType.Select,
        required: true,
        defaultValue: 'pipeline',
        placeholder: 'mineruSelectBackend',
        options: [
          { label: 'pipeline', value: 'pipeline' },
          { label: 'vlm-transformers', value: 'vlm-transformers' },
          { label: 'vlm-vllm-engine', value: 'vlm-vllm-engine' },
          { label: 'vlm-http-client', value: 'vlm-http-client' },
          { label: 'vlm-mlx-engine', value: 'vlm-mlx-engine' },
          { label: 'vlm-vllm-async-engine', value: 'vlm-vllm-async-engine' },
          { label: 'vlm-lmdeploy-engine', value: 'vlm-lmdeploy-engine' },
        ],
        validation: { message: 'mineruBackendMessage' },
      },
      {
        name: 'mineru_server_url',
        label: 'mineruServerUrl',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'mineruServerUrlPlaceholder',
        shouldRender: (values: any) =>
          values?.mineru_backend === 'vlm-http-client',
        validation: { message: 'mineruServerUrlMessage' },
      },
      {
        name: 'mineru_delete_output',
        label: 'mineruDeleteOutput',
        type: FormFieldType.Switch,
        required: false,
        defaultValue: true,
      },
    ],
    verifyTransform: (values) => {
      const cfg: Record<string, any> = { ...values };
      delete cfg.instance_name;
      delete cfg.model_name;
      cfg.mineru_delete_output = values.mineru_delete_output ? '1' : '0';
      if (values.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      return {
        apiKey: cfg,
        baseUrl: values.mineru_apiserver,
        modelInfo: buildModelInfoFromValues({
          ...values,
          model_type: ['ocr'],
        }),
      };
    },
    submitTransform: (values) => {
      const cfg: Record<string, any> = { ...values };
      delete cfg.instance_name;
      delete cfg.model_name;
      cfg.mineru_delete_output = values.mineru_delete_output ? '1' : '0';
      if (values.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.MinerU,
        api_key: cfg,
        api_base: '',
        model_info: buildModelInfoFromValues({
          ...values,
          model_type: ['ocr'],
        }),
      };
    },
  },
};
