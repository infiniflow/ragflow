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
) =>
  JSON.stringify([
    values.auth_mode,
    values.bedrock_endpoint_type,
    values.bedrock_api_key,
    values.bedrock_region,
    values.bedrock_endpoint_url,
    values.bedrock_discovery_endpoint_url,
    values.bedrock_ak,
    values.bedrock_sk,
    values.aws_role_arn,
  ]);
