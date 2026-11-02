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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useRef } from 'react';

import { modelNameKey } from '@/utils/llm-util';
import { useInstanceSaveState } from '../hooks';
import {
  DRAFT_INSTANCE_SENTINEL,
  useModelMutations,
  useModelsCatalog,
  useModelsDerived,
  useModelEdit,
  useModelVerify,
  useResolveCreds,
} from './hooks';

const mockListProviderModels = jest.fn();
const mockUpdateProviderInstance = jest.fn();
const mockVerifyProviderConnection = jest.fn();
const mockPatchInstanceModel = jest.fn();
function mockInstanceModelsKey(providerName: string, instanceName: string) {
  return ['addedProviders', providerName, instanceName, 'models'];
}

jest.mock('@/hooks/use-llm-request', () => ({
  LlmKeys: { instanceModels: mockInstanceModelsKey },
  useAddInstanceModel: () => ({ addInstanceModel: jest.fn() }),
  useDeleteInstanceModels: () => ({ deleteInstanceModels: jest.fn() }),
  useListProviderModels: () => ({
    listProviderModels: mockListProviderModels,
  }),
  usePatchInstanceModel: () => ({
    patchInstanceModel: mockPatchInstanceModel,
    loading: false,
  }),
  useUpdateProviderInstance: () => ({
    updateProviderInstance: mockUpdateProviderInstance,
    loading: false,
  }),
  useVerifyProviderConnection: () => ({
    verifyProviderConnection: mockVerifyProviderConnection,
  }),
}));

// The module chain under test (instance-card/hooks -> provider-schema field
// configs) reads `FormFieldType` at import time, so the stub must expose it.
jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_t, key) => String(key) }),
}));

jest.mock('../../provider-schema/hooks', () => ({
  useProviderFields: jest.fn(),
}));

jest.mock('../use-custom-model-fields', () => ({
  useCustomModelFields: () => [],
}));

jest.mock('@/services/llm-service', () => ({
  __esModule: true,
  default: { verifyProviderConnection: jest.fn() },
}));

jest.mock('../available-models', () => ({
  sortModelTypes: (types: string[]) => types,
}));

