import { Operator } from '../../constant';
import { CodeOutputContract } from '../../form/code-form/utils';

const SYSTEM_OUTPUT_NAMES = new Set([
  '_ERROR',
  '_ARTIFACTS',
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
