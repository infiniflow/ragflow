import { Operator, VariableRegex } from '../../constant';
import { buildBeginInputListFromObject } from '../../form/begin-form/utils';
import { CodeOutputContract } from '../../form/code-form/utils';

export function buildDebugInputList(
  inputForm: Record<string, any>,
  configuredValues: Record<string, any> = {},
) {
  return buildBeginInputListFromObject(inputForm).map((input) => {
    const value = configuredValues[input.key] ?? input.value;
    // A single-block test needs concrete runtime inputs, not unresolved templates.
    const unresolved =
      typeof value === 'string' &&
      (value.match(VariableRegex) !== null ||
        value.includes('{%') ||
        /^(?:begin@\S+|[A-Za-z][A-Za-z0-9_]*:[A-Za-z0-9][A-Za-z0-9_-]*@\S+|(?:sys|env)\.\S+)$/.test(
          value,
        ));
    return { ...input, value: unresolved ? undefined : value };
  });
}

const SYSTEM_OUTPUT_NAMES = new Set([
  '_ERROR',
  '_ARTIFACTS',
  'attachments',
  '_ATTACHMENT_CONTENT',
]);

export type GroupedCodeExecDebugOutput = {
  expectedType: string;
  actualType: string;
  rawResult: unknown;
  content: string;
  systemOutputs: Record<string, unknown>;
};

export function groupCodeExecDebugOutput(
  data: Record<string, unknown> | undefined,
  contract: CodeOutputContract | null,
): GroupedCodeExecDebugOutput {
  const businessName = contract?.name ?? '';
  const source = data ?? {};
  const systemOutputs = Object.fromEntries(
    Object.entries(source).filter(([key]) => SYSTEM_OUTPUT_NAMES.has(key)),
  );

  return {
    expectedType: contract?.type ?? '',
    actualType: String(source.actual_type ?? ''),
    rawResult:
      source.raw_result ?? (businessName ? source[businessName] : undefined),
    content: String(source.content ?? ''),
    systemOutputs,
  };
}

export function shouldUseCodeExecDebugLayout(label?: string): boolean {
  return label === Operator.Code;
}
