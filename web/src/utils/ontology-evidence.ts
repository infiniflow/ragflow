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

import { OntologyEvidenceLevel } from '@/interfaces/database/ontology-retrieval';

type Translate = (key: string) => string;

/**
 * Names one evidence grade.
 *
 * An unknown grade is shown as the server spelled it rather than mapped to the
 * nearest known one: a reader who sees a grade this build does not know is
 * looking at a newer server, and inventing a label would hide that.
 */
export function ontologyEvidenceLabel(
  level: string | undefined,
  t: Translate,
): string {
  switch (level) {
    case OntologyEvidenceLevel.Structured:
      return t('chat.ontologyEvidenceStructured');
    case OntologyEvidenceLevel.SchemaFree:
      return t('chat.ontologyEvidenceSchemaFree');
    case OntologyEvidenceLevel.None:
      return t('chat.ontologyEvidenceNone');
    default:
      return level ?? '';
  }
}

/** What the grade means for the rows under it. */
export function ontologyEvidenceHint(
  level: string | undefined,
  t: Translate,
): string {
  switch (level) {
    case OntologyEvidenceLevel.Structured:
      return t('chat.ontologyEvidenceStructuredHint');
    case OntologyEvidenceLevel.SchemaFree:
      return t('chat.ontologyEvidenceSchemaFreeHint');
    case OntologyEvidenceLevel.None:
      return t('chat.ontologyEvidenceNoneHint');
    default:
      return '';
  }
}
