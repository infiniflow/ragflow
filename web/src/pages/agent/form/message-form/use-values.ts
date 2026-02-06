import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialMessageValues } from '../../constant';
import { convertToObjectArray } from '../../utils';

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialMessageValues;
    }

    return {
      ...formData,
      content: convertToObjectArray(formData.content),
    };
  }, [node]);

  return values;
}
