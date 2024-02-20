import { LlmModelType } from '@/constants/knowledge';
import { IThirdOAIModelCollection } from '@/interfaces/database/llm';
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
        })),
      };
    });
  }, [llmInfo]);

  return embeddingModelOptions;
};
