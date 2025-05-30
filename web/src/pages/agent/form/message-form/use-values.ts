import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { convertToObjectArray } from '../../utils';

const defaultValues = {
  content: [],
};

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    return {
      ...formData,
      content: convertToObjectArray(formData.content),
    };
  }, [node]);

  return values;
}
