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

import {
  buildGroupUpdatePayload,
  sortSectionNames,
  transformDetailToForm,
  transformTemplateToPayload,
} from './utils';

describe('sortSectionNames', () => {
  it('prioritizes known sections and preserves remaining order and duplicates', () => {
    expect(
      sortSectionNames([
        'claim',
        'relation',
        'claim',
        'entity',
        'concept',
        'claim',
      ]),
    ).toEqual(['entity', 'relation', 'claim', 'claim', 'concept', 'claim']);
  });
});

/**
 * The builtin ontology template declares more than the editor models:
 * `config.base_uri` at the top level and `entity.output_fields` inside a section.
 * Both used to be destroyed by a read/write cycle — the form read them as
 * sections and wrote back an empty one — so saving a template silently dropped
 * them.
 */
const ontologyDetail = (): any => ({
  id: 'tpl-1',
  name: 'Ontology',
  description: '',
  kind: 'ontology',
  config: {
    kind: 'ontology',
    base_uri: 'http://example.org/ontology#',
    global_rules: '',
    rechunk: false,
    rechunk_rules: '',
    entity: {
      description: 'The classes.',
      output_fields: [{ name: 'name', shape: 'text', required: true }],
      fields: [
        {
          type: 'person',
          label: 'Person',
          description: 'A human.',
          parent: '',
          rule: '',
        },
      ],
    },
    relation: {
      fields: [
        {
          type: 'birth_date',
          kind: 'datatype',
          domain: 'person',
          range: '',
          datatype: 'date',
          label: 'birth date',
          description: 'When someone was born.',
          rule: '',
        },
      ],
    },
  },
});

describe('compilation template round trip', () => {
  it('keeps base_uri, which is not a section', () => {
    const payload = transformTemplateToPayload(
      transformDetailToForm(ontologyDetail()),
    );
    expect((payload.config as any).base_uri).toBe(
      'http://example.org/ontology#',
    );
  });

  it('keeps a section key the field editor does not model', () => {
    const payload = transformTemplateToPayload(
      transformDetailToForm(ontologyDetail()),
    );
    expect((payload.config as any).entity.output_fields).toEqual([
      { name: 'name', shape: 'text', required: true },
    ]);
  });

  it('keeps the ontology sections themselves', () => {
    const payload = transformTemplateToPayload(
      transformDetailToForm(ontologyDetail()),
    );
    expect((payload.config as any).entity.fields).toHaveLength(1);
    expect((payload.config as any).relation.fields[0].datatype).toBe('date');
  });
});

describe('buildGroupUpdatePayload', () => {
  const templates = [
    {
      id: 'tpl-1',
      name: 'model',
      description: 'the ontology',
      kind: 'ontology',
      config: { kind: 'ontology', entity: { fields: [] } },
    },
    {
      id: 'tpl-2',
      name: 'chunks',
      description: '',
      kind: 'text',
      config: { kind: 'text' },
    },
  ] as never;

  // The group endpoint validates every child as a complete template, so dropping
  // the name here is what produced "102 invalid template name" on the drift fixes.
  it('keeps every child name and description', () => {
    const payload = buildGroupUpdatePayload(
      templates,
      'tpl-1',
      (config) => config,
    );
    expect(payload.templates?.map((item) => item.name)).toEqual([
      'model',
      'chunks',
    ]);
    expect(payload.templates?.[0].description).toBe('the ontology');
    expect(payload.templates?.map((item) => item.id)).toEqual([
      'tpl-1',
      'tpl-2',
    ]);
  });

  it('patches only the target template and leaves the others untouched', () => {
    const payload = buildGroupUpdatePayload(templates, 'tpl-1', (config) => ({
      ...config,
      entity: { fields: [{ type: 'invoice' }] },
    }));
    expect(payload.templates?.[0].config.entity).toEqual({
      fields: [{ type: 'invoice' }],
    });
    expect(payload.templates?.[1].config).toEqual({ kind: 'text' });
  });
});
