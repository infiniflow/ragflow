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
  getBedrockCatalogCredentialScopeFromPayload,
  getBedrockModelListRequest,
  replaceBedrockBaselineModels,
  shouldAcknowledgeBedrockInstanceModels,
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
  ] as const)('changes when %s changes', (field, value) => {
    expect(
      getBedrockCatalogCredentialScope({ ...Values, [field]: value }),
    ).not.toBe(getBedrockCatalogCredentialScope(Values));
  });

  it('tracks only credentials used by the selected authentication mode', () => {
    expect(
      getBedrockCatalogCredentialScope({
        ...Values,
        bedrock_api_key: 'hidden-token',
        aws_role_arn: 'hidden-role',
      }),
    ).toBe(getBedrockCatalogCredentialScope(Values));

    expect(
      getBedrockCatalogCredentialScope({
        ...Values,
        auth_mode: 'iam_role',
        aws_role_arn: 'changed-role',
      }),
    ).not.toBe(
      getBedrockCatalogCredentialScope({
        ...Values,
        auth_mode: 'iam_role',
        aws_role_arn: 'role',
      }),
    );
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

  it('derives the persisted scope from the acknowledged payload', () => {
    const payloadScope = getBedrockCatalogCredentialScopeFromPayload({
      api_key: {
        auth_mode: 'bedrock_api_key',
        bedrock_api_key: 'sent-token',
        bedrock_endpoint_type: 'runtime',
        bedrock_region: 'ap-northeast-1',
      },
      region: 'ap-northeast-1',
    });

    expect(payloadScope).toBe(
      getBedrockCatalogCredentialScope({
        auth_mode: 'bedrock_api_key',
        bedrock_api_key: 'sent-token',
        bedrock_endpoint_type: 'runtime',
        bedrock_region: 'ap-northeast-1',
      }),
    );
  });

  it('updates only persisted models in an existing baseline', () => {
    const baseline = JSON.stringify({
      api_key: { bedrock_api_key: 'persisted-token' },
      region: 'ap-northeast-1',
      model_info: [{ model_name: 'model-a', max_tokens: 1024 }],
    });
    const models = [
      {
        model_name: 'model-a',
        model_type: ['chat'],
        max_tokens: 2048,
      },
    ];

    expect(JSON.parse(replaceBedrockBaselineModels(baseline, models))).toEqual({
      api_key: { bedrock_api_key: 'persisted-token' },
      region: 'ap-northeast-1',
      model_info: models,
    });
  });

  it('acknowledges instance-model callbacks only for persisted API-key models', () => {
    expect(
      shouldAcknowledgeBedrockInstanceModels('bedrock_api_key', false, false),
    ).toBe(true);
    expect(
      shouldAcknowledgeBedrockInstanceModels('access_key_secret', false, false),
    ).toBe(false);
    expect(
      shouldAcknowledgeBedrockInstanceModels('bedrock_api_key', false, true),
    ).toBe(false);
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
