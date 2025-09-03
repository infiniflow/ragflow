import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialSwitchValues } from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;
    if (isEmpty(formData)) {
      return initialSwitchValues;
    }

    return formData;
  }, [node]);

  return values;
}
