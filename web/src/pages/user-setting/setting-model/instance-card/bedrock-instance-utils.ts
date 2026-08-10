/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

import isEqual from 'lodash/isEqual';
import type { IModelInfo } from '@/interfaces/request/llm';
import { parseApiKeyAsObject } from '../provider-schema/field-config/utils';

export type AuthMode =
  | 'access_key_secret'
  | 'iam_role'
  | 'assume_role'
  | 'bedrock_api_key';
export type BedrockEndpointType =
  | 'runtime'
  | 'mantle_openai'
  | 'mantle_anthropic';

export type BedrockFormValues = {
  auth_mode: AuthMode;
  bedrock_ak?: string;
  bedrock_sk?: string;
  aws_role_arn?: string;
  bedrock_api_key?: string;
  bedrock_endpoint_type?: BedrockEndpointType;
  bedrock_endpoint_url?: string;
  bedrock_discovery_endpoint_url?: string;
  bedrock_region: string;
  llm_name: string;
  max_tokens: number;
  model_type: ('chat' | 'embedding')[];
};

export const getBedrockModelListRequest = (values: BedrockFormValues) => {
  const extensions: Record<string, unknown> = {
    auth_mode: values.auth_mode,
  };
  if (values.auth_mode === 'access_key_secret') {
    extensions.bedrock_ak = values.bedrock_ak;
    extensions.bedrock_sk = values.bedrock_sk;
  } else if (values.auth_mode === 'iam_role') {
    extensions.aws_role_arn = values.aws_role_arn;
  } else if (values.auth_mode === 'bedrock_api_key') {
    extensions.endpoint_type = values.bedrock_endpoint_type;
    extensions.endpoint_url = values.bedrock_endpoint_url;
    if (values.bedrock_endpoint_type === 'runtime') {
      extensions.discovery_endpoint_url = values.bedrock_discovery_endpoint_url;
    }
  }
  return {
    api_key:
      values.auth_mode === 'bedrock_api_key'
        ? (values.bedrock_api_key ?? '')
        : '',
    region: values.bedrock_region,
    extensions,
  };
};

export const shouldResetBedrockForm = (
  previousValues: BedrockFormValues,
  nextValues: BedrockFormValues,
) => !isEqual(previousValues, nextValues);

export const getBedrockCatalogResetDecision = (
  persistedScope: string,
  currentScope: string,
  nextScope: string,
) => {
  const invalidateCatalog = persistedScope !== nextScope;
  if (nextScope === currentScope) {
    return {
      pendingReset: null,
      invalidateCatalogImmediately: invalidateCatalog,
    };
  }
  return {
    pendingReset: { scope: nextScope, invalidateCatalog },
    invalidateCatalogImmediately: false,
  };
};

export const getBedrockCatalogCredentialScope = (
  values: Pick<
    BedrockFormValues,
    | 'auth_mode'
    | 'bedrock_endpoint_type'
    | 'bedrock_api_key'
    | 'bedrock_region'
    | 'bedrock_endpoint_url'
    | 'bedrock_discovery_endpoint_url'
    | 'bedrock_ak'
    | 'bedrock_sk'
    | 'aws_role_arn'
  >,
) => {
  const common = [values.auth_mode, values.bedrock_region];
  if (values.auth_mode === 'access_key_secret') {
    return JSON.stringify([...common, values.bedrock_ak, values.bedrock_sk]);
  }
  if (values.auth_mode === 'iam_role') {
    return JSON.stringify([...common, values.aws_role_arn]);
  }
  if (values.auth_mode === 'bedrock_api_key') {
    const scope = [
      ...common,
      values.bedrock_endpoint_type,
      values.bedrock_api_key,
      values.bedrock_endpoint_url,
    ];
    if (values.bedrock_endpoint_type === 'runtime') {
      scope.push(values.bedrock_discovery_endpoint_url);
    }
    return JSON.stringify(scope);
  }
  return JSON.stringify(common);
};

export const getBedrockCatalogCredentialScopeFromPayload = (
  payload: Readonly<Record<string, any>>,
) => {
  const credentials = parseApiKeyAsObject(payload.api_key) ?? {};
  return getBedrockCatalogCredentialScope({
    auth_mode: (credentials.auth_mode as AuthMode) ?? 'access_key_secret',
    bedrock_endpoint_type: credentials.bedrock_endpoint_type,
    bedrock_api_key: credentials.bedrock_api_key,
    bedrock_region: credentials.bedrock_region ?? payload.region ?? '',
    bedrock_endpoint_url: credentials.bedrock_endpoint_url,
    bedrock_discovery_endpoint_url: credentials.bedrock_discovery_endpoint_url,
    bedrock_ak: credentials.bedrock_ak,
    bedrock_sk: credentials.bedrock_sk,
    aws_role_arn: credentials.aws_role_arn,
  });
};

export const replaceBedrockBaselineModels = (
  baselinePayload: string,
  modelInfo: IModelInfo[],
) => {
  if (!baselinePayload) return baselinePayload;
  const baseline = JSON.parse(baselinePayload) as Record<string, any>;
  baseline.model_info = modelInfo;
  return JSON.stringify(baseline);
};

export const shouldAcknowledgeBedrockInstanceModels = (
  authMode: AuthMode,
  isDraft: boolean,
  catalogCredentialsDirty: boolean,
) => authMode === 'bedrock_api_key' && !isDraft && !catalogCredentialsDirty;
