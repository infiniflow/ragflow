import { ModelVariableType } from '@/constants/knowledge';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { get, isEmpty, isPlainObject, omit } from 'lodash';
import { useMemo } from 'react';
import { buildCategorizeListFromObject } from '../../utils';

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
    const items = buildCategorizeListFromObject(
      get(node, 'data.form.category_description', {}),
    );
    if (isPlainObject(formData)) {
      const nextValues = {
        ...omit(formData, 'category_description'),
        items,
      };

      return nextValues;
    }
  }, [node]);

  return values;
}
