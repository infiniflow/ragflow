import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import {
  initialStringTransformValues,
  StringTransformMethod,
} from '../../constant';

function transferDelimiters(formData: typeof initialStringTransformValues) {
  return formData.method === StringTransformMethod.Merge
    ? formData.delimiters[0]
    : formData.delimiters;
}

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return {
        ...initialStringTransformValues,
        delimiters: transferDelimiters(formData),
      };
    }

    return {
      ...formData,
      delimiters: transferDelimiters(formData),
    };
  }, [node?.data?.form]);

  return values;
}
