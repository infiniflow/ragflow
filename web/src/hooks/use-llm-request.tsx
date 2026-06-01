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
  IDeleteProviderInstanceRequestBody,
  IListAllModelsRequestParams,
  IListProvidersRequestParams,
  ISetDefaultModelRequestBody,
  IUpdateModelStatusRequestBody,
} from '@/interfaces/request/llm';
import llmService from '@/services/llm-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { buildModelValue, parseModelValue } from '@/utils/llm-util';
import { useWarnEmptyModel } from './use-warn-empty-model';

export const enum LLMApiAction {
  AllModels = 'allModels',
  AvailableProviders = 'availableProviders',
  AddedProviders = 'addedProviders',
  AddProvider = 'addProvider',
  AddProviderInstance = 'addProviderInstance',
  AddInstanceModel = 'addInstanceModel',
  DeleteProviderInstance = 'deleteProviderInstance',
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

export const useFetchAllAddedModels = (modelType?: string) => {
  const { data, isFetching: loading } = useQuery<IAddedModel[]>({
    queryKey: LlmKeys.allModels(modelType),
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const params: IListAllModelsRequestParams = {};
      if (modelType) {
        params.type = modelType;
      }
      const { data } = await llmService.listAllAddedModels({ params }, true);

      return data?.data ?? [];
    },
  });

  return { data, loading };
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

export const useFetchInstanceModels = (
  providerName: string,
  instanceName: string,
) => {
  const { data, isFetching: loading } = useQuery<IInstanceModel[]>({
    queryKey: LlmKeys.instanceModels(providerName, instanceName),
    initialData: [],
    gcTime: 0,
    enabled: !!providerName && !!instanceName,
    queryFn: async () => {
      const { data } = await llmService.listInstanceModels(
        { provider_name: providerName, instance_name: instanceName },
        true,
      );
      return data?.data ?? [];
    },
  });

  return { data, loading };
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

        const { data: instancesRes } = await llmService.listProviderInstances(
          { provider_name: params.llm_factory },
          true,
        );
        const instanceExists = instancesRes?.data?.some(
          (i: IProviderInstance) => i.instance_name === params.instance_name,
        );
        if (instanceExists && !params.verify) {
          return { code: 0, data: null };
        }
      } catch {
        // ignore list failure and proceed to add
      }

      const { data } = await llmService.addProviderInstance(params);
      if (data.code === 0 && !params.verify) {
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
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
        queryClient.invalidateQueries({
          queryKey: LlmKeys.addedProviders(),
        });
        queryClient.invalidateQueries({
          queryKey: LlmKeys.allModels(),
        });
      }
      return data;
    },
  });

  return { data, loading, addInstanceModel: mutateAsync };
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
  const { data: defaultModels } = useFetchDefaultModels();

  const result = useMemo(() => {
    const dict: Record<string, string> = {};
    Object.entries(ModelTypeToField).forEach(([key, field]) => {
      const model = defaultModels.find((m) => m.model_type === key);
      dict[field] = model && model.enable ? buildModelValue(model) : '';
    });
    return dict;
  }, [defaultModels]);

  useWarnEmptyModel(showEmptyModelWarn, result.embd_id, result.llm_id);

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
