import { useFetchModelId } from '@/hooks/logic-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { get, isEmpty } from 'lodash';
import { useMemo } from 'react';
import { Operator, initialAgentValues } from '../../constant';

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

    return { ...formData, prompts: get(formData, 'prompts.0.content', '') };
  }, [defaultValues, node?.data?.form]);

  return values;
}

function buildOptions(list: string[]) {
  return list.map((x) => ({ label: x, value: x }));
}

export function useToolOptions() {
  const options = useMemo(() => {
    const options = [
      {
        label: 'Search',
        options: buildOptions([
          Operator.Google,
          Operator.Bing,
          Operator.DuckDuckGo,
          Operator.Wikipedia,
          Operator.YahooFinance,
          Operator.PubMed,
          Operator.GoogleScholar,
        ]),
      },
      {
        label: 'Communication',
        options: buildOptions([Operator.Email]),
      },
      {
        label: 'Productivity',
        options: [],
      },
      {
        label: 'Developer',
        options: buildOptions([
          Operator.GitHub,
          Operator.ExeSQL,
          Operator.Invoke,
          Operator.Crawler,
          Operator.Code,
        ]),
      },
    ];

    return options;
  }, []);

  return options;
}
