import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialQueritValues } from '../../constant';
import { deserializeQueritFormValues } from './utils';

export function useValues(node?: RAGFlowNodeType) {
  return useMemo(() => {
    const formData = node?.data?.form;
    const values = isEmpty(formData) ? initialQueritValues : formData;

    return deserializeQueritFormValues(values);
  }, [node?.data?.form]);
}
