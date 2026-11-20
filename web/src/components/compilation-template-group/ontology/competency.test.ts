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

import {
  nextCompetencyId,
  parseCompetencyQuestions,
  requiredElementCount,
  withCompetencyQuestions,
} from './competency';

describe('parseCompetencyQuestions', () => {
  it('reads the section as a field list', () => {
    const config = {
      competency: {
        fields: [
          { type: 'CQ-1', question: '谁供货？', elements: 'class:vendor' },
          { type: 'CQ-2', question: '合同金额？', elements: '' },
        ],
      },
    } as unknown as ICompilationTemplateConfig;

    expect(parseCompetencyQuestions(config)).toEqual([
      { type: 'CQ-1', question: '谁供货？', elements: 'class:vendor' },
      { type: 'CQ-2', question: '合同金额？', elements: '' },
    ]);
  });

  it('falls back to the question text when the id is missing', () => {
    const config = {
      competency: { fields: [{ question: '谁批准了？', elements: '' }] },
    } as unknown as ICompilationTemplateConfig;

    expect(parseCompetencyQuestions(config)[0].type).toBe('谁批准了？');
  });

  it('returns an empty list for a model without the section', () => {
    expect(parseCompetencyQuestions({})).toEqual([]);
    expect(parseCompetencyQuestions(undefined)).toEqual([]);
  });
});

describe('withCompetencyQuestions', () => {
  it('replaces the section and keeps everything else', () => {
    const config = {
      kind: 'ontology',
      entity: { fields: [{ type: 'vendor' }] },
    } as unknown as ICompilationTemplateConfig;

    const next = withCompetencyQuestions(config, [
      { type: 'CQ-1', question: 'q', elements: 'class:vendor' },
    ]);

    expect(next.kind).toBe('ontology');
    expect(next.entity).toEqual({ fields: [{ type: 'vendor' }] });
    expect((next.competency as { fields: unknown[] }).fields).toEqual([
      { type: 'CQ-1', question: 'q', elements: 'class:vendor' },
    ]);
  });

  it('round-trips through the parser', () => {
    const questions = [
      { type: 'CQ-1', question: 'q1', elements: 'class:a' },
      { type: 'CQ-2', question: 'q2', elements: 'class:b' },
    ];
    expect(
      parseCompetencyQuestions(
        withCompetencyQuestions({} as ICompilationTemplateConfig, questions),
      ),
    ).toEqual(questions);
  });
});

describe('requiredElementCount', () => {
  it('counts only required elements', () => {
    expect(requiredElementCount('class:a | ?class:b | relation:a->b')).toBe(2);
    expect(requiredElementCount('')).toBe(0);
  });

  it('accepts one element per line as well as a pipe-separated clause', () => {
    expect(requiredElementCount('class:a\n?class:b\nproperty:a.x')).toBe(2);
  });
});

describe('nextCompetencyId', () => {
  it('continues from the highest existing id', () => {
    expect(
      nextCompetencyId([
        { type: 'CQ-2', question: '', elements: '' },
        { type: 'CQ-7', question: '', elements: '' },
      ]),
    ).toBe('CQ-8');
  });

  it('starts at CQ-1 for an empty model', () => {
    expect(nextCompetencyId([])).toBe('CQ-1');
  });

  it('ignores ids that are not in the CQ-n form', () => {
    expect(
      nextCompetencyId([{ type: 'who-supplies', question: '', elements: '' }]),
    ).toBe('CQ-1');
  });
});
