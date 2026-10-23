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
  OntologyClassKeys,
  OntologyPropertyKeys,
  addClass,
  addProperty,
  draftFromSections,
  effectiveKind,
  removeClass,
  removeProperty,
  renameClass,
  sectionsFromDraft,
  splitTypeList,
  validateDraft,
  type OntologyDraft,
} from './model';

const draft = (): OntologyDraft =>
  draftFromSections(
    {
      fields: [
        {
          type: 'agent',
          label: 'Agent',
          description: 'Anything that acts.',
          parent: '',
          rule: '',
        },
        {
          type: 'person',
          label: 'Person',
          description: 'A human.',
          parent: 'agent',
          rule: '',
        },
        {
          type: 'place',
          label: 'Place',
          description: 'Somewhere.',
          parent: '',
          rule: '',
        },
      ],
    },
    {
      fields: [
        {
          type: 'born_in',
          kind: 'object',
          domain: 'person',
          range: 'place',
          datatype: '',
          label: 'born in',
          description: 'Where someone was born.',
          rule: '',
        },
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
  );

const codes = (d: OntologyDraft) => validateDraft(d).map((issue) => issue.code);

describe('ontology editor model', () => {
  // The one guarantee that matters: whatever the canvas does, the two sections it
  // saves carry the template's key sets, in the template's order.
  it('writes fields with the canonical key order', () => {
    let d = addClass(draft());
    d = addProperty(d, 'object');
    d = addProperty(d, 'datatype');
    const sections = sectionsFromDraft(d);

    expect(Object.keys(sections.entity.fields![3])).toEqual([
      ...OntologyClassKeys,
    ]);
    expect(Object.keys(sections.relation.fields![2])).toEqual([
      ...OntologyPropertyKeys,
    ]);
    expect(Object.keys(sections.relation.fields![3])).toEqual([
      ...OntologyPropertyKeys,
    ]);
  });

  it('fills a missing key with an empty string instead of dropping it', () => {
    const d = draftFromSections(
      { fields: [{ type: 'person', description: 'A human.' }] },
      undefined,
    );
    const [field] = sectionsFromDraft(d).entity.fields!;
    expect(field.parent).toBe('');
    expect(field.label).toBe('');
  });

  // A template may have been authored by hand (or by a newer version): dropping
  // what this editor does not model would be data loss on save.
  it('keeps keys the editor does not model', () => {
    const d = draftFromSections(
      {
        fields: [
          { type: 'person', description: 'A human.', emoji: '🧑', parent: '' },
        ],
      },
      undefined,
    );
    expect(sectionsFromDraft(d).entity.fields![0].emoji).toBe('🧑');
  });

  it('survives a read/write round trip unchanged', () => {
    const first = draft();
    const again = draftFromSections(
      sectionsFromDraft(first).entity,
      sectionsFromDraft(first).relation,
    );
    expect(again).toEqual(first);
  });

  it('mirrors the service: names, references and datatypes', () => {
    expect(validateDraft(draft())).toEqual([]);

    // A parent that is not a declared class.
    let d = draft();
    d = { ...d, classes: d.classes.map((c) => (c.type === 'person' ? { ...c, parent: 'nobody' } : c)) };
    expect(codes(d)).toContain('unknown_parent');

    // A cyclic parent chain would never terminate the inherited-attribute walk.
    d = draft();
    d = { ...d, classes: d.classes.map((c) => ({ ...c, parent: c.type === 'agent' ? 'person' : c.parent })) };
    expect(codes(d)).toContain('cyclic_parent');

    // A range that is not a class.
    d = draft();
    d = {
      ...d,
      properties: d.properties.map((p) => (p.type === 'born_in' ? { ...p, range: 'nowhere' } : p)),
    };
    expect(codes(d)).toContain('unknown_range');

    // A datatype outside the service's whitelist.
    d = draft();
    d = {
      ...d,
      properties: d.properties.map((p) => (p.type === 'birth_date' ? { ...p, datatype: 'bigint' } : p)),
    };
    expect(codes(d)).toContain('unsupported_datatype');

    // Duplicate names and missing descriptions.
    d = draft();
    d = { ...d, classes: [...d.classes, { ...d.classes[1] }] };
    expect(codes(d)).toContain('duplicate_class');

    d = draft();
    d = { ...d, classes: d.classes.map((c) => (c.type === 'place' ? { ...c, description: '' } : c)) };
    expect(codes(d)).toContain('class_without_description');
  });

  // The service infers the kind when `kind` is empty; the editor has to agree,
  // or it would show one thing and save another.
  it('infers the kind the way the service does', () => {
    const classes = new Set(['person', 'place']);
    expect(effectiveKind({ kind: '', range: 'place' }, classes)).toBe('object');
    expect(effectiveKind({ kind: '', range: '', datatype: 'date' }, classes)).toBe(
      'datatype',
    );
    expect(effectiveKind({ kind: 'OBJECT', range: '' }, classes)).toBe('object');
  });

  it('clears references when a class is deleted instead of dropping properties', () => {
    const d = removeClass(draft(), 2); // 'place'
    const bornIn = d.properties.find((p) => p.type === 'born_in');
    expect(bornIn?.range).toBe('');
    // The property is still there, and now reports what has to be decided.
    expect(codes(d)).toContain('missing_range');
  });

  it('rewrites references when a class is renamed', () => {
    const d = renameClass(draft(), 2, 'location'); // 'place' -> 'location'
    expect(d.properties.find((p) => p.type === 'born_in')?.range).toBe('location');
    expect(validateDraft(d)).toEqual([]);
  });

  it('deletes a property without touching the classes', () => {
    const d = removeProperty(draft(), 0);
    expect(d.properties).toHaveLength(1);
    expect(d.classes).toHaveLength(3);
  });

  it('splits "|"-separated lists exactly like the service', () => {
    expect(splitTypeList(' person | place ')).toEqual(['person', 'place']);
    expect(splitTypeList('')).toEqual([]);
    expect(splitTypeList(undefined)).toEqual([]);
  });
});
