import { ICodeForm } from '@/interfaces/database/agent';

export type CodeOutputContract = {
  name: string;
  type: string;
};

type DeserializeCodeOutputResult = {
  contract: CodeOutputContract | null;
  legacyOutputs: string[];
};

const CodeExecReservedOutputKeys = [
  'content',
  'actual_type',
  'raw_result',
  '_ERROR',
  '_ARTIFACTS',
  '_ATTACHMENT_CONTENT',
  '_created_time',
  '_elapsed_time',
] as const;

export const CodeExecPanelSystemOutputs: ICodeForm['outputs'] = {
  content: {
    type: 'String',
    value: '',
  },
  actual_type: {
    type: 'String',
    value: '',
  },
};

const CodeExecReservedOutputKeySet = new Set<string>(
  CodeExecReservedOutputKeys,
);

export function buildDefaultCodeOutput(): CodeOutputContract {
  return {
    name: 'result',
    type: 'String',
  };
}

export function isValidCodeOutputName(name: string): boolean {
  const value = name.trim();
  return (
    !!value && !CodeExecReservedOutputKeySet.has(value) && !value.includes('.')
  );
}

export function getBusinessOutputs(
  outputs: ICodeForm['outputs'] = {},
): ICodeForm['outputs'] {
  return Object.entries(outputs).reduce<ICodeForm['outputs']>((next, entry) => {
    const [name, value] = entry;

    if (!CodeExecReservedOutputKeySet.has(name)) {
      next[name] = value;
    }

    return next;
  }, {});
}

export function deserializeCodeOutputContract(
  form?: Pick<ICodeForm, 'outputs'> | null,
): DeserializeCodeOutputResult {
  const outputs = form?.outputs ?? {};
  const businessOutputs = Object.entries(getBusinessOutputs(outputs));

  if (businessOutputs.length === 0) {
    return { contract: buildDefaultCodeOutput(), legacyOutputs: [] };
  }

  if (businessOutputs.length > 1) {
    return {
      contract: null,
      legacyOutputs: businessOutputs.map(([name]) => name),
    };
  }

  const [name, output] = businessOutputs[0];

  return {
    contract: {
      name,
      type: output.type,
    },
    legacyOutputs: [],
  };
}

export function hasLegacyMultiOutputs(
  outputs: ICodeForm['outputs'] = {},
): boolean {
  return Object.keys(getBusinessOutputs(outputs)).length > 1;
}

export function serializeCodeOutputContract(
  contract: CodeOutputContract | null,
): ICodeForm['outputs'] {
  const name = contract?.name?.trim();
  const type = contract?.type?.trim();

  if (!name || !type || !isValidCodeOutputName(name)) {
    return {};
  }

  return {
    [name]: {
      type,
      value: null,
    },
  };
}
