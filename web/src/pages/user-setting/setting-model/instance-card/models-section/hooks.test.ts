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
const mockPatchInstanceModel = jest.fn();
const mockAddInstanceModel = jest.fn();
const mockDeleteInstanceModels = jest.fn();
const mockUpdateProviderInstance = jest.fn();
jest.mock('@/hooks/use-llm-request', () => ({
  LlmKeys: {
    instanceModels: (providerName: string, instanceName: string) => [
      'addedProviders',
      providerName,
      instanceName,
      'models',
    ],
  },
  useListProviderModels: () => ({
    listProviderModels: mockListProviderModels,
  }),
  usePatchInstanceModel: () => ({
    loading: false,
    patchInstanceModel: async (...args: unknown[]) =>
      (await mockPatchInstanceModel(...args)).data,
  }),
  useAddInstanceModel: () => ({
    addInstanceModel: mockAddInstanceModel,
  }),
  useDeleteInstanceModels: () => ({
    deleteInstanceModels: mockDeleteInstanceModels,
  }),
  useUpdateProviderInstance: () => ({
    loading: false,
    updateProviderInstance: mockUpdateProviderInstance,
  }),
}));
jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: {
    patchInstanceModel: (...args: unknown[]) => mockPatchInstanceModel(...args),
  },
}));
jest.mock('../available-models', () => ({
  sortModelTypes: (types: string[]) => types,
}));
jest.mock('../use-custom-model-fields', () => ({
  useCustomModelFields: () => [],
}));

import { LLMFactory } from '@/constants/llm';
import { LlmKeys } from '@/hooks/use-llm-request';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { createElement } from 'react';
import {
  areCatalogCredentialsReady,
  buildVerifyArgs,
  DRAFT_INSTANCE_SENTINEL,
  hasKnownModelTypes,
  isCatalogBaseURLReady,
  useModelsCatalog,
  useModelsDerived,
  useModelMutations,
  useModelEdit,
} from './hooks';

describe('hasKnownModelTypes', () => {
  it('distinguishes typed models from availability-only candidates', () => {
    expect(
      hasKnownModelTypes({
        name: 'typed',
        model_types: ['chat'],
        max_tokens: 8192,
        features: [],
      }),
    ).toBe(true);
    expect(
      hasKnownModelTypes({
        name: 'candidate',
        model_types: [],
        max_tokens: 8192,
        features: [],
      }),
    ).toBe(false);
  });
});