describe('Bedrock model credentials', () => {
  beforeEach(() => {
    mockListProviderModels.mockReset().mockResolvedValue({ code: 0, data: [] });
    mockUpdateProviderInstance.mockReset().mockResolvedValue(undefined);
    mockVerifyProviderConnection.mockReset().mockResolvedValue({ code: 0 });
  });

  it('waits for saved Bedrock API key details before listing models', async () => {
    let apiKey: Record<string, string> = {
      auth_mode: 'access_key_secret',
      bedrock_api_key: '',
      bedrock_region: '',
    };
    const resolveCreds = () => ({ apiKey, baseUrl: undefined });

    const { rerender } = renderHook(
      ({ instanceDetailsLoaded }) =>
        useModelsCatalog({
          providerName: 'Bedrock',
          instanceName: 'bedrock-instance',
          hideActions: false,
          resolveCreds,
          instanceModels: undefined,
          apiKeyValue: apiKey,
          baseUrlValue: undefined,
          instanceDetailsLoaded,
        }),
      { initialProps: { instanceDetailsLoaded: false } },
    );

    expect(mockListProviderModels).not.toHaveBeenCalled();

    apiKey = {
      auth_mode: 'bedrock_api_key',
      bedrock_api_key: 'saved-key',
      bedrock_region: 'us-east-1',
    };
    rerender({ instanceDetailsLoaded: true });

    await waitFor(() =>
      expect(mockListProviderModels).toHaveBeenCalledWith({
        provider_name: 'Bedrock',
        api_key: apiKey,
        base_url: undefined,
        instance_name: 'bedrock-instance',
      }),
    );
  });

  it('lists models explicitly for a Bedrock API key draft', async () => {
    const apiKey = {
      auth_mode: 'bedrock_api_key',
      bedrock_api_key: 'draft-key',
      bedrock_region: 'us-east-1',
    };
    const resolveCreds = () => ({ apiKey, baseUrl: undefined });

    const { result } = renderHook(() =>
      useModelsCatalog({
        providerName: 'Bedrock',
        instanceName: DRAFT_INSTANCE_SENTINEL,
        hideActions: false,
        resolveCreds,
        instanceModels: undefined,
        apiKeyValue: apiKey,
        baseUrlValue: undefined,
      }),
    );

    expect(mockListProviderModels).not.toHaveBeenCalled();

    await act(async () => result.current.handleListModels());

    expect(mockListProviderModels).toHaveBeenCalledWith({
      provider_name: 'Bedrock',
      api_key: apiKey,
      base_url: undefined,
    });
  });

  it('preserves structured credentials for verify and batch update', async () => {
    const apiKey = {
      auth_mode: 'bedrock_api_key',
      bedrock_api_key: 'saved-key',
      bedrock_region: 'us-east-1',
    };
    const model = {
      name: 'anthropic.claude',
      model_types: ['chat'],
      max_tokens: 8192,
      features: [],
    };
    const instance = {
      id: 'instance-id',
      instance_name: 'bedrock-instance',
      provider_id: 'provider-id',
      region: 'default',
      status: 'active',
      api_key: '',
    };

    const { result } = renderHook(() => {
      const { resolveCreds } = useResolveCreds(instance, () => ({
        api_key: apiKey,
      }));
      const verify = useModelVerify({
        providerName: 'Bedrock',
        resolveCreds,
        instanceModels: [],
        instance,
        getFormValues: () => ({ auth_mode: 'bedrock_api_key' }),
        verifyTransform: () => ({ apiKey, region: 'default' }),
      });
      const mutations = useModelMutations({
        providerName: 'Bedrock',
        instanceName: instance.instance_name,
        isDraftInstance: false,
        hideActions: false,
        resolveCreds,
        instance,
        instanceItems: [],
        filteredModels: [model],
        isModelAdded: () => false,
        setCatalog: jest.fn(),
        clearCatalogOverride: jest.fn(),
      });
      return { verify, mutations };
    });

    await act(async () => result.current.verify.handleVerify(model));
    expect(mockVerifyProviderConnection).toHaveBeenCalledWith(
      expect.objectContaining({ api_key: apiKey, region: 'default' }),
    );

    await act(async () => result.current.mutations.handleBatchToggleModels());
    expect(mockUpdateProviderInstance).toHaveBeenCalledWith(
      expect.objectContaining({ api_key: apiKey, region: 'default' }),
    );
  });
});

describe('API-key-gated model listing (SystemOne / RAGcon)', () => {
  beforeEach(() => {
    mockListProviderModels.mockReset().mockResolvedValue({ code: 0, data: [] });
  });

  it('defers the SystemOne auto-fetch until an api_key is available', async () => {
    let apiKey = '';
    const resolveCreds = () => ({ apiKey, baseUrl: undefined });

    const { rerender } = renderHook(
      ({ apiKeyValue }) =>
        useModelsCatalog({
          providerName: 'SystemOne',
          instanceName: DRAFT_INSTANCE_SENTINEL,
          hideActions: false,
          resolveCreds,
          instanceModels: undefined,
          apiKeyValue,
          baseUrlValue: undefined,
        }),
      { initialProps: { apiKeyValue: apiKey } },
    );

    expect(mockListProviderModels).not.toHaveBeenCalled();

    apiKey = 'draft-key';
    rerender({ apiKeyValue: apiKey });

    await waitFor(() =>
      expect(mockListProviderModels).toHaveBeenCalledWith({
        provider_name: 'SystemOne',
        api_key: apiKey,
        base_url: undefined,
      }),
    );
  });

  it('defers the RAGcon auto-fetch when base_url is set but api_key is missing', async () => {
    let apiKey = '';
    const baseUrl = 'https://api.ragcon.ai/v1';
    const resolveCreds = () => ({ apiKey, baseUrl });

    const { rerender } = renderHook(
      ({ apiKeyValue }) =>
        useModelsCatalog({
          providerName: 'RAGcon',
          instanceName: DRAFT_INSTANCE_SENTINEL,
          hideActions: false,
          resolveCreds,
          instanceModels: undefined,
          apiKeyValue,
          baseUrlValue: baseUrl,
        }),
      { initialProps: { apiKeyValue: apiKey } },
    );

    // base_url alone must not trigger the fetch — the key gate still applies.
    expect(mockListProviderModels).not.toHaveBeenCalled();

    apiKey = 'draft-key';
    rerender({ apiKeyValue: apiKey });

    await waitFor(() =>
      expect(mockListProviderModels).toHaveBeenCalledWith({
        provider_name: 'RAGcon',
        api_key: apiKey,
        base_url: baseUrl,
      }),
    );
  });

  it('auto-fetches a saved RAGcon instance once its stored api_key loads', async () => {
    let apiKey = '';
    const baseUrl = 'https://api.ragcon.ai/v1';
    const resolveCreds = () => ({ apiKey, baseUrl });

    const { rerender } = renderHook(
      ({ apiKeyValue, instanceDetailsLoaded }) =>
        useModelsCatalog({
          providerName: 'RAGcon',
          instanceName: 'ragcon-instance',
          hideActions: false,
          resolveCreds,
          instanceModels: undefined,
          apiKeyValue,
          baseUrlValue: baseUrl,
          instanceDetailsLoaded,
        }),
      { initialProps: { apiKeyValue: apiKey, instanceDetailsLoaded: false } },
    );

    expect(mockListProviderModels).not.toHaveBeenCalled();

    apiKey = 'saved-key';
    rerender({ apiKeyValue: apiKey, instanceDetailsLoaded: true });

    await waitFor(() =>
      expect(mockListProviderModels).toHaveBeenCalledWith({
        provider_name: 'RAGcon',
        api_key: apiKey,
        base_url: baseUrl,
        instance_name: 'ragcon-instance',
      }),
    );
  });
});

