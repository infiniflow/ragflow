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
  getBedrockCatalogCredentialScope,
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
});
