import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
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

// The backend expects non-negative integers for these fields. Canvases saved
// by older builds may hold fractional values; round them on load so they are
// displayed and re-saved as integers.
const INTEGER_FIELDS = [
  'message_history_window_size',
  'max_retries',
  'max_rounds',
] as const;

function toIntegerValues(values: Record<string, any>) {
  const next = { ...values };
  for (const key of INTEGER_FIELDS) {
    const v = next[key];
    if (typeof v === 'number' && !Number.isInteger(v)) {
      next[key] = Math.round(v);
    }
  }
  return next;
}

export function useValues(node?: RAGFlowNodeType) {
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const defaultValues = useMemo(
    () => ({
      ...omitToolsAndMcp(initialAgentValues),
      llm_id: defaultModelDictionary.llm_id,
      prompts: '',
    }),
    [defaultModelDictionary],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    return {
      ...toIntegerValues(omitToolsAndMcp(formData)),
      tool_timeout: get(formData, 'tool_timeout', 10),
      prompts: get(formData, 'prompts.0.content', ''),
    };
  }, [defaultValues, node?.data?.form]);

  return values;
}
