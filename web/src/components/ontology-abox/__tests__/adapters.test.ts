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

import type { IOntologyGraphPage } from '@/interfaces/database/document-structure';

import {
  classColor,
  classGraphFrom,
  egoSubgraph,
  instanceGraphFrom,
  propertyOptionsFrom,
} from '../adapters';

describe('classColor', () => {
  it('is stable for the same class and differs across classes', () => {
    expect(classColor('bank_card')).toBe(classColor('bank_card'));
    expect(classColor('bank_card')).not.toBe(classColor('customer'));
  });

  it('falls back to the neutral grey without a class', () => {
    expect(classColor(undefined)).toBe('#cbd5e1');
    expect(classColor('')).toBe('#cbd5e1');
  });
});

describe('propertyOptionsFrom', () => {
  it('offers a polymorphic property once, keeping the first declaration label', () => {
    const options = propertyOptionsFrom([
      {
        type: 'resides_in',
        label: 'Resides in',
        source: 'person',
        target: 'place',
      },
      {
        type: 'resides_in',
        label: 'resides in',
        source: 'company',
        target: 'place',
      },
      {
        type: 'founded_by',
        label: 'Founded by',
        source: 'company',
        target: 'person',
      },
      {
        type: 'founded_by',
        label: 'Founded by',
        source: 'university',
        target: 'person',
      },
    ]);
    expect(options).toEqual([
      { label: 'Resides in', value: 'resides_in' },
      { label: 'Founded by', value: 'founded_by' },
    ]);
  });

  it('folds case and padding so the same property cannot appear twice', () => {
    const options = propertyOptionsFrom([
      { type: 'Resides_In', source: 'person', target: 'place' },
      { type: ' resides_in ', source: 'company', target: 'place' },
    ]);
    expect(options).toEqual([{ label: 'Resides_In', value: 'Resides_In' }]);
  });

  it('drops an entry with no property name', () => {
    expect(
      propertyOptionsFrom([{ type: '  ', source: 'a', target: 'b' }]),
    ).toEqual([]);
  });
});

describe('classGraphFrom', () => {
  it('sizes a bubble by its instance count and drops edges to unknown classes', () => {
    const graph = classGraphFrom(
      [
        { type: 'bank_card', label: '银行卡', entities: 268 },
        { type: 'customer', label: '客户', entities: 312 },
      ],
      [
        {
          type: 'held_by',
          label: '持有',
          source: 'bank_card',
          target: 'customer',
        },
        { type: 'mentions', source: 'bank_card', target: 'ghost' },
      ],
    );

    expect(graph.entities.map((item) => item.weight)).toEqual([268, 312]);
    expect(graph.entities[0].name).toBe('银行卡');
    expect(graph.relations).toEqual([
      { from: 'bank_card', to: 'customer', type: '持有' },
    ]);
  });

  it('keeps a class the caller only knows by name', () => {
    const graph = classGraphFrom([{ type: 'bank_card' }], []);
    expect(graph.entities).toHaveLength(1);
    expect(graph.entities[0].weight).toBe(0);
  });
});

describe('instanceGraphFrom', () => {
  const page: IOntologyGraphPage = {
    class: 'bank_card',
    hop: 1,
    total: 268,
    drawn: 2,
    truncated: true,
    classes: ['bank_card', 'customer'],
    entities: [
      {
        name: '卡 6222',
        type: 'bank_card',
        document_ids: ['d1', 'd2'],
      },
      { name: '张伟', type: 'customer', mention_count: 4 },
    ],
    relations: [
      {
        from: '卡 6222',
        to: '张伟',
        type: 'held_by',
        source_type: 'bank_card',
        target_type: 'customer',
      },
      // The endpoint did not make the node budget: drawing this edge would draw
      // a line to a node that is not on the canvas.
      { from: '卡 6222', to: '不在图里', type: 'held_by' },
    ],
  };

  it('maps endpoints through the drawn node set', () => {
    const graph = instanceGraphFrom(page);
    expect(graph?.entities).toHaveLength(2);
    expect(graph?.relations).toHaveLength(1);
    expect(graph?.relations?.[0].type).toBe('held_by');
  });

  it('reports the budget it spent next to the scope it came from', () => {
    const graph = instanceGraphFrom(page);
    expect(graph?.total_entities).toBe(268);
    expect(graph?.returned_entities).toBe(2);
  });

  it('sizes a node by source documents, then mentions', () => {
    const graph = instanceGraphFrom(page);
    expect(graph?.entities[0].weight).toBe(2);
    expect(graph?.entities[1].weight).toBe(4);
  });

  it('returns nothing while the query is still empty', () => {
    expect(instanceGraphFrom(undefined)).toBeUndefined();
  });
});

describe('egoSubgraph', () => {
  const graph = {
    entities: [
      {
        slug: 'a',
        name: 'A',
        aliases: [],
        description: '',
        type: 't1',
        weight: 1,
      },
      {
        slug: 'b',
        name: 'B',
        aliases: [],
        description: '',
        type: 't2',
        weight: 1,
      },
      {
        slug: 'c',
        name: 'C',
        aliases: [],
        description: '',
        type: 't2',
        weight: 1,
      },
      {
        slug: 'd',
        name: 'D',
        aliases: [],
        description: '',
        type: 't3',
        weight: 1,
      },
    ],
    relations: [
      { from: 'a', to: 'b', type: 'r' },
      { from: 'c', to: 'a', type: 'r' },
      // Between two neighbours: kept, because "who else knows B" is the reason
      // to open one node in the first place.
      { from: 'b', to: 'c', type: 'r' },
      { from: 'd', to: 'b', type: 'r' },
    ],
  };

  it('keeps the focus, its neighbours, and the ties among them', () => {
    const ego = egoSubgraph(graph, 'a');
    expect(ego.entities.map((item) => item.slug).sort()).toEqual([
      'a',
      'b',
      'c',
    ]);
    expect(ego.relations).toHaveLength(3);
    // D is two hops away, so both it and its edge are gone.
    expect(ego.entities.some((item) => item.slug === 'd')).toBe(false);
  });

  it('passes the graph through when nothing is selected', () => {
    expect(egoSubgraph(graph, undefined)).toBe(graph);
  });
});
