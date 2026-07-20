/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { FormFieldType } from '@/components/dynamic-form';
import { LLMFactory } from '@/constants/llm';
import type { ProviderConfig } from '../types';

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
        type: FormFieldType.Password,
        required: false,
        placeholder: 'apiKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'apiKeyMessage' },
      },
      {
        name: 'api_version',
        label: 'apiVersion',
        type: FormFieldType.Text,
        required: false,
        placeholder: 'apiVersionMessage',
        defaultValue: '2024-02-01',
      },
    ],
    verifyTransform: (values) => ({
      apiKey: values.api_key,
      baseUrl: values.api_base,
      modelInfo: [],
    }),
    submitTransform: (values) => {
      const apiKey = values.api_version
        ? { api_key: values.api_key ?? '', api_version: values.api_version }
        : (values.api_key ?? '');
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.AzureOpenAI,
        api_base: values.api_base,
        api_key: apiKey,
        model_info: [],
      };
    },
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
        name: 'api_key',
        label: 'addArkApiKey',
        type: FormFieldType.Password,
        required: true,
        placeholder: 'ArkApiKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'ArkApiKeyMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: values.api_key,
      modelInfo: [],
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.VolcEngine,
      api_key: values.api_key,
      model_info: [],
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
        type: FormFieldType.Password,
        required: true,
        placeholder: 'GoogleServiceAccountKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'GoogleServiceAccountKeyMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        google_project_id: values.google_project_id,
        google_region: values.google_region,
        google_service_account_key: values.google_service_account_key,
      },
      modelInfo: [],
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.GoogleCloud,
      google_project_id: values.google_project_id,
      google_region: values.google_region,
      google_service_account_key: values.google_service_account_key,
      model_info: [],
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
      modelInfo: [],
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.TencentCloud,
      TencentCloud_sid: values.TencentCloud_sid,
      TencentCloud_sk: values.TencentCloud_sk,
      model_info: [],
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
        name: 'spark_api_password',
        label: 'addSparkAPIPassword',
        type: FormFieldType.Password,
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
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'SparkAPPIDMessage' },
      },
      {
        name: 'spark_api_secret',
        label: 'addSparkAPISecret',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPISecretMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'SparkAPISecretMessage' },
      },
      {
        name: 'spark_api_key',
        label: 'addSparkAPIKey',
        type: FormFieldType.Text,
        required: true,
        placeholder: 'SparkAPIKeyMessage',
        shouldRender: 'hideWhenInstanceExists',
        validation: { message: 'SparkAPIKeyMessage' },
      },
    ],
    verifyTransform: (values) => ({
      apiKey: {
        spark_api_password: values.spark_api_password,
        spark_app_id: values.spark_app_id,
        spark_api_secret: values.spark_api_secret,
        spark_api_key: values.spark_api_key,
      },
      modelInfo: [],
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
      model_info: [],
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
    ],
    verifyTransform: (values) => ({
      apiKey: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      modelInfo: [],
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.BaiduYiYan,
      api_key: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      model_info: [],
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
    ],
    verifyTransform: (values) => ({
      apiKey: {
        fish_audio_ak: values.fish_audio_ak,
        fish_audio_refid: values.fish_audio_refid,
      },
      modelInfo: [],
    }),
    submitTransform: (values) => ({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.FishAudio,
      fish_audio_ak: values.fish_audio_ak,
      fish_audio_refid: values.fish_audio_refid,
      model_info: [],
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
        type: FormFieldType.Password,
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
      return {
        apiKey: cfg,
        baseUrl: values.opendataloader_apiserver,
        modelInfo: [],
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
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.OpenDataLoader,
        api_key: cfg,
        api_base: '',
        model_info: [],
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
        type: FormFieldType.Password,
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
        modelInfo: [],
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
        model_info: [],
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
      cfg.mineru_delete_output = values.mineru_delete_output ? '1' : '0';
      if (values.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      return {
        apiKey: cfg,
        baseUrl: values.mineru_apiserver,
        modelInfo: [],
      };
    },
    submitTransform: (values) => {
      const cfg: Record<string, any> = { ...values };
      delete cfg.instance_name;
      cfg.mineru_delete_output = values.mineru_delete_output ? '1' : '0';
      if (values.mineru_backend !== 'vlm-http-client') {
        delete cfg.mineru_server_url;
      }
      return {
        instance_name: values.instance_name,
        llm_factory: LLMFactory.MinerU,
        api_key: cfg,
        api_base: '',
        model_info: [],
      };
    },
  },
};
