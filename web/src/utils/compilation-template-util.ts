import { CompilationTemplateKind } from '@/constants/compilation';
import { TFunction } from 'i18next';
import { capitalize, lowerCase } from 'lodash';

export function formatKindLabel(t: TFunction, kind: string): string {
  if (kind === CompilationTemplateKind.Artifacts) {
    return 'Wiki';
  }
  if (kind === CompilationTemplateKind.KnowledgeGraph) {
    return t('knowledgeDetails.structureGraph');
  }
  return capitalize(lowerCase(kind));
}
export const isCreateCompilationTemplateGroup = (
  id?: string,
): id is undefined => {
  return !id;
};
