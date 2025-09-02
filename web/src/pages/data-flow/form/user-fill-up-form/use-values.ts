import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialUserFillUpValues } from '../../constant';
import { buildBeginInputListFromObject } from '../begin-form/utils';

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialUserFillUpValues;
    }

    const inputs = buildBeginInputListFromObject(formData?.inputs);

    return { ...(formData || {}), inputs };
  }, [node?.data?.form]);

  return values;
}
