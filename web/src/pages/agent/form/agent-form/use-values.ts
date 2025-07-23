import { useFetchModelId } from '@/hooks/logic-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { get, isEmpty } from 'lodash';
import { useMemo } from 'react';
import { AgentExceptionMethod, initialAgentValues } from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const llmId = useFetchModelId();

  const defaultValues = useMemo(
    () => ({
      ...initialAgentValues,
      llm_id: llmId,
      prompts: '',
    }),
    [llmId],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    return {
      ...formData,
      prompts: get(formData, 'prompts.0.content', ''),
      exception_method:
        formData.exception_method === null
          ? AgentExceptionMethod.Null
          : formData.exception_method,
    };
  }, [defaultValues, node?.data?.form]);

  return values;
}
