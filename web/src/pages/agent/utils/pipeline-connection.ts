import { Operator } from '@/constants/agent';
import { ChunkerOperators, SingleOperators } from '../constant/pipeline';

export function isChunkerOperator(operator: Operator): boolean {
  return (ChunkerOperators as Operator[]).includes(operator);
}

/**
 * Operators that may appear at most once on a pipeline canvas. Duplicating a
 * node with such an operator always exceeds the limit, so the copy action is
 * disabled for them outright.
 */
export function isSingleInstanceOperator(operator: Operator): boolean {
  return SingleOperators.includes(operator);
}

export interface PipelineNextOperators {
  // Allowed next operators, minus operators already on canvas.
  operators: Operator[];
}

const pipelineNextOperators: Partial<Record<Operator, Operator[]>> = {
  [Operator.File]: [Operator.Parser],
  [Operator.Parser]: ChunkerOperators,
  [Operator.TokenChunker]: [
    Operator.Extractor,
    Operator.Compiler,
    Operator.Tokenizer,
  ],
  [Operator.TitleChunker]: [
    Operator.Extractor,
    Operator.Compiler,
    Operator.Tokenizer,
  ],
  [Operator.Extractor]: [Operator.Compiler, Operator.Tokenizer],
  [Operator.Compiler]: [Operator.Tokenizer],
};

/**
 * Build the "next step" operator menu from the pipeline's component order.
 * Single-instance operators already on the canvas are filtered out.
 */
export function buildPipelineNextOperators(
  source: Operator | undefined,
  hasOperator: (operator: Operator) => boolean,
): PipelineNextOperators {
  const allowedOperators = source
    ? (pipelineNextOperators[source] ?? [])
    : [Operator.Parser];
  const chunkerSlotTaken = ChunkerOperators.some(hasOperator);
  const operators = allowedOperators.filter(
    (operator) =>
      !hasOperator(operator) &&
      !(isChunkerOperator(operator) && chunkerSlotTaken),
  );

  return {
    operators,
  };
}

/**
 * Whether the "next step" menu for a source node contains at least one
 * selectable operator. Used by the connection-drag flow to avoid showing an
 * empty dropdown attached to a placeholder node.
 */
export function hasPipelineNextOperators(
  source: Operator | undefined,
  hasOperator: (operator: Operator) => boolean,
): boolean {
  return buildPipelineNextOperators(source, hasOperator).operators.length > 0;
}

/**
 * Pipeline topology rules, enforced when a connection is created.
 */
export function isValidPipelineConnection(
  source: Operator,
  target: Operator,
): boolean {
  return (pipelineNextOperators[source] ?? []).includes(target);
}
