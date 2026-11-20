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
  parseCompetencyElements,
  parseCompetencyQuestions,
  requiredElementCount,
  suggestCompetencyElements,
  toggleCompetencyElement,
  traceCompetencyElements,
  vocabularyOf,
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

describe('parseCompetencyElements', () => {
  it('splits on both separators and keeps the optional flag', () => {
    expect(
      parseCompetencyElements(
        'class:contract | relation:contract->vendor\nproperty:contract.amount | ?class:person',
      ),
    ).toEqual([
      { kind: 'class', name: 'contract', optional: false },
      { kind: 'relation', name: 'contract->vendor', optional: false },
      { kind: 'property', name: 'contract.amount', optional: false },
      { kind: 'class', name: 'person', optional: true },
    ]);
  });

  it('drops a clause with no kind or an unknown kind', () => {
    expect(parseCompetencyElements('contract | metric:x | class:')).toEqual([]);
  });

  it('is the parser requiredElementCount counts through', () => {
    // One parser, two consumers: a count that disagreed with the rendered list
    // would show "3 required elements" above two rows.
    const elements = 'class:a | ?class:b | property:a.x | nonsense';
    expect(parseCompetencyElements(elements)).toHaveLength(3);
    expect(requiredElementCount(elements)).toBe(2);
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
describe('vocabularyOf', () => {
  const config = {
    entity: {
      fields: [
        { type: 'artifact', label: 'Artifact', aliases: ['item'] },
        { type: 'person', label: '人', aliases: [] },
      ],
    },
    relation: {
      fields: [
        { type: 'purpose', kind: 'datatype', domain: 'artifact', range: '' },
        {
          type: 'created_by',
          kind: 'object',
          domain: 'artifact',
          range: 'person|organization',
        },
      ],
    },
  } as unknown as ICompilationTemplateConfig;

  it('collects every surface form a declaration answers to', () => {
    const vocabulary = vocabularyOf(config);
    expect(vocabulary.classes[0]).toEqual({
      type: 'artifact',
      terms: ['artifact', 'Artifact', 'item'],
    });
    expect(vocabulary.properties[1]).toMatchObject({
      type: 'created_by',
      object: true,
      domains: ['artifact'],
      ranges: ['person', 'organization'],
    });
  });

  it('treats a range-less property without a kind as a datatype', () => {
    const vocabulary = vocabularyOf(config);
    expect(vocabulary.properties[0]).toMatchObject({
      type: 'purpose',
      object: false,
    });
  });
});

describe('traceCompetencyElements', () => {
  const vocabulary = vocabularyOf({
    entity: {
      fields: [
        { type: 'artifact', label: 'Artifact', aliases: ['item'] },
        { type: 'person', label: '人', aliases: ['someone'] },
        { type: 'taxon', label: '物种', aliases: [] },
      ],
    },
    relation: {
      fields: [
        { type: 'purpose', kind: 'datatype', domain: 'artifact' },
        { type: 'material', kind: 'datatype', domain: 'artifact' },
        { type: 'occupation', kind: 'datatype', domain: 'person' },
        {
          type: 'created_by',
          kind: 'object',
          domain: 'artifact',
          range: 'person',
        },
      ],
    },
  } as unknown as ICompilationTemplateConfig);

  it('traces classes, attributes and declared edges from the question', () => {
    const result = traceCompetencyElements(
      'Which artifacts state a purpose and a material, and which person created them?',
      vocabulary,
    );
    expect(result.classes).toEqual(['artifact', 'person']);
    expect(result.properties).toEqual([
      'property:artifact.purpose',
      'property:artifact.material',
    ]);
    // Only declared pairs become edges: the fixture has no person->artifact
    // property, so the reverse direction is not suggested.
    expect(result.relations).toEqual(['relation:artifact->person']);
    expect(result.elements.split(' | ')).toEqual([
      'class:artifact',
      'class:person',
      'property:artifact.purpose',
      'property:artifact.material',
      'relation:artifact->person',
    ]);
  });

  it('matches CJK labels as substrings and keeps unmatched words', () => {
    const result = traceCompetencyElements(
      '这个物种属于什么分类阶元？还有 habitat 吗',
      vocabulary,
    );
    expect(result.classes).toEqual(['taxon']);
    expect(result.unmatched).toEqual(['habitat']);
  });

  it('says nothing when the question names no class', () => {
    const result = traceCompetencyElements('有多少？', vocabulary);
    expect(result.elements).toBe('');
  });

  it('does not attach an attribute to a class the question never names', () => {
    const result = traceCompetencyElements('occupation', vocabulary);
    expect(result.properties).toEqual([]);
  });
});

// A declaration spelling is not a prose spelling: `born_in` is written "born in",
// and matching the raw string left most underscored properties unmatchable.
const proseVocabulary: any = {
  classes: [
    { type: 'person', terms: ['person'] },
    { type: 'place', terms: ['place'] },
  ],
  properties: [
    {
      type: 'born_in',
      object: true,
      terms: ['born_in', 'birthplace'],
      domains: ['person'],
      ranges: ['place'],
    },
  ],
};

describe('traceCompetencyElements across declaration spelling', () => {
  it('matches an underscored property written as prose', () => {
    const result = traceCompetencyElements(
      'Where was this person born in?',
      proseVocabulary,
    );
    // Only the near end is named; the far end comes from the declaration.
    expect(result.classes).toEqual(['person']);
    expect(result.relations).toEqual(['relation:person->place']);
  });

  it('matches the same property through its label', () => {
    const result = traceCompetencyElements(
      'What is the birthplace of this person?',
      proseVocabulary,
    );
    expect(result.relations).toEqual(['relation:person->place']);
  });
});

describe('suggestCompetencyElements', () => {
  it('offers declared elements even when the question names none', () => {
    const rows = suggestCompetencyElements(
      '这批货物的仓储温度是多少？',
      proseVocabulary,
    );
    expect(rows.map((row) => row.clause)).toEqual(
      expect.arrayContaining([
        'class:person',
        'class:place',
        'relation:person->place',
      ]),
    );
    expect(rows.every((row) => !row.matched)).toBe(true);
  });

  it('orders the elements the question names first', () => {
    const rows = suggestCompetencyElements(
      'Where was this person born in?',
      proseVocabulary,
    );
    expect(rows[0].matched).toBe(true);
    expect(
      rows.find((row) => row.clause === 'relation:person->place')!.matched,
    ).toBe(true);
  });
});

describe('toggleCompetencyElement', () => {
  it('adds a clause once and removes it on the second click', () => {
    expect(toggleCompetencyElement('', 'class:person')).toBe('class:person');
    expect(toggleCompetencyElement('class:person', 'class:person')).toBe('');
    expect(
      toggleCompetencyElement('class:person | class:place', 'class:person'),
    ).toBe('class:place');
  });

  it('keeps an optional marker distinct from the clause itself', () => {
    expect(toggleCompetencyElement('?class:person', 'class:person')).toBe('');
  });
});