describe('saved instance model baseline', () => {
  it('does not accept a failed model query as an authoritative snapshot', () => {
    const onInstanceModelsEdited = jest.fn();

    renderHook(() =>
      useModelsDerived({
        catalog: [],
        instanceModels: [],
        instanceModelsLoading: false,
        instanceModelsSucceeded: false,
        draftModels: [],
        isDraftInstance: false,
        onInstanceModelsChange: jest.fn(),
        onInstanceModelsEdited,
      }),
    );

    expect(onInstanceModelsEdited).not.toHaveBeenCalled();
  });

  it('stays clean after persisted models load without hiding credential edits', async () => {
    const initialValues = {
      api_key: 'saved-key',
      base_url: '',
      region: 'default',
    };
    const instanceDetails = {
      id: 'instance-id',
      instance_name: 'saved-instance',
      provider_id: 'provider-id',
      region: 'default',
      status: 'active',
      api_key: 'saved-key',
    };
    const persistedModel = {
      name: 'saved-model',
      model_type: ['chat'],
      max_tokens: 8192,
      status: 'active',
    };
    const formValues = { ...initialValues };

    const { result, rerender } = renderHook(
      ({ instanceModels, instanceModelsLoading, instanceModelsSucceeded }) => {
        const modelInfoRef = useRef<
          {
            model_name: string;
            model_type: string | string[];
            max_tokens: number;
            extra?: Record<string, unknown>;
          }[]
        >([]);
        const formRef = useRef({
          submit: jest.fn(),
          isDirty: () => false,
          getValues: () => formValues,
          getFilteredValues: () => formValues,
          reset: jest.fn(),
          trigger: jest.fn(),
          watch: jest.fn(),
          watchDirty: jest.fn(),
          updateFieldType: jest.fn(),
          onFieldUpdate: jest.fn(),
        });
        const saveState = useInstanceSaveState({
          formRef,
          providerName: 'Bedrock',
          instanceName: instanceDetails.instance_name,
          instanceId: instanceDetails.id,
          isDraft: false,
          draftName: '',
          instanceDetails,
          initialValues,
          modelInfoRef,
        });

        useModelsDerived({
          catalog: [],
          instanceModels,
          instanceModelsLoading,
          instanceModelsSucceeded,
          draftModels: [],
          isDraftInstance: false,
          onInstanceModelsChange: (modelInfo) => {
            modelInfoRef.current = modelInfo;
          },
          onInstanceModelsEdited: saveState.markModelsEdited,
        });

        return saveState;
      },
      {
        initialProps: {
          instanceModels: [] as (typeof persistedModel)[],
          instanceModelsLoading: true,
          instanceModelsSucceeded: false,
        },
      },
    );

    formValues.api_key = 'changed-before-models-load';
    expect(result.current.getSavePayload()?.payload.api_key).toBe(
      'changed-before-models-load',
    );
    formValues.api_key = 'saved-key';

    rerender({
      instanceModels: [persistedModel],
      instanceModelsLoading: false,
      instanceModelsSucceeded: true,
    });

    await waitFor(() => expect(result.current.getSavePayload()).toBeNull());

    formValues.api_key = 'changed-key';
    expect(result.current.getSavePayload()?.payload.api_key).toBe(
      'changed-key',
    );
  });

  it('updates the saved snapshot only after a model edit succeeds', async () => {
    const providerName = 'Bedrock';
    const instanceName = 'saved-instance';
    const persistedModel = {
      name: 'saved-model',
      model_type: ['chat'],
      max_tokens: 4096,
      status: 'active',
    };
    const queryClient = new QueryClient();
    const queryKey = mockInstanceModelsKey(providerName, instanceName);
    queryClient.setQueryData(queryKey, [persistedModel]);
    let resolvePatch!: (value: { code: number }) => void;
    mockPatchInstanceModel.mockReset().mockImplementationOnce(
      () =>
        new Promise<{ code: number }>((resolve) => {
          resolvePatch = resolve;
        }),
    );

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(
      () =>
        useModelEdit({
          providerName,
          instanceName,
          isModelAdded: (name: string) =>
            modelNameKey(name) === modelNameKey(persistedModel.name),
          updateCatalogModel: jest.fn(),
          clearCatalogOverride: jest.fn(),
        }),
      { wrapper },
    );

    act(() => {
      result.current.setEditingModel({
        name: persistedModel.name,
        model_types: persistedModel.model_type,
        max_tokens: persistedModel.max_tokens,
        features: [],
      });
    });
    let submitPromise!: Promise<void>;
    act(() => {
      submitPromise = result.current.handleEditSubmit({
        name: persistedModel.name,
        model_types: persistedModel.model_type,
        max_tokens: 8192,
        features: [],
      });
    });

    expect(queryClient.getQueryData(queryKey)).toEqual([persistedModel]);

    await act(async () => {
      resolvePatch({ code: 0 });
      await submitPromise;
    });

    expect(mockPatchInstanceModel).toHaveBeenCalledWith(
      expect.objectContaining({
        model_name: persistedModel.name,
        max_tokens: 8192,
      }),
    );
    expect(queryClient.getQueryData(queryKey)).toEqual([
      expect.objectContaining({ max_tokens: 8192 }),
    ]);

    mockPatchInstanceModel.mockResolvedValueOnce({ code: 1 });
    act(() => {
      result.current.setEditingModel({
        name: persistedModel.name,
        model_types: persistedModel.model_type,
        max_tokens: 8192,
        features: [],
      });
    });
    await act(async () => {
      await result.current.handleEditSubmit({
        name: persistedModel.name,
        model_types: persistedModel.model_type,
        max_tokens: 16384,
        features: [],
      });
    });

    expect(queryClient.getQueryData(queryKey)).toEqual([
      expect.objectContaining({ max_tokens: 8192 }),
    ]);
  });
});

describe('model name matching', () => {
  it('collapses instance and catalog entries that differ only in case', () => {
    const { result } = renderHook(() =>
      useModelsDerived({
        catalog: [
          {
            name: 'gpt-4o',
            model_types: ['chat'],
            max_tokens: 4096,
            features: [],
          },
          {
            name: 'embedding-1',
            model_types: ['embedding'],
            max_tokens: 512,
            features: [],
          },
        ],
        instanceModels: [
          {
            name: 'GPT-4O',
            model_type: ['chat'],
            max_tokens: 2048,
            status: 'active',
          },
        ],
        instanceModelsLoading: false,
        instanceModelsSucceeded: true,
        draftModels: [],
        isDraftInstance: false,
        onInstanceModelsChange: jest.fn(),
      }),
    );

    expect(result.current.models.map((m) => m.name)).toEqual([
      'GPT-4O',
      'embedding-1',
    ]);
    expect(result.current.isModelAdded('gpt-4o')).toBe(true);
  });
});
