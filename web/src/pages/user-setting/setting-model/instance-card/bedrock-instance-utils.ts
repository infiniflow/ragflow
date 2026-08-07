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
