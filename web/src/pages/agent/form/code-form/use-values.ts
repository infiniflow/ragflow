import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialCodeValues } from '../../constant';
import { buildDefaultCodeOutput, deserializeCodeOutputContract } from './utils';

function convertToArray(args: Record<string, string>) {
  return Object.entries(args).map(([key, value]) => ({
    name: key,
    type: value,
  }));
}

export function useValues(node?: RAGFlowNodeType) {
  const valueState = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return {
        values: {
          ...initialCodeValues,
          arguments: convertToArray(initialCodeValues.arguments),
          output: buildDefaultCodeOutput(),
        },
        legacyOutputs: [],
      };
    }

    const { contract, legacyOutputs } = deserializeCodeOutputContract(formData);

    return {
      values: {
        ...formData,
        arguments: convertToArray(formData.arguments),
        output: contract ?? buildDefaultCodeOutput(),
      },
      legacyOutputs,
    };
  }, [node?.data?.form]);

  return valueState;
}
