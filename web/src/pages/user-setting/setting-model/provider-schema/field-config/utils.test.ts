/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

import { parseApiKeyAsObject, serializeProviderAPIKey } from './utils';

describe('parseApiKeyAsObject', () => {
  it('parses a persisted JSON credential bundle', () => {
    expect(
      parseApiKeyAsObject(
        '{"auth_mode":"bedrock_api_key","bedrock_api_key":"token"}',
      ),
    ).toEqual({ auth_mode: 'bedrock_api_key', bedrock_api_key: 'token' });
  });

  it('keeps an already parsed credential bundle', () => {
    const credentials = { auth_mode: 'iam_role', aws_role_arn: 'role' };

    expect(parseApiKeyAsObject(credentials)).toBe(credentials);
  });

  it('rejects malformed credential JSON', () => {
    expect(parseApiKeyAsObject('{not-json')).toBeUndefined();
  });
});

describe('serializeProviderAPIKey', () => {
  it('keeps a string API key unchanged', () => {
    expect(serializeProviderAPIKey('token')).toBe('token');
  });

  it('serializes a provider-specific API key object', () => {
    expect(serializeProviderAPIKey({ yiyan_ak: 'ak', yiyan_sk: 'sk' })).toBe(
      '{"yiyan_ak":"ak","yiyan_sk":"sk"}',
    );
  });

  it('maps missing API keys to an empty string', () => {
    expect(serializeProviderAPIKey(null)).toBe('');
    expect(serializeProviderAPIKey(undefined)).toBe('');
  });
});
