import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import {
  initialStringTransformValues,
  StringTransformMethod,
} from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialStringTransformValues;
    }

    return {
      ...formData,
      delimiters:
        formData.method === StringTransformMethod.Merge
          ? formData.delimiters[0]
          : formData.delimiters,
    };
  }, [node?.data?.form]);

  return values;
}
