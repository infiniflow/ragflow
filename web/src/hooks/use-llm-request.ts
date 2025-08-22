import { LlmModelType } from '@/constants/knowledge';
import userService from '@/services/user-service';
import { useQuery } from '@tanstack/react-query';

import {
  IThirdOAIModelCollection as IThirdAiModelCollection,
  IThirdOAIModel,
} from '@/interfaces/database/llm';
import { buildLlmUuid } from '@/utils/llm-util';

export const enum LLMApiAction {
  LlmList = 'llmList',
}

export const useFetchLlmList = (modelType?: LlmModelType) => {
  const { data } = useQuery<IThirdAiModelCollection>({
    queryKey: [LLMApiAction.LlmList],
    initialData: {},
    queryFn: async () => {
      const { data } = await userService.llm_list({ model_type: modelType });

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
