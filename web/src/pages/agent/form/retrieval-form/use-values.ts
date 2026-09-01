import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty, omit } from 'lodash';
import { useMemo } from 'react';
import { initialRetrievalValues } from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const defaultValues = useMemo(
    () => ({
      ...initialRetrievalValues,
    }),
    [],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    return omit(formData, 'top_k');
  }, [defaultValues, node?.data?.form]);

  return values;
}
