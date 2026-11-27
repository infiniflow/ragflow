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
  withDeclaredAttribute,
  withDeclaredClass,
  withDeclaredClassAndAttributes,
  withClassDescription,
  withPropertyNotes,
  withoutDeclaredClass,
  withoutDeclaredProperty,
} from './declaration';

const configOf = (
  overrides: Record<string, unknown> = {},
): ICompilationTemplateConfig =>
  ({
    kind: 'ontology',
    entity: { fields: [{ type: 'contract', label: '合同' }] },
    ...overrides,
  }) as unknown as ICompilationTemplateConfig;

describe('withDeclaredClass', () => {
  it('appends the observed class to the declaration', () => {
    const next = withDeclaredClass(configOf(), 'invoice');
    expect((next.entity as { fields: unknown[] }).fields).toEqual([
      { type: 'contract', label: '合同' },
      { type: 'invoice', label: 'invoice' },
    ]);
  });

  it('leaves an already declared class untouched', () => {
    const config = configOf();
    expect(withDeclaredClass(config, 'contract')).toBe(config);
    expect(withDeclaredClass(config, '  contract  ')).toBe(config);
  });

  it('keeps the rest of the entity section', () => {
    const next = withDeclaredClass(
      configOf({ entity: { description: '主体', fields: [] } }),
      'invoice',
    );
    expect((next.entity as { description?: string }).description).toBe('主体');
  });

  it('is a no-op for a blank name', () => {
    const config = configOf();
    expect(withDeclaredClass(config, '   ')).toBe(config);
  });
});

describe('withDeclaredAttribute', () => {
  it('adds a datatype property on the owning class', () => {
    const next = withDeclaredAttribute(configOf(), 'contract', 'due_date');
    expect((next.relation as { fields: unknown[] }).fields).toEqual([
      {
        type: 'due_date',
        label: 'due_date',
        domain: 'contract',
        datatype: 'string',
      },
    ]);
  });

  it('does not duplicate the same class.field pair', () => {
    const config = withDeclaredAttribute(configOf(), 'contract', 'due_date');
    expect(withDeclaredAttribute(config, 'contract', 'due_date')).toBe(config);
  });

  it('allows the same field name on another class', () => {
    const config = withDeclaredAttribute(configOf(), 'contract', 'due_date');
    const next = withDeclaredAttribute(config, 'invoice', 'due_date');
    expect((next.relation as { fields: unknown[] }).fields).toHaveLength(2);
  });
});

describe('withDeclaredClassAndAttributes', () => {
  it('declares the class and the attributes the corpus already fills', () => {
    const next = withDeclaredClassAndAttributes(configOf(), 'invoice', [
      'due_date',
      'amount',
    ]);
    expect((next.entity as { fields: unknown[] }).fields).toEqual([
      { type: 'contract', label: '合同' },
      { type: 'invoice', label: 'invoice' },
    ]);
    expect((next.relation as { fields: unknown[] }).fields).toEqual([
      {
        type: 'due_date',
        label: 'due_date',
        domain: 'invoice',
        datatype: 'string',
      },
      {
        type: 'amount',
        label: 'amount',
        domain: 'invoice',
        datatype: 'string',
      },
    ]);
  });

  it('skips attributes that are already declared for the class', () => {
    const config = withDeclaredAttribute(configOf(), 'invoice', 'due_date');
    const next = withDeclaredClassAndAttributes(config, 'invoice', [
      'due_date',
      'amount',
    ]);
    expect((next.relation as { fields: unknown[] }).fields).toHaveLength(2);
  });

  it('declares only the class when there is nothing observed', () => {
    const next = withDeclaredClassAndAttributes(configOf(), 'invoice', []);
    expect(next.relation).toBeUndefined();
  });
});

