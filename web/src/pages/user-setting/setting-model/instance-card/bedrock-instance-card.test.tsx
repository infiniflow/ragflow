/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

import {
  getBedrockCatalogResetDecision,
  getBedrockCatalogCredentialScope,
  getBedrockModelListRequest,
  shouldResetBedrockForm,
} from './bedrock-instance-utils';
import type { BedrockFormValues } from './bedrock-instance-utils';

const Values: BedrockFormValues = {
  auth_mode: 'access_key_secret',
  bedrock_ak: 'ak',
  bedrock_sk: 'sk',
  aws_role_arn: '',
  bedrock_api_key: '',
  bedrock_endpoint_type: 'runtime',
  bedrock_endpoint_url: '',
  bedrock_discovery_endpoint_url: '',
  bedrock_region: 'ap-northeast-1',
  llm_name: '',
  max_tokens: 8192,
  model_type: ['chat'],
};

describe('Bedrock catalog credential scope', () => {
  it.each([
    ['bedrock_ak', 'changed-ak'],
    ['bedrock_sk', 'changed-sk'],
    ['aws_role_arn', 'changed-role'],
  ] as const)('changes when %s changes', (field, value) => {
    expect(
      getBedrockCatalogCredentialScope({ ...Values, [field]: value }),
    ).not.toBe(getBedrockCatalogCredentialScope(Values));
  });

  it('does not reset for an identical instance-details refetch', () => {
    expect(shouldResetBedrockForm(Values, { ...Values })).toBe(false);
  });

  it('does not leave a pending reset when the refetched scope is current', () => {
    expect(getBedrockCatalogResetDecision('scope', 'scope', 'scope')).toEqual({
      pendingReset: null,
      invalidateCatalogImmediately: false,
    });
  });

  it('invalidates immediately when a new persisted scope is already current', () => {
    expect(
      getBedrockCatalogResetDecision(
        'persisted-scope',
        'next-scope',
        'next-scope',
      ),
    ).toEqual({
      pendingReset: null,
      invalidateCatalogImmediately: true,
    });
  });

  it('defers a reset until a different watched scope is applied', () => {
    expect(
      getBedrockCatalogResetDecision(
        'persisted-scope',
        'current-scope',
        'next-scope',
      ),
    ).toEqual({
      pendingReset: {
        scope: 'next-scope',
        invalidateCatalog: true,
      },
      invalidateCatalogImmediately: false,
    });
  });
});

describe('Bedrock model list request', () => {
  it('omits the Runtime discovery endpoint for Mantle', () => {
    const request = getBedrockModelListRequest({
      ...Values,
      auth_mode: 'bedrock_api_key',
      bedrock_api_key: 'token',
      bedrock_endpoint_type: 'mantle_anthropic',
      bedrock_endpoint_url:
        'https://bedrock-mantle.ap-northeast-1.api.aws/anthropic',
      bedrock_discovery_endpoint_url: 'https://attacker.example.com',
    });

    expect(request.extensions).not.toHaveProperty('discovery_endpoint_url');
  });
});
