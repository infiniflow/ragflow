import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';

const defaultValues = {
  conditions: [],
};

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;
    if (isEmpty(formData)) {
      return defaultValues;
    }

    return formData;
  }, [node]);

  return values;
}
