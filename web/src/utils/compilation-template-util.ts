/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { CompilationTemplateKind } from '@/constants/compilation';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { TFunction } from 'i18next';
import { capitalize, lowerCase } from 'lodash';

export function formatKindLabel(t: TFunction, kind: string): string {
  if (kind === CompilationTemplateKind.Artifacts) {
    return 'Wiki';
  }
  if (kind === CompilationTemplateKind.KnowledgeGraph) {
    return t('knowledgeCompilation.graph');
  }
  if (kind === CompilationTemplateKind.Ontology) {
    return t('knowledgeCompilation.ontology', { defaultValue: 'Ontology' });
  }
  if (kind === CompilationTemplateKind.PageIndex) {
    return 'PageIndex';
  }
  if (kind === CompilationTemplateKind.MindMap) {
    return 'Mindmap';
  }
  return capitalize(lowerCase(kind));
}
export const isCreateCompilationTemplateGroup = (
  id?: string,
): id is undefined => {
  return !id;
};

/**
 * The template row inside a group that the model-scoped routes address.
 *
 * A group is the addressable unit (`/compilation-templates/edit-next/:group_id`),
 * but every model-scoped API — actions, executions, ABox rows — is keyed by the
 * template row id. The workspace pages hang off the group's URL and resolve this
 * from the group payload; passing the group id down instead asks those APIs
 * about a model that does not exist.
 *
 * Ontology groups hold exactly one template; the first-template fallback keeps
 * a group whose kind label was written by an older build openable.
 */
export function ontologyTemplateIdOf(
  group?: ICompilationTemplateGroup,
): string {
  const templates = group?.templates ?? [];
  return (
    (
      templates.find(
        (item) => item.kind === CompilationTemplateKind.Ontology,
      ) ?? templates[0]
    )?.id ?? ''
  );
}
