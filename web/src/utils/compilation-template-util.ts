import { capitalize, lowerCase } from 'lodash';

export function formatKindLabel(kind: string): string {
  return capitalize(lowerCase(kind));
}
export const isCreateCompilationTemplateGroup = (
  id?: string,
): id is undefined => {
  return !id;
};
