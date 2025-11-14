import { ModelVariableType } from '@/constants/knowledge';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty, isPlainObject } from 'lodash';
import { useMemo } from 'react';

const defaultValues = {
  parameter: ModelVariableType.Precise,
  message_history_window_size: 1,
  temperatureEnabled: true,
  topPEnabled: true,
  presencePenaltyEnabled: true,
  frequencyPenaltyEnabled: true,
  maxTokensEnabled: true,
  items: [],
};

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;
    if (isEmpty(formData)) {
      return defaultValues;
    }
    if (isPlainObject(formData)) {
      // const nextValues = {
      //   ...omit(formData, 'category_description'),
      //   items,
      // };

      return formData;
    }
  }, [node]);

  return values;
}
