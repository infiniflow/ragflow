import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty, omit } from 'lodash';
import { useMemo } from 'react';

export function useFormValues(
  defaultValues: Record<string, any>,
  node?: RAGFlowNodeType,
) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return omit(defaultValues, 'outputs');
    }

    return omit(formData, 'outputs');
  }, [defaultValues, node?.data?.form]);

  return values;
}
