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

import { ICompilationTemplateConfig } from '@/interfaces/database/compilation-template';
import { IOntologyCompetencyQuestionDraft } from '@/interfaces/database/ontology-insight';

/**
 * The config section competency questions live in.
 *
 * They are a section of the model rather than a table of their own because they
 * are a statement about the model — "this is what it must be able to answer" —
 * and the version snapshot has to carry them for the publish gate to judge a
 * version by what it promised.
 */
export const CompetencySectionKey = 'competency';

const readSection = (
  config?: ICompilationTemplateConfig,
): IOntologyCompetencyQuestionDraft[] => {
  const section = config?.[CompetencySectionKey];
  const fields = (section as { fields?: unknown } | undefined)?.fields;
  if (!Array.isArray(fields)) {
    return [];
  }
  return fields.flatMap((raw) => {
    if (!raw || typeof raw !== 'object') {
      return [];
    }
    const entry = raw as Record<string, unknown>;
    const type = String(entry.type ?? '').trim();
    const question = String(entry.question ?? '').trim();
    if (!type && !question) {
      return [];
    }
    return [
      {
        type: type || question,
        question,
        elements: String(entry.elements ?? '').trim(),
      },
    ];
  });
};

export const parseCompetencyQuestions = (
  config?: ICompilationTemplateConfig,
): IOntologyCompetencyQuestionDraft[] => readSection(config);

/**
 * Returns the config with its competency section replaced.
 *
 * The whole section is rewritten rather than patched in place: the array is the
 * unit the backend diffs by `type`, and a half-updated section would let two
 * questions disagree about which one owns a `type` value.
 */
export const withCompetencyQuestions = (
  config: ICompilationTemplateConfig | undefined,
  questions: IOntologyCompetencyQuestionDraft[],
): ICompilationTemplateConfig => ({
  ...(config ?? {}),
  [CompetencySectionKey]: {
    fields: questions.map((question) => ({
      type: question.type,
      question: question.question,
      elements: question.elements,
    })),
  },
});

/**
 * Counts the required elements a question writes.
 *
 * The coverage numbers come from the backend, which measures the corpus; this
 * only renders "3 个必需元素" for a question that has not been measured yet. An
 * element prefixed with `?` is optional and is not counted.
 */
export const requiredElementCount = (elements: string): number =>
  elements
    .split('|')
    .map((part) => part.trim())
    .filter((part) => part !== '' && !part.startsWith('?')).length;

/**
 * Suggests the next question id.
 *
 * Ids are the diff key, so they have to be unique and stable. Reusing the
 * highest number keeps a deleted question from being silently resurrected by a
 * later insert that picked the same id.
 */
export const nextCompetencyId = (
  questions: IOntologyCompetencyQuestionDraft[],
): string => {
  let highest = 0;
  for (const question of questions) {
    const match = /^CQ-(\d+)$/i.exec(question.type.trim());
    if (match) {
      highest = Math.max(highest, Number(match[1]));
    }
  }
  return `CQ-${highest + 1}`;
};

/**
 * The element clause grammar, in the one place both the editor and the page read
 * it: `kind:name`, `|` separated, `?` prefix for optional.
 */
export const CompetencyElementGrammar =
  'class:Contract | relation:Contract->Vendor | property:Contract.amount | ?class:Person';
