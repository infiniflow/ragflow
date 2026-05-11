import { LlmIcon } from '@/components/svg-icon';
import message from '@/components/ui/message';
import { LlmModelType } from '@/constants/knowledge';
import { DefaultOptionType } from '@/interfaces/antd-compat';
import { ResponseGetType } from '@/interfaces/database/base';
import {
  IAvailableProvider,
  IFactory,
  IInstanceModel,
  IMyLlmValue,
  IProviderInstance,
  IThirdOAIModelCollection as IThirdAiModelCollection,
  IThirdOAIModel,
  IThirdOAIModelCollection,
} from '@/interfaces/database/llm';
import {
  IAddLlmRequestBody,
  IAddProviderInstanceRequestBody,
  IAddProviderRequestBody,
  IDeleteLlmRequestBody,
  IDeleteProviderInstanceRequestBody,
  IListProvidersRequestParams,
  IUpdateModelStatusRequestBody,
} from '@/interfaces/request/llm';
import llmService from '@/services/llm-service';
import userService from '@/services/user-service';
import { getLLMIconName, getRealModelName } from '@/utils/llm-util';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { buildLlmUuid } from '@/utils/llm-util';

export const enum LLMApiAction {
  LlmList = 'llmList',
  MyLlmList = 'myLlmList',
  MyLlmListDetailed = 'myLlmListDetailed',
  FactoryList = 'factoryList',
  AvailableProviders = 'availableProviders',
  AddedProviders = 'addedProviders',
  SaveApiKey = 'saveApiKey',
  SaveTenantInfo = 'saveTenantInfo',
  AddLlm = 'addLlm',
  AddProvider = 'addProvider',
  AddProviderInstance = 'addProviderInstance',
  DeleteLlm = 'deleteLlm',
  EnableLlm = 'enableLlm',
  DeleteFactory = 'deleteFactory',
  DeleteProviderInstance = 'deleteProviderInstance',
}

export const useFetchLlmList = (modelType?: LlmModelType) => {
  const { data } = useQuery<IThirdAiModelCollection>({
    queryKey: [LLMApiAction.LlmList],
    initialData: {},
    queryFn: async () => {
      const { data } = await userService.llmList({ model_type: modelType });

      return data?.data ?? {};
    },
  });

  return data;
};

type IThirdOAIModelWithUuid = IThirdOAIModel & { uuid: string };

export function useSelectFlatLlmList(modelType?: LlmModelType) {
  const llmList = useFetchLlmList(modelType);

  return Object.values(llmList).reduce<IThirdOAIModelWithUuid[]>((pre, cur) => {
    pre.push(...cur.map((x) => ({ ...x, uuid: buildLlmUuid(x) })));

    return pre;
  }, []);
}

export function useFindLlmByUuid(modelType?: LlmModelType) {
  const flatList = useSelectFlatLlmList(modelType);

  return (uuid: string) => {
    return flatList.find((x) => x.uuid === uuid);
  };
}

function buildLlmOptionsWithIcon(x: IThirdOAIModel) {
  return {
    label: (
      <div className="flex items-center justify-center gap-2">
        <LlmIcon
          name={getLLMIconName(x.fid, x.llm_name)}
          width={24}
          height={24}
          imgClass="size-6"
        />
        <span>{getRealModelName(x.llm_name)}</span>
      </div>
    ),
    value: `${x.llm_name}@${x.fid}`,
    disabled: !x.available,
    is_tools: x.is_tools,
  };
}

