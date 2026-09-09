import type { ReactNode } from 'react';

type PromptVariableOptionLike = {
  label: string;
  value: string;
  parentLabel?: string | ReactNode;
  icon?: ReactNode;
  type?: string;
};

type PromptVariablePathParts = {
  rootValue: string;
  pathSuffix: string;
};

type PromptVariableLeadingPathMatch = {
  pathSuffix: string;
  remainingText: string;
};

const PromptVariableLeadingPathRegex =
  /^(?<pathSuffix>(?:\.(?:\d+|[A-Za-z_][A-Za-z0-9_]*))+)/;

function splitPromptVariablePath(value: string): PromptVariablePathParts {
  const [nodeId, variable = ''] = value.split('@');

  if (!nodeId || !variable) {
    return { rootValue: value, pathSuffix: '' };
  }

  const dotIndex = variable.indexOf('.');
  if (dotIndex < 0) {
    return { rootValue: value, pathSuffix: '' };
  }

  return {
    rootValue: `${nodeId}@${variable.slice(0, dotIndex)}`,
    pathSuffix: variable.slice(dotIndex),
  };
}

export function extractLeadingPromptVariablePath(
  text: string,
): PromptVariableLeadingPathMatch | undefined {
  const match = PromptVariableLeadingPathRegex.exec(text);
  const pathSuffix = match?.groups?.pathSuffix;

  if (!pathSuffix) {
    return undefined;
  }

  return {
    pathSuffix,
    remainingText: text.slice(pathSuffix.length),
  };
}

export function appendPromptVariablePath(
  option: PromptVariableOptionLike,
  pathSuffix: string,
): PromptVariableOptionLike {
  if (!pathSuffix) {
    return option;
  }

  return {
    ...option,
    value: `${option.value}${pathSuffix}`,
    label: `${option.label}${pathSuffix}`,
  };
}

export function resolvePromptVariableOption(
  value: string,
  options: PromptVariableOptionLike[],
): PromptVariableOptionLike | undefined {
  const exactMatch = options.find((option) => option.value === value);
  if (exactMatch) {
    return exactMatch;
  }

  const { rootValue, pathSuffix } = splitPromptVariablePath(value);
  if (!pathSuffix) {
    return undefined;
  }

  const rootOption = options.find((option) => option.value === rootValue);
  if (!rootOption) {
    return undefined;
  }

  return appendPromptVariablePath(rootOption, pathSuffix);
}
