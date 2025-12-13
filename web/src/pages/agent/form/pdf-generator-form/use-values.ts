import { useMemo } from 'react';
import { Node } from 'reactflow';
import { initialPDFGeneratorValues } from '../../constant';

export const useValues = (node?: Node) => {
  const values = useMemo(() => {
    return node?.data.form ?? initialPDFGeneratorValues;
  }, [node?.data.form]);

  return values;
};
