import { LlmModelType } from '@/constants/knowledge';
import {
  IFactory,
  IMyLlmValue,
  IThirdOAIModelCollection,
} from '@/interfaces/database/llm';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchLlmList = (
  modelType?: LlmModelType,
  isOnMountFetching: boolean = true,
) => {
  const dispatch = useDispatch();

  const fetchLlmList = useCallback(() => {
    dispatch({
      type: 'settingModel/llm_list',
      payload: { model_type: modelType },
    });
  }, [dispatch, modelType]);

  useEffect(() => {
    if (isOnMountFetching) {
      fetchLlmList();
    }
  }, [fetchLlmList, isOnMountFetching]);

  return fetchLlmList;
};

export const useSelectLlmInfo = () => {
  const llmInfo: IThirdOAIModelCollection = useSelector(
    (state: any) => state.settingModel.llmInfo,
  );

  return llmInfo;
};

export const useSelectLlmOptions = () => {
  const llmInfo: IThirdOAIModelCollection = useSelectLlmInfo();

  const embeddingModelOptions = useMemo(() => {
    return Object.entries(llmInfo).map(([key, value]) => {
      return {
        label: key,
        options: value.map((x) => ({
          label: x.llm_name,
          value: x.llm_name,
          disabled: !x.available,
        })),
      };
    });
  }, [llmInfo]);

  return embeddingModelOptions;
};

export const useSelectLlmOptionsByModelType = () => {
  const llmInfo: IThirdOAIModelCollection = useSelectLlmInfo();

  const groupOptionsByModelType = (modelType: LlmModelType) => {
    return Object.entries(llmInfo)
      .filter(([, value]) =>
        modelType ? value.some((x) => x.model_type === modelType) : true,
      )
      .map(([key, value]) => {
        return {
          label: key,
          options: value
            .filter((x) => (modelType ? x.model_type === modelType : true))
            .map((x) => ({
              label: x.llm_name,
              value: x.llm_name,
              disabled: !x.available,
            })),
        };
      });
  };

  return {
    [LlmModelType.Chat]: groupOptionsByModelType(LlmModelType.Chat),
    [LlmModelType.Embedding]: groupOptionsByModelType(LlmModelType.Embedding),
    [LlmModelType.Image2text]: groupOptionsByModelType(LlmModelType.Image2text),
    [LlmModelType.Speech2text]: groupOptionsByModelType(
      LlmModelType.Speech2text,
    ),
  };
};

export const useSelectLlmFactoryList = () => {
  const factoryList: IFactory[] = useSelector(
    (state: any) => state.settingModel.factoryList,
  );

  return factoryList;
};

export const useSelectMyLlmList = () => {
  const myLlmList: Record<string, IMyLlmValue> = useSelector(
    (state: any) => state.settingModel.myLlmList,
  );

  return myLlmList;
};

export const useFetchLlmFactoryListOnMount = () => {
  const dispatch = useDispatch();
  const factoryList = useSelectLlmFactoryList();
  const myLlmList = useSelectMyLlmList();

  const list = useMemo(
    () =>
      factoryList.filter((x) =>
        Object.keys(myLlmList).every((y) => y !== x.name),
      ),
    [factoryList, myLlmList],
  );

  const fetchLlmFactoryList = useCallback(() => {
    dispatch({
      type: 'settingModel/factories_list',
    });
  }, [dispatch]);

  useEffect(() => {
    fetchLlmFactoryList();
  }, [fetchLlmFactoryList]);

  return list;
};

export type LlmItem = { name: string; logo: string } & IMyLlmValue;

export const useFetchMyLlmListOnMount = () => {
  const dispatch = useDispatch();
  const llmList = useSelectMyLlmList();
  const factoryList = useSelectLlmFactoryList();

  const list: Array<LlmItem> = useMemo(() => {
    return Object.entries(llmList).map(([key, value]) => ({
      name: key,
      logo: factoryList.find((x) => x.name === key)?.logo ?? '',
      ...value,
    }));
  }, [llmList, factoryList]);

  const fetchMyLlmList = useCallback(() => {
    dispatch({
      type: 'settingModel/my_llm',
    });
  }, [dispatch]);

  useEffect(() => {
    fetchMyLlmList();
  }, [fetchMyLlmList]);

  return list;
};

export interface IApiKeySavingParams {
  llm_factory: string;
  api_key: string;
  llm_name?: string;
  model_type?: string;
  base_url?: string;
}

export const useSaveApiKey = () => {
  const dispatch = useDispatch();

  const saveApiKey = useCallback(
    (savingParams: IApiKeySavingParams) => {
      return dispatch<any>({
        type: 'settingModel/set_api_key',
        payload: savingParams,
      });
    },
    [dispatch],
  );

  return saveApiKey;
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
  const dispatch = useDispatch();

  const saveTenantInfo = useCallback(
    (savingParams: ISystemModelSettingSavingParams) => {
      return dispatch<any>({
        type: 'settingModel/set_tenant_info',
        payload: savingParams,
      });
    },
    [dispatch],
  );

  return saveTenantInfo;
};
