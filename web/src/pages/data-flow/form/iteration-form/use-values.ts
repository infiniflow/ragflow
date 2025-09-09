import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialIterationValues } from '../../constant';
import { OutputObject } from './interface';

function convertToArray(outputObject: OutputObject) {
  return Object.entries(outputObject).map(([key, value]) => ({
    name: key,
    ref: value.ref,
    type: value.type,
  }));
}

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return { ...initialIterationValues, outputs: [] };
    }

    return { ...formData, outputs: convertToArray(formData.outputs) };
  }, [node?.data?.form]);

  return values;
}
