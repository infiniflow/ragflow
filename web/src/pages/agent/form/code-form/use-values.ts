import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialCodeValues } from '../../constant';

function convertToArray(args: Record<string, string>) {
  return Object.entries(args).map(([key, value]) => ({
    name: key,
    component_id: value,
  }));
}

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialCodeValues;
    }

    return { ...formData, arguments: convertToArray(formData.arguments) };
  }, [node?.data?.form]);

  return values;
}