export const useSelectLlmOptionsByModelType = () => {
  const llmInfo: IThirdOAIModelCollection = useFetchLlmList();

  const groupImage2TextOptions = useCallback(() => {
    const modelType = LlmModelType.Image2text;
    const modelTag = modelType.toUpperCase();
    return Object.entries(llmInfo)
      .map(([key, value]) => {
        return {
          label: key,
          options: value
            .filter(
              (x) =>
                (x.model_type.includes(modelType) ||
                  (x.tags && x.tags.includes(modelTag))) &&
                x.available &&
                x.status === '1',
            )
            .map(buildLlmOptionsWithIcon),
        };
      })
      .filter((x) => x.options.length > 0);
  }, [llmInfo]);

  const groupOptionsByModelType = useCallback(
    (modelType: LlmModelType) => {
      return Object.entries(llmInfo)
        .filter(([, value]) =>
          modelType
            ? value.some((x) => x.model_type.includes(modelType))
            : true,
        )
        .map(([key, value]) => {
          return {
            label: key,
            options: value
              .filter(
                (x) =>
                  (modelType ? x.model_type.includes(modelType) : true) &&
                  x.available,
              )
              .map(buildLlmOptionsWithIcon),
          };
        })
        .filter((x) => x.options.length > 0);
    },
    [llmInfo],
  );

  return {
    [LlmModelType.Chat]: groupOptionsByModelType(LlmModelType.Chat),
    [LlmModelType.Embedding]: groupOptionsByModelType(LlmModelType.Embedding),
    [LlmModelType.Image2text]: groupImage2TextOptions(),
    [LlmModelType.Speech2text]: groupOptionsByModelType(
      LlmModelType.Speech2text,
    ),
    [LlmModelType.Rerank]: groupOptionsByModelType(LlmModelType.Rerank),
    [LlmModelType.TTS]: groupOptionsByModelType(LlmModelType.TTS),
    [LlmModelType.Ocr]: groupOptionsByModelType(LlmModelType.Ocr),
  };
};

// Merge different types of models from the same manufacturer under one manufacturer
export const useComposeLlmOptionsByModelTypes = (
  modelTypes: LlmModelType[],
) => {
  const allOptions = useSelectLlmOptionsByModelType();
  return modelTypes.reduce<
    (DefaultOptionType & {
      options: {
        label: JSX.Element;
        value: string;
        disabled: boolean;
        is_tools: boolean;
      }[];
    })[]
  >((pre, cur) => {
    const options = allOptions[cur];
    options.forEach((x) => {
      const item = pre.find((y) => y.label === x.label);
      if (item) {
        x.options.forEach((y) => {
          // A model that is both an image2text and speech2text model
          if (!item.options.some((z) => z.value === y.value)) {
            item.options.push(y);
          }
        });
      } else {
        pre.push(x);
      }
    });

    return pre;
  }, []);
};

export const useFetchLlmFactoryList = (): ResponseGetType<IFactory[]> => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [LLMApiAction.FactoryList],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.factoriesList();

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchAvailableProviders = () => {
  const { data, isFetching: loading } = useQuery<IAvailableProvider[]>({
    queryKey: [LLMApiAction.AvailableProviders],
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
    queryKey: [LLMApiAction.AddedProviders],
    initialData: [],
    gcTime: 0,
    queryFn: async () => {
      const { data } = await llmService.listProviders({ params: {} }, true);

      return data?.data ?? [];
    },
  });

  return { data, loading };
};

