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

import message from '@/components/ui/message';
import { ModelTypeToField } from '@/constants/llm';
import {
  IAddedModel,
  IAvailableProvider,
  IDefaultModel,
  IInstanceModel,
  IMyLlmValue,
  IProviderInstance,
} from '@/interfaces/database/llm';
import {
  IAddInstanceModelRequestBody,
  IAddProviderInstanceRequestBody,
  IAddProviderRequestBody,
  IDeleteInstanceModelsRequestBody,
  IDeleteProviderInstanceRequestBody,
  IEditInstanceModelRequestBody,
  IListAllModelsRequestParams,
  IListProviderModelsRequestBody,
  IListProvidersRequestParams,
  IModelInfo,
  IPatchInstanceModelRequestBody,
  ISetDefaultModelRequestBody,
  IUpdateModelStatusRequestBody,
  IUpdateProviderInstanceRequestBody,
} from '@/interfaces/request/llm';
import llmService from '@/services/llm-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import {
  buildModelValue,
  buildValidModelIds,
  parseModelValue,
} from '@/utils/llm-util';
import { useWarnEmptyModel } from './use-warn-empty-model';

export const enum LLMApiAction {
  AllModels = 'allModels',
  AvailableProviders = 'availableProviders',
  AddedProviders = 'addedProviders',
  AddProvider = 'addProvider',
  AddProviderInstance = 'addProviderInstance',
  VerifyProviderConnection = 'verifyProviderConnection',
  ListProviderModels = 'listProviderModels',
  AddInstanceModel = 'addInstanceModel',
  EditInstanceModel = 'editInstanceModel',
  DeleteProviderInstance = 'deleteProviderInstance',
  DeleteInstanceModels = 'deleteInstanceModels',
  UpdateProviderInstance = 'updateProviderInstance',
  PatchInstanceModel = 'patchInstanceModel',
  ListDefaultModels = 'listDefaultModels',
  SetDefaultModel = 'setDefaultModel',
}

export const LlmKeys = {
  availableProviders: () => [LLMApiAction.AvailableProviders] as const,
  addedProviders: () => [LLMApiAction.AddedProviders] as const,
  allModels: (modelType?: string) =>
    [LLMApiAction.AllModels, modelType] as const,
  providerInstances: (providerName: string) =>
    [LLMApiAction.AddedProviders, providerName, 'instances'] as const,
  providerInstance: (providerName: string, id: string) =>
    [LLMApiAction.AddedProviders, providerName, id, 'instance'] as const,
  instanceModels: (providerName: string, instanceName: string) =>
    [
      LLMApiAction.AddedProviders,
      providerName,
      instanceName,
      'models',
    ] as const,
  defaultModels: () => [LLMApiAction.ListDefaultModels] as const,
};