describe('withoutDeclaredClass', () => {
  const withRelations = () =>
    configOf({
      entity: {
        fields: [
          { type: 'contract', label: '合同' },
          { type: 'invoice', label: 'invoice' },
        ],
      },
      relation: {
        fields: [
          { type: 'belongs_to', domain: 'contract|invoice', range: 'party' },
          { type: 'located_in', domain: 'party', range: 'place' },
        ],
      },
    });

  it('removes the class and clears the endpoints that pointed at it', () => {
    const next = withoutDeclaredClass(withRelations(), 'invoice');
    expect((next.entity as { fields: unknown[] }).fields).toEqual([
      { type: 'contract', label: '合同' },
    ]);
    expect((next.relation as { fields: { domain?: string }[] }).fields).toEqual(
      [
        { type: 'belongs_to', domain: 'contract', range: 'party' },
        { type: 'located_in', domain: 'party', range: 'place' },
      ],
    );
  });

  it('keeps the other sections untouched', () => {
    const next = withoutDeclaredClass(
      configOf({ relation: { description: '关系' }, entity: { fields: [] } }),
      'contract',
    );
    expect((next.relation as { description?: string }).description).toBe(
      '关系',
    );
  });

  it('is a no-op for an unknown class', () => {
    const config = withRelations();
    expect(withoutDeclaredClass(config, 'nope')).toBe(config);
  });
});

describe('withoutDeclaredProperty', () => {
  it('removes only the named relation', () => {
    const config = configOf({
      relation: {
        fields: [
          { type: 'belongs_to', domain: 'contract', range: 'party' },
          { type: 'located_in', domain: 'party', range: 'place' },
        ],
      },
    });
    const next = withoutDeclaredProperty(config, 'belongs_to');
    expect((next.relation as { fields: unknown[] }).fields).toEqual([
      { type: 'located_in', domain: 'party', range: 'place' },
    ]);
  });

  it('is a no-op for an unknown property', () => {
    const config = configOf({
      relation: { fields: [{ type: 'belongs_to' }] },
    });
    expect(withoutDeclaredProperty(config, 'nope')).toBe(config);
  });
});

describe('withPropertyNotes', () => {
  it('writes the description and the value set on the property', () => {
    const config = configOf({
      relation: {
        fields: [
          { type: 'status', domain: 'contract', datatype: 'string' },
          { type: 'amount', domain: 'contract', datatype: 'number' },
        ],
      },
    });
    const next = withPropertyNotes(config, 'status', {
      description: '合同当前状态',
      enumValues: 'draft,signed',
    });
    expect((next.relation as { fields: unknown[] }).fields).toEqual([
      {
        type: 'status',
        domain: 'contract',
        datatype: 'string',
        description: '合同当前状态',
        enum: 'draft,signed',
      },
      { type: 'amount', domain: 'contract', datatype: 'number' },
    ]);
  });

  it('leaves a key alone when the caller does not pass it', () => {
    const config = configOf({
      relation: { fields: [{ type: 'status', description: '旧说明' }] },
    });
    const next = withPropertyNotes(config, 'status', {});
    expect(
      (next.relation as { fields: { description?: string }[] }).fields[0]
        .description,
    ).toBe('旧说明');
  });

  it('is a no-op for an unknown property', () => {
    const config = configOf({ relation: { fields: [{ type: 'status' }] } });
    expect(withPropertyNotes(config, 'nope', { description: 'x' })).toBe(
      config,
    );
  });
});

describe('withClassDescription', () => {
  it('writes the description that tells extraction what to look for', () => {
    const next = withClassDescription(
      configOf(),
      'contract',
      '采购或销售合同的主体文档',
    );
    expect(
      (next.entity as { fields: Record<string, unknown>[] }).fields[0],
    ).toEqual({
      type: 'contract',
      label: '合同',
      description: '采购或销售合同的主体文档',
    });
  });

  it('is a no-op for an unknown class', () => {
    const config = configOf();
    expect(withClassDescription(config, 'nope', 'x')).toBe(config);
  });
});
