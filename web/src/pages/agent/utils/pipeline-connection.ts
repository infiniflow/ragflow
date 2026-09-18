import { Operator } from '@/constants/agent';
import { ChunkerOperators } from '../constant/pipeline';

export function isChunkerOperator(operator: Operator): boolean {
  return (ChunkerOperators as Operator[]).includes(operator);
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