export const useFetchAvailableProviders = () => {
  const { data, isFetching: loading } = useQuery<IAvailableProvider[]>({
    queryKey: LlmKeys.availableProviders(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const params: IListProvidersRequestParams = { available: true };
      const { data } = await llmService.listProviders({ params }, true);

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchAddedProviders = () => {
  const { data, isFetching: loading } = useQuery<IAvailableProvider[]>({
    queryKey: LlmKeys.addedProviders(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await llmService.listProviders({ params: {} }, true);

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchAllAddedModels = (
  modelType?: string,
  ownerTenantId?: string,
) => {
  const {
    data,
    isFetching: loading,
    isFetched,
    isError,
  } = useQuery<IAddedModel[]>({
    queryKey: [...LlmKeys.allModels(modelType), ownerTenantId],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const params: IListAllModelsRequestParams = {};
      if (modelType) {
        params.type = modelType;
      }
      if (ownerTenantId) {
        params.owner_tenant_id = ownerTenantId;
      }
      const { data } = await llmService.listAllAddedModels({ params }, true);

      return data?.data ?? [];
    },
  });

  // `data` is seeded with `initialData: []`, so it can't tell a real empty
  // result apart from "fetch hasn't completed yet" — `isFetched` stays false
  // until a genuine response (or error) arrives.
  return { data, loading, isFetched, isError };
};

/**
 * The set of ids under which added models of the given types can be
 * referenced (both the model_id form and the legacy composite form), for
 * validating that a persisted form value still points at an existing model.
 * `isFetched` must be checked before trusting `validIds` — while the model
 * list is loading it is empty and every value would look missing.
 */
export const useModelValidIds = (
  modelTypes: string[],
  ownerTenantId?: string,
) => {
  const { data, isFetched } = useFetchAllAddedModels(undefined, ownerTenantId);

  const validIds = useMemo(
    () => buildValidModelIds(data, modelTypes),
    [data, modelTypes],
  );

  return { validIds, isFetched };
};

export function useFindLlmByUuid() {
  const { data: models } = useFetchAllAddedModels();

  return (uuid: string) => {
    const parsed = parseModelValue(uuid);
    if (parsed) {
      return models.find(
        (m) =>
          m.name === parsed.model_name &&
          m.instance_name === parsed.model_instance &&
          m.provider_name === parsed.model_provider,
      );
    }
    return undefined;
  };
}

export const useFetchProviderInstances = (providerName: string) => {
  const { data, isFetching: loading } = useQuery<IProviderInstance[]>({
    queryKey: LlmKeys.providerInstances(providerName),
    initialData: [],
    gcTime: 0,
    enabled: !!providerName,
    queryFn: async () => {
      const { data } = await llmService.listProviderInstances(
        { provider_name: providerName },
        true,
      );
      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchProviderInstance = (providerName: string, id: string) => {
  return useQuery<IProviderInstance>({
    queryKey: LlmKeys.providerInstance(providerName, id),
    initialData: undefined as unknown as IProviderInstance,
    gcTime: 0,
    enabled: false,
    queryFn: async () => {
      const { data } = await llmService.showProviderInstance(
        { provider_name: providerName, id },
        true,
      );
      return (data?.data ?? {}) as IProviderInstance;
    },
  });
};

export const useFetchInstanceModels = (
  providerName: string,
  instanceName: string,
) => {
  const {
    data,
    isFetching: loading,
    isSuccess,
  } = useQuery<IInstanceModel[]>({
    queryKey: LlmKeys.instanceModels(providerName, instanceName),
    initialData: [],
    gcTime: 0,
    enabled: !!providerName && !!instanceName && instanceName !== '__draft__',
    queryFn: async () => {
      const { data } = await llmService.listInstanceModels(
        { provider_name: providerName, instance_name: instanceName },
        true,
      );
      if (data?.code !== 0) {
        throw new Error(data?.message || 'Failed to fetch instance models');
      }
      return data?.data ?? [];
    },
  });

  return { data, loading, isSuccess };
};

export type LlmItem = { name: string; logo: string } & IMyLlmValue;

export const useAddProvider = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.AddProvider],
    mutationFn: async (params: IAddProviderRequestBody) => {
      try {
        const { data: listRes } = await llmService.listProviders(
          { params: {} },
          true,
        );
        const isProviderAdded = listRes?.data?.some(
          (p: IAvailableProvider) => p.name === params.provider_name,
        );
        if (isProviderAdded) {
          return { code: 0, data: null };
        }
      } catch {
        // ignore list failure and proceed to add
      }
      const { data } = await llmService.addProvider(params);
      return data;
    },
  });

  return { data, loading, addProvider: mutateAsync };
};

export const useAddProviderInstance = () => {
  const { addProvider } = useAddProvider();
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.AddProviderInstance],
    mutationFn: async (
      params: IAddProviderInstanceRequestBody & { verify?: boolean },
    ) => {
      try {
        await addProvider({ provider_name: params.llm_factory });
      } catch {
        // ignore and proceed to add
      }

      // The provider is carried in the URL path
      // (`/providers/<llm_factory>/instances`), so `llm_factory` must not
      // be duplicated in the request body. Keep it only for URL building
      // (native-config form) and send the remaining fields as the body.
      const { llm_factory, ...body } = params;
      const { data } = await llmService.addProviderInstance(
        { llm_factory, data: body },
        true,
      );
      if (data.code === 0 && !params.verify) {
        // Invalidate `addedProviders` so `has_instance` flips to `true`
        // for providers that just gained their first instance. Without
        // this, the parent page keeps `providerQueryName === ''` (the
        // `has_instance` gate in index.tsx) and the `providerInstances`
        // query stays disabled, so the newly-saved instance never
        // appears. `exact: true` avoids cascading into every
        // providerInstances / instanceModels query (they share the
        // `['AddedProviders', ...]` prefix) - the dedicated invalidation
        // below handles those.
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.providerInstances(params.llm_factory),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
      }
      return data;
    },
  });

  return { data, loading, addProviderInstance: mutateAsync };
};

export const useVerifyProviderConnection = () => {
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.VerifyProviderConnection],
    mutationFn: async (params: {
      provider_name: string;
      api_key: string;
      base_url?: string;
      region?: string;
      model_info?: IModelInfo[];
      instance_id?: string;
    }) => {
      const { data } = await llmService.verifyProviderConnection(params);
      return data;
    },
  });

  return { data, loading, verifyProviderConnection: mutateAsync };
};

export const useListProviderModels = () => {
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.ListProviderModels],
    mutationFn: async (params: IListProviderModelsRequestBody) => {
      const { provider_name, api_key, base_url } = params;
      // GET /api/v1/providers/<provider_name>/models
      // The API accepts api_key and base_url as optional query parameters.
      // api_key is expected as a string; values in {} object form must be
      // JSON-stringified before being sent.
      const queryParams: Record<string, string> = {};
      if (api_key) {
        queryParams.api_key =
          typeof api_key === 'string' ? api_key : JSON.stringify(api_key);
      }
      if (base_url) {
        queryParams.base_url = base_url;
      }
      const { data } = await llmService.listProviderModels(
        { provider_name, params: queryParams },
        true,
      );
      return data;
    },
  });

  return { loading, listProviderModels: mutateAsync };
};

