import { useFetchModelId } from '@/hooks/logic-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { get, isEmpty, omit } from 'lodash';
import { useMemo } from 'react';
import { initialAgentValues } from '../../constant';

// You need to exclude the mcp and tools fields that are not in the form,
// otherwise the form data update will reset the tools or mcp data to an array
// Exclude data that is not in the form to avoid writing this data to the canvas when using useWatch.
// Outputs, tools, and MCP data are directly synchronized to the canvas without going through the form.
function omitToolsAndMcp(values: Record<string, any>) {
  return omit(values, ['mcp', 'tools', 'outputs']);
}

export function useValues(node?: RAGFlowNodeType) {
  const llmId = useFetchModelId();

  const defaultValues = useMemo(
    () => ({
      ...omitToolsAndMcp(initialAgentValues),
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
      ...omitToolsAndMcp(formData),
      prompts: get(formData, 'prompts.0.content', ''),
    };
  }, [defaultValues, node?.data?.form]);

  return values;
}