describe('useModelMutations', () => {
  it('disables batch mutations when only unclassified candidates are visible', () => {
    const { result } = renderHook(() =>
      useModelMutations({
        providerName: 'Bedrock',
        instanceName: DRAFT_INSTANCE_SENTINEL,
        isDraftInstance: true,
        hideActions: false,
        instanceItems: [],
        filteredModels: [
          {
            name: 'candidate',
            model_types: [],
            max_tokens: 8192,
            features: [],
          },
        ],
        addedSet: new Set(),
        setCatalog: jest.fn(),
        clearCatalogOverride: jest.fn(),
      }),
    );

    expect(result.current.canBatchToggle).toBe(false);
  });

  it('disables saved batch mutations until the canonical payload is available', () => {
    const { result } = renderHook(() =>
      useModelMutations({
        providerName: 'OpenAI',
        instanceName: 'saved',
        isDraftInstance: false,
        hideActions: false,
        instanceItems: [],
        filteredModels: [
          {
            name: 'model-a',
            model_types: ['chat'],
            max_tokens: 8192,
            features: [],
          },
        ],
        addedSet: new Set(),
        setCatalog: jest.fn(),
        clearCatalogOverride: jest.fn(),
      }),
    );

    expect(result.current.canBatchToggle).toBe(false);
  });

  it('uses the host canonical payload and acknowledges only successful batches', async () => {
    const clearCatalogOverride = jest.fn();
    const onInstanceModelsEdited = jest.fn();
    const buildInstanceUpdatePayload = jest.fn((modelInfo) => ({
      provider_name: 'BaiduYiYan',
      instance_name: 'saved',
      id: 'instance-id',
      api_key: { yiyan_ak: 'ak', yiyan_sk: 'sk' },
      model_info: modelInfo,
      verify: false,
    }));
    mockUpdateProviderInstance.mockResolvedValue({ code: 0 });
    const model = {
      name: 'ernie',
      model_types: ['chat'],
      max_tokens: 8192,
      features: [],
    };
    const { result } = renderHook(() =>
      useModelMutations({
        providerName: 'BaiduYiYan',
        instanceName: 'saved',
        isDraftInstance: false,
        hideActions: false,
        instanceItems: [],
        filteredModels: [model],
        addedSet: new Set(),
        setCatalog: jest.fn(),
        clearCatalogOverride,
        buildInstanceUpdatePayload,
        onInstanceModelsEdited,
      }),
    );

    await act(async () => result.current.handleBatchToggleModels());

    expect(buildInstanceUpdatePayload).toHaveBeenCalledWith([
      expect.objectContaining({ model_name: 'ernie', model_type: ['chat'] }),
    ]);
    expect(mockUpdateProviderInstance).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: { yiyan_ak: 'ak', yiyan_sk: 'sk' },
      }),
    );
    expect(clearCatalogOverride).toHaveBeenCalledWith('ernie');
    expect(onInstanceModelsEdited).toHaveBeenCalledWith([
      expect.objectContaining({ model_name: 'ernie' }),
    ]);
  });

  it('keeps catalog overrides when a batch update is rejected', async () => {
    const clearCatalogOverride = jest.fn();
    mockUpdateProviderInstance.mockResolvedValue({ code: 1 });
    const model = {
      name: 'model-a',
      model_types: ['chat'],
      max_tokens: 8192,
      features: [],
    };
    const { result } = renderHook(() =>
      useModelMutations({
        providerName: 'OpenAI',
        instanceName: 'saved',
        isDraftInstance: false,
        hideActions: false,
        instanceItems: [],
        filteredModels: [model],
        addedSet: new Set(),
        setCatalog: jest.fn(),
        clearCatalogOverride,
        buildInstanceUpdatePayload: (modelInfo) => ({
          provider_name: 'OpenAI',
          instance_name: 'saved',
          id: 'instance-id',
          api_key: 'token',
          model_info: modelInfo,
        }),
      }),
    );

    await act(async () => result.current.handleBatchToggleModels());

    expect(clearCatalogOverride).not.toHaveBeenCalled();
  });
});

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

  it('uses the canonical form region when a transform omits it', () => {
    expect(
      buildVerifyArgs(
        model,
        'SiliconFlow',
        () => ({
          apiKey: 'token',
          baseUrl: 'https://api.siliconflow.com/v1',
          region: 'intl',
          extensions: {},
        }),
        undefined,
        () => ({
          api_key: 'token',
          base_url: 'https://api.siliconflow.com/v1',
          region: 'intl',
        }),
        () => ({
          apiKey: 'token',
          baseUrl: 'https://api.siliconflow.com/v1',
        }),
      ),
    ).toMatchObject({ region: 'intl' });
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

  it('forwards the canonical input-select region when listing models', async () => {
    const { result } = renderHook(() =>
      useModelsCatalog({
        providerName: 'SiliconFlow',
        instanceName: 'intl-instance',
        hideActions: false,
        resolveCreds: () => ({
          apiKey: 'token',
          baseUrl: 'https://api.siliconflow.com/v1',
          region: 'intl',
          extensions: {},
        }),
        instanceModels: [],
        apiKeyValue: 'token',
        baseUrlValue: 'https://api.siliconflow.com/v1',
        instanceDetailsLoaded: true,
        regionValue: 'intl',
      }),
    );

    await act(async () => {
      await result.current.handleListModels();
    });

    expect(mockListProviderModels).toHaveBeenCalledWith(
      expect.objectContaining({ region: 'intl' }),
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

describe('useModelEdit acknowledgements', () => {
  const createWrapper = (queryClient: any) =>
    function Wrapper({ children }: any) {
      return createElement(
        QueryClientProvider,
        { client: queryClient },
        children,
      );
    };

  beforeEach(() => {
    mockPatchInstanceModel.mockReset();
  });

  it('updates the baseline callback only after PATCH succeeds', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    queryClient.setQueryData(LlmKeys.instanceModels('OpenAI', 'instance'), [
      {
        name: 'model-a',
        model_type: ['chat'],
        max_tokens: 1024,
        status: 'active',
      },
      {
        name: 'model-b',
        model_type: ['chat'],
        max_tokens: 1024,
        status: 'active',
      },
    ]);
    let resolvePatch!: (value: unknown) => void;
    mockPatchInstanceModel.mockReturnValue(
      new Promise((resolve) => {
        resolvePatch = resolve;
      }),
    );
    const onInstanceModelsEdited = jest.fn();
    const { result } = renderHook(
      () =>
        useModelEdit({
          providerName: 'OpenAI',
          instanceName: 'instance',
          addedSet: new Set(['model-a']),
          updateCatalogModel: jest.fn(),
          clearCatalogOverride: jest.fn(),
          onInstanceModelsEdited,
        }),
      { wrapper: createWrapper(queryClient) },
    );
    act(() => {
      result.current.setEditingModel({
        name: 'model-a',
        model_types: ['chat'],
        max_tokens: 1024,
        features: null,
      });
    });

    let submitPromise!: Promise<void>;
    act(() => {
      submitPromise = result.current.handleEditSubmit({
        name: 'model-a',
        model_types: ['chat'],
        max_tokens: 2048,
        features: null,
      });
    });
    expect(onInstanceModelsEdited).not.toHaveBeenCalled();

    queryClient.setQueryData<any[]>(
      LlmKeys.instanceModels('OpenAI', 'instance'),
      (current) =>
        current?.map((model) =>
          model.name === 'model-b' ? { ...model, max_tokens: 4096 } : model,
        ),
    );

    await act(async () => {
      resolvePatch({ data: { code: 0 } });
      await submitPromise;
    });

    expect(onInstanceModelsEdited).toHaveBeenCalledWith([
      expect.objectContaining({
        model_name: 'model-a',
        max_tokens: 2048,
      }),
      expect.objectContaining({
        model_name: 'model-b',
        max_tokens: 4096,
      }),
    ]);
  });

  it('does not acknowledge a failed PATCH', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    queryClient.setQueryData(LlmKeys.instanceModels('OpenAI', 'instance'), [
      {
        name: 'model-a',
        model_type: ['chat'],
        max_tokens: 1024,
        status: 'active',
      },
    ]);
    mockPatchInstanceModel.mockResolvedValue({ data: { code: 1 } });
    const onInstanceModelsEdited = jest.fn();
    const { result } = renderHook(
      () =>
        useModelEdit({
          providerName: 'OpenAI',
          instanceName: 'instance',
          addedSet: new Set(['model-a']),
          updateCatalogModel: jest.fn(),
          clearCatalogOverride: jest.fn(),
          onInstanceModelsEdited,
        }),
      { wrapper: createWrapper(queryClient) },
    );
    act(() => {
      result.current.setEditingModel({
        name: 'model-a',
        model_types: ['chat'],
        max_tokens: 1024,
        features: null,
      });
    });

    await act(async () => {
      await result.current.handleEditSubmit({
        name: 'model-a',
        model_types: ['chat'],
        max_tokens: 2048,
        features: null,
      });
    });

    expect(onInstanceModelsEdited).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData<any[]>(
        LlmKeys.instanceModels('OpenAI', 'instance'),
      )?.[0].max_tokens,
    ).toBe(1024);
  });
});
