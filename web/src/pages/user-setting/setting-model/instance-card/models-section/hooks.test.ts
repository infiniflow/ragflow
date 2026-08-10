/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

const mockListProviderModels = jest.fn();
jest.mock('@/hooks/use-llm-request', () => ({
  useListProviderModels: () => ({
    listProviderModels: mockListProviderModels,
  }),
}));
jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {},
}));
jest.mock('../available-models', () => ({
  sortModelTypes: (types: string[]) => types,
}));

import { LLMFactory } from '@/constants/llm';
import { act, renderHook } from '@testing-library/react';
import {
  areCatalogCredentialsReady,
  buildVerifyArgs,
  isCatalogBaseURLReady,
  useModelsCatalog,
  useModelsDerived,
} from './hooks';

describe('areCatalogCredentialsReady', () => {
  it('does not treat a Bedrock API key mode without its token as ready', () => {
    expect(
      areCatalogCredentialsReady(
        LLMFactory.Bedrock,
        '',
        'ap-northeast-1',
        'bedrock_api_key',
      ),
    ).toBe(false);
  });

  it('requires both the Bedrock token and region', () => {
    expect(
      areCatalogCredentialsReady(
        LLMFactory.Bedrock,
        'token',
        'ap-northeast-1',
        'bedrock_api_key',
      ),
    ).toBe(true);
  });

  it('allows static catalogs for SigV4 authentication modes', () => {
    expect(
      areCatalogCredentialsReady(
        LLMFactory.Bedrock,
        '',
        'ap-northeast-1',
        'access_key_secret',
      ),
    ).toBe(true);
  });
});

describe('isCatalogBaseURLReady', () => {
  it('allows Bedrock Runtime discovery without a custom base URL', () => {
    expect(isCatalogBaseURLReady(LLMFactory.Bedrock, '')).toBe(true);
  });

  it('still requires configured base URLs for generic providers', () => {
    expect(isCatalogBaseURLReady(LLMFactory.Ollama, '')).toBe(false);
    expect(
      isCatalogBaseURLReady(LLMFactory.Ollama, 'http://localhost:11434'),
    ).toBe(true);
  });

  it('treats a provider without a base URL field as ready', () => {
    expect(isCatalogBaseURLReady(LLMFactory.VolcEngine, undefined)).toBe(true);
  });
});

describe('buildVerifyArgs', () => {
  const model = {
    name: 'qwen.qwen3-coder-30b-a3b-v1:0',
    model_types: ['chat'],
    max_tokens: 8192,
    features: null,
  };

  it('uses transformed Bedrock credentials for model verification', () => {
    const credentials = {
      auth_mode: 'bedrock_api_key',
      bedrock_api_key: 'token',
      bedrock_region: 'ap-northeast-1',
      bedrock_endpoint_type: 'runtime',
    };

    expect(
      buildVerifyArgs(
        model,
        LLMFactory.Bedrock,
        () => ({
          apiKey: 'token',
          baseUrl: '',
          region: 'ap-northeast-1',
          extensions: {},
        }),
        undefined,
        () => ({ api_key: 'token' }),
        () => ({
          apiKey: JSON.stringify(credentials),
          baseUrl: '',
          region: 'ap-northeast-1',
        }),
      ),
    ).toMatchObject({
      api_key: JSON.stringify(credentials),
      base_url: '',
      region: 'ap-northeast-1',
      model_info: [
        {
          model_name: model.name,
          model_type: ['chat'],
          max_tokens: 8192,
        },
      ],
    });
  });

  it('keeps generic credentials when no transform is configured', () => {
    expect(
      buildVerifyArgs(
        model,
        LLMFactory.OpenAI,
        () => ({
          apiKey: 'token',
          baseUrl: 'https://api.example.com',
          region: '',
          extensions: {},
        }),
        undefined,
        undefined,
        undefined,
      ),
    ).toMatchObject({
      api_key: 'token',
      base_url: 'https://api.example.com',
    });
  });

  it('does not persist verification when no saved instance is supplied', () => {
    const args = buildVerifyArgs(
      model,
      LLMFactory.Bedrock,
      () => ({
        apiKey: 'token',
        baseUrl: '',
        region: 'ap-northeast-1',
        extensions: {},
      }),
      undefined,
      undefined,
      undefined,
    );

    expect(args).not.toHaveProperty('instance_id');
  });
});

describe('useModelsCatalog', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockListProviderModels.mockReset();
    mockListProviderModels.mockResolvedValue({ code: 0, data: [] });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('debounces Bedrock auto-fetch and cancels stale credentials', async () => {
    let apiKey = 'first-token';
    const resolveCreds = () => ({
      apiKey,
      baseUrl: '',
      region: 'ap-northeast-1',
      extensions: { auth_mode: 'bedrock_api_key' },
    });
    const { rerender } = renderHook(
      ({ apiKeyValue }) =>
        useModelsCatalog({
          providerName: LLMFactory.Bedrock,
          instanceName: 'bedrock-instance',
          hideActions: false,
          resolveCreds,
          instanceModels: [],
          apiKeyValue,
          baseUrlValue: '',
          instanceDetailsLoaded: true,
          regionValue: 'ap-northeast-1',
          authMode: 'bedrock_api_key',
        }),
      { initialProps: { apiKeyValue: apiKey } },
    );

    act(() => {
      jest.advanceTimersByTime(499);
    });
    expect(mockListProviderModels).not.toHaveBeenCalled();

    apiKey = 'second-token';
    rerender({ apiKeyValue: apiKey });
    await act(async () => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(mockListProviderModels).toHaveBeenCalledTimes(1);
    expect(mockListProviderModels).toHaveBeenCalledWith(
      expect.objectContaining({ api_key: 'second-token' }),
    );
  });
});

describe('useModelsDerived', () => {
  it('keeps a draft selection instead of selecting the full catalog', () => {
    const onInstanceModelsChange = jest.fn();
    const persistedModels = [
      {
        name: 'model-a',
        model_type: 'chat',
        max_tokens: 1024,
      },
    ];
    const catalog = [
      {
        name: 'model-a',
        model_types: ['chat'],
        max_tokens: 8192,
        features: null,
      },
      {
        name: 'model-b',
        model_types: ['chat'],
        max_tokens: 8192,
        features: null,
      },
      {
        name: 'model-c',
        model_types: ['chat'],
        max_tokens: 8192,
        features: null,
      },
    ];

    const { result } = renderHook(() =>
      useModelsDerived({
        catalog,
        instanceModels: persistedModels as any,
        draftModels: persistedModels as any,
        isDraftInstance: true,
        onInstanceModelsChange,
      }),
    );

    expect(result.current.instanceItems.map((model) => model.name)).toEqual([
      'model-a',
    ]);
    expect(result.current.models.map((model) => model.name)).toEqual([
      'model-a',
      'model-b',
      'model-c',
    ]);
    expect([...result.current.addedSet]).toEqual(['model-a']);
    expect(onInstanceModelsChange).toHaveBeenLastCalledWith([
      expect.objectContaining({ model_name: 'model-a' }),
    ]);
  });
});