export const useFetchProviderInstances = (providerName: string) => {
  const { data, isFetching: loading } = useQuery<IProviderInstance[]>({
    queryKey: [LLMApiAction.AddedProviders, providerName, 'instances'],
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
    queryKey: [
      LLMApiAction.AddedProviders,
      providerName,
      instanceName,
      'models',
    ],
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

export const useFetchMyLlmList = (): ResponseGetType<
  Record<string, IMyLlmValue>
> => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [LLMApiAction.MyLlmList],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.myLlm();

      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useFetchMyLlmListDetailed = (): ResponseGetType<
  Record<string, any>
> => {
  const { data, isFetching: loading } = useQuery({
    queryKey: [LLMApiAction.MyLlmListDetailed],
    initialData: {},
    gcTime: 0,
    queryFn: async () => {
      const { data } = await userService.myLlm({ include_details: true });

      return data?.data ?? {};
    },
  });

  return { data, loading };
};

export const useSelectLlmList = () => {
  const { data: myLlmList, loading: myLlmListLoading } = useFetchMyLlmList();
  const { data: factoryList, loading: factoryListLoading } =
    useFetchLlmFactoryList();

  const nextMyLlmList: Array<LlmItem> = useMemo(() => {
    return Object.entries(myLlmList).map(([key, value]) => ({
      name: key,
      logo: factoryList.find((x) => x.name === key)?.logo ?? '',
      ...value,
      llm: value.llm?.map((x) => ({ ...x, name: x.name })),
    }));
  }, [myLlmList, factoryList]);

  const nextFactoryList = useMemo(() => {
    const currentList = factoryList.filter((x) =>
      Object.keys(myLlmList).every((y) => y !== x.name),
    );
    return currentList;
    // return sortLLmFactoryListBySpecifiedOrder(currentList);
  }, [factoryList, myLlmList]);

  return {
    myLlmList: nextMyLlmList,
    factoryList: nextFactoryList,
    loading: myLlmListLoading || factoryListLoading,
  };
};

export interface IApiKeySavingParams {
  instance_name?: string;
  llm_factory: string;
  api_key: string;
  llm_name?: string;
  model_type?: string;
  base_url?: string;
  source_fid?: string;
  verify?: boolean;
}

export const useSaveApiKey = () => {
  const queryClient = useQueryClient();
  // const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.SaveApiKey],
    mutationFn: async (params: IApiKeySavingParams) => {
      const { data } = await userService.setApiKey(params);
      if (data.code === 0) {
        // message.success(t('message.modified'));
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
      }
      return data;
    },
  });

  return { data, loading, saveApiKey: mutateAsync };
};

export interface ISystemModelSettingSavingParams {
  tenant_id: string;
  name?: string;
  asr_id: string;
  embd_id: string;
  img2txt_id: string;
  llm_id: string;
}

export const useSaveTenantInfo = () => {
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.SaveTenantInfo],
    mutationFn: async (params: ISystemModelSettingSavingParams) => {
      const { data } = await userService.setTenantInfo(params);
      if (data.code === 0) {
        message.success(t('message.modified'));
      }
      return data.code;
    },
  });

  return { data, loading, saveTenantInfo: mutateAsync };
};

export const useAddLlm = () => {
  const queryClient = useQueryClient();
  // const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.AddLlm],
    mutationFn: async (params: IAddLlmRequestBody & { verify?: boolean }) => {
      const { data } = await userService.addLlm(params);
      if (data.code === 0 && !params.verify) {
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.LlmList] });
        // message.success(t('message.modified'));
      }
      return data;
    },
  });

  return { data, loading, addLlm: mutateAsync };
};

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
      const { data } = await llmService.addProviderInstance(params);
      if (data.code === 0 && !params.verify) {
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.AddedProviders],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.LlmList] });
      }
      return data;
    },
  });

  return { data, loading, addProviderInstance: mutateAsync };
};

export const useDeleteLlm = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.DeleteLlm],
    mutationFn: async (params: IDeleteLlmRequestBody) => {
      const { data } = await userService.deleteLlm(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
        message.success(t('message.deleted'));
      }
      return data.code;
    },
  });

  return { data, loading, deleteLlm: mutateAsync };
};

export const useEnableLlm = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.EnableLlm],
    mutationFn: async (params: IDeleteLlmRequestBody & { enable: boolean }) => {
      const reqParam: IDeleteLlmRequestBody & {
        enable?: boolean;
        status?: 1 | 0;
      } = { ...params, status: params.enable ? 1 : 0 };
      delete reqParam.enable;
      const { data } = await userService.enableLlm(reqParam);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
        message.success(t('message.modified'));
      }
      return data.code;
    },
  });

  return { data, loading, enableLlm: mutateAsync };
};

export const useDeleteFactory = () => {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const {
    data,
    isPending: loading,
    mutateAsync,
  } = useMutation({
    mutationKey: [LLMApiAction.DeleteFactory],
    mutationFn: async (params: IDeleteLlmRequestBody) => {
      const { data } = await userService.deleteFactory(params);
      if (data.code === 0) {
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.MyLlmList] });
        queryClient.invalidateQueries({
          queryKey: [LLMApiAction.MyLlmListDetailed],
        });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.FactoryList] });
        queryClient.invalidateQueries({ queryKey: [LLMApiAction.LlmList] });
        message.success(t('message.deleted'));
      }
      return data.code;
    },
  });

  return { data, loading, deleteFactory: mutateAsync };
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
          queryKey: [LLMApiAction.AddedProviders],
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
          queryKey: [LLMApiAction.AddedProviders],
        });
      }
      return data;
    },
  });

  return { loading, updateModelStatus: mutateAsync };
};