export const useAddInstanceModel = () => {
  const queryClient = useQueryClient();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.AddInstanceModel],
    mutationFn: async (
      params: {
        provider_name: string;
        instance_name: string;
      } & IAddInstanceModelRequestBody,
    ) => {
      const { data } = await llmService.addInstanceModel(params);
      if (data.code === 0) {
        // `exact: true` keeps the invalidation to the provider summary
        // list. Without it the [AddedProviders] prefix would also match
        // every providerInstances / instanceModels query and refetch
        // every provider's instances — we only want the current one.
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
      }
      return data;
    },
  });

  return { data, loading, addInstanceModel: mutateAsync };
};

export const useEditInstanceModel = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.EditInstanceModel],
    mutationFn: async (
      params: {
        provider_name: string;
        instance_name: string;
      } & IEditInstanceModelRequestBody,
    ) => {
      const { data } = await llmService.editInstanceModel(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
      }
      return data;
    },
  });

  return { data, loading, editInstanceModel: mutateAsync };
};

export const useDeleteProviderInstance = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.DeleteProviderInstance],
    mutationFn: async (params: IDeleteProviderInstanceRequestBody) => {
      const { data } = await llmService.deleteProviderInstance(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.providerInstances(params.provider_name),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });

        message.success(t('message.deleted'));
      }
      return data;
    },
  });

  return { data, loading, deleteProviderInstance: mutateAsync };
};

export const useUpdateModelStatus = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.AddedProviders, 'updateModelStatus'],
    mutationFn: async (params: IUpdateModelStatusRequestBody) => {
      const { data } = await llmService.updateModelStatus(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
      }
      return data;
    },
  });

  return { loading, updateModelStatus: mutateAsync };
};

/**
 * PATCH `/providers/{name}/instances/{name}/models/{model_name}` — updates
 * a single model's editable fields (max_tokens, model_type, status, is_tools).
 * Used by the per-row Edit dialog. Distinct from `useUpdateModelStatus`
 * (which only flips the active/inactive bit) so call sites can pass the
 * full set of editable fields without coercing them into the status hook.
 */
export const usePatchInstanceModel = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.PatchInstanceModel],
    mutationFn: async (params: IPatchInstanceModelRequestBody) => {
      const { data } = await llmService.patchInstanceModel(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
      }
      return data;
    },
  });

  return { loading, patchInstanceModel: mutateAsync };
};

export const useDeleteInstanceModels = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.DeleteInstanceModels],
    mutationFn: async (params: IDeleteInstanceModelsRequestBody) => {
      const { data } = await llmService.deleteInstanceModels(params);
      if (data.code === 0) {
        message.success(t('message.deleted'));
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
      }
      return data;
    },
  });

  return { loading, deleteInstanceModels: mutateAsync };
};

export const useUpdateProviderInstance = () => {
  const queryClient = useQueryClient();
  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.UpdateProviderInstance],
    mutationFn: async (params: IUpdateProviderInstanceRequestBody) => {
      const { data } = await llmService.updateProviderInstance(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
          exact: true,
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.providerInstances(params.provider_name),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.providerInstance(params.provider_name, params.id),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.instanceModels(
            params.provider_name,
            params.instance_name,
          ),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
      }
      return data;
    },
  });

  return { loading, updateProviderInstance: mutateAsync };
};

export const useFetchDefaultModels = () => {
  const { data, isFetching: loading } = useQuery<IDefaultModel[]>({
    queryKey: LlmKeys.defaultModels(),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await llmService.listDefaultModels({}, true);
      return data?.data?.models ?? [];
    },
  });

  return { data, loading };
};

export const useFetchDefaultModelDictionary = (showEmptyModelWarn = false) => {
  const { data: defaultModels, loading } = useFetchDefaultModels();

  const result = useMemo(() => {
    const dict: Record<string, string> = {};
    Object.entries(ModelTypeToField).forEach(([key, field]) => {
      const model = defaultModels.find((m) => m.model_type === key);
      if (!model || !model.enable) {
        dict[field] = '';
        return;
      }
      dict[field] =
        model.model_id ||
        buildModelValue({
          model_name: model.model_name,
          model_instance: model.model_instance,
          model_provider: model.model_provider,
        });
    });
    return dict;
  }, [defaultModels]);

  useWarnEmptyModel(showEmptyModelWarn, result.embd_id, result.llm_id, loading);

  return result;
};

export const useSetDefaultModel = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  const { isPending: loading, mutateAsync } = useMutation({
    mutationKey: [LLMApiAction.SetDefaultModel],
    mutationFn: async (params: ISetDefaultModelRequestBody) => {
      const { data } = await llmService.setDefaultModel(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
        queryClient.invalidateQueries({
          queryKey: LlmKeys.defaultModels(),
        });
      }
      return data;
    },
  });

  return { loading, setDefaultModel: mutateAsync };
};
