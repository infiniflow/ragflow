import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialTavilyValues } from '../../constant';
import { convertToObjectArray } from '../../utils';

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialTavilyValues;
    }

    return {
      ...formData,
      include_domains: convertToObjectArray(formData.include_domains),
      exclude_domains: convertToObjectArray(formData.exclude_domains),
    };
  }, [node?.data?.form]);

  return values;
}
