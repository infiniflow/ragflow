import { ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/agent';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { initialCodeValues } from '../../constant';

function convertToArray(args: Record<string, string>) {
  return Object.entries(args).map(([key, value]) => ({
    name: key,
    type: value,
  }));
}

type OutputsFormType = { name: string; type: string };

function convertOutputsToArray({ lang, outputs = {} }: ICodeForm) {
  if (lang === ProgrammingLanguage.Python) {
    return Object.entries(outputs).map(([key, val]) => ({
      name: key,
      type: val.type,
    }));
  }
  return Object.entries(outputs).reduce<OutputsFormType>((pre, [key, val]) => {
    pre.name = key;
    pre.type = val.type;
    return pre;
  }, {} as OutputsFormType);
}

export function useValues(node?: RAGFlowNodeType) {
  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return initialCodeValues;
    }

    return {
      ...formData,
      arguments: convertToArray(formData.arguments),
      outputs: convertOutputsToArray(formData),
    };
  }, [node?.data?.form]);

  return values;
}
