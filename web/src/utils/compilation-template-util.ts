import { capitalize, lowerCase } from 'lodash';

export function formatKindLabel(kind: string): string {
  return capitalize(lowerCase(kind));
}
