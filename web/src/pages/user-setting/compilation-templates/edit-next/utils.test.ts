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

import { transformDetailToForm, transformTemplateToPayload } from './utils';

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
        { type: 'person', label: 'Person', description: 'A human.', parent: '', rule: '' },
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
