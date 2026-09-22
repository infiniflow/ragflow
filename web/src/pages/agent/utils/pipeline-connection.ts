import { Operator } from '@/constants/agent';
import { ChunkerOperators } from '../constant/pipeline';

export function isChunkerOperator(operator: Operator): boolean {
  return (ChunkerOperators as Operator[]).includes(operator);
}

export interface PipelineNextOperators {
  // Operators offered in the flat top-level list (Parser, Tokenizer,
  // Extractor, Compiler), minus the single-instance ones already on canvas.
  operators: Operator[];
  // Single-instance chunkers not yet on canvas, shown in the Chunker group.
  chunkerOperators: Operator[];
  showChunker: boolean;
}

/**
 * Build the "next step" operator menu for a pipeline source node. Mirrors the
 * Go topology rules from `isValidGoPipelineConnection`: on the Go backend a
 * Parser offers only the chunker group, and Extractor/Compiler/chunker sources
 * never offer the chunker group. Single-instance operators are filtered out
 * through `hasOperator`. Pure function so both the menu component and the
 * connection-drag gate can reuse it.
 */
export function buildPipelineNextOperators(
  source: Operator | undefined,
  isGoBackend: boolean,
  hasOperator: (operator: Operator) => boolean,
): PipelineNextOperators {
  const operators: Operator[] = [];

  // Go pipelines require a Parser to feed a chunker, so from a Parser node
  // the menu offers only the chunker group.
  if (!(isGoBackend && source === Operator.Parser)) {
    [Operator.Parser, Operator.Tokenizer].forEach((operator) => {
      if (!hasOperator(operator)) {
        operators.push(operator);
      }
    });
    operators.push(Operator.Extractor);
    if (source !== Operator.Compiler) {
      operators.push(Operator.Compiler);
    }
  }

  const chunkerOperators = ChunkerOperators.filter(
    (operator) => !hasOperator(operator),
  );

  // Go pipelines forbid chunker -> chunker, mirroring the existing rule
  // that Extractor/Compiler never offer the chunker group.
  const sourceExcluded =
    source === Operator.Extractor ||
    source === Operator.Compiler ||
    (isGoBackend && !!source && isChunkerOperator(source));

  return {
    operators,
    chunkerOperators,
    showChunker: !sourceExcluded && chunkerOperators.length > 0,
  };
}

/**
 * Whether the "next step" menu for a source node contains at least one
 * selectable operator. Used by the connection-drag flow to avoid showing an
 * empty dropdown attached to a placeholder node.
 */
export function hasPipelineNextOperators(
  source: Operator | undefined,
  isGoBackend: boolean,
  hasOperator: (operator: Operator) => boolean,
): boolean {
  const { operators, chunkerOperators, showChunker } =
    buildPipelineNextOperators(source, isGoBackend, hasOperator);
  return operators.length > 0 || (showChunker && chunkerOperators.length > 0);
}

/**
 * Go-backend pipeline topology rules, enforced when a connection is created:
 * a Parser may only feed a chunker, and a chunker may not feed another
 * chunker. All other source/target pairs are unrestricted. Pure function so
 * it can be unit-tested without mocking; callers gate on the backend through
 * `useIsGoBackend()`.
 */
export function isValidGoPipelineConnection(
  source: Operator,
  target: Operator,
): boolean {
  if (source === Operator.Parser) {
    return isChunkerOperator(target);
  }
  if (isChunkerOperator(source)) {
    return !isChunkerOperator(target);
  }
  return true;
}
