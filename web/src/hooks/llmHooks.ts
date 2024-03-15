import { LlmModelType } from '@/constants/knowledge';
import {
  IFactory,
  IMyLlmValue,
  IThirdOAIModelCollection,
} from '@/interfaces/database/llm';
import { useCallback, useEffect, useMemo } from 'react';
import { useDispatch, useSelector } from 'umi';

export const useFetchLlmList = (modelType: LlmModelType) => {
  const dispatch = useDispatch();

  const fetchLlmList = useCallback(() => {
    dispatch({
      type: 'settingModel/llm_list',
      payload: { model_type: modelType },
    });
  }, [dispatch, modelType]);

  useEffect(() => {
    fetchLlmList();
  }, [fetchLlmList]);
};

export const useSelectLlmOptions = () => {
  const llmInfo: IThirdOAIModelCollection = useSelector(
    (state: any) => state.settingModel.llmInfo,
  );

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
  api_base?: string;
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
