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

/**
 * The ontology editor's model, kept pure on purpose.
 *
 * The canvas is a VIEW of the template's `config.entity.fields` /
 * `config.relation.fields`; everything that changes those arrays — adding a
 * entity, binding a property's domain, renaming, deleting, validating — happens
 * here, on plain objects, so the one thing that must never break (the saved
 * shape) can be tested without a browser.
 *
 * The key sets below are the contract with the backend
 * (`compilation_template_service.go` reads exactly these keys, and
 * `add-field-modal` derives its inputs from the first field of the builtin
 * template, which lists them in this order).
 */

/** An entity field, in the order the builtin ontology template lists it. */
export const OntologyEntityKeys = [
  'type',
  'label',
  'description',
  'parent',
  'aliases',
  'rule',
  // Which contract fields an action entity requires. A "|"-separated list, like
  // `aliases`: every value in an entity field is a string, and the service reads
  // the same form the input edits.
  'contract',
] as const;

/**
 * A property field, same idea.
 *
 * `enum` and `required` are the acceptance half of a declaration: `enum` closes a
 * datatype property's value set (stated to the model, enforced on the stored
 * value), `required` says every instance of the domain entity should carry the
 * attribute. `required` is deliberately not stated to the model — a model told an
 * attribute is mandatory invents a value — so it only shows up as a `missing`
 * count on the entity's attribute list.
 */
export const OntologyPropertyKeys = [
  'type',
  'kind',
  'domain',
  'range',
  'datatype',
  'aliases',
  'enum',
  'required',
  'label',
  'description',
  'rule',
] as const;

/** Mirrors the service's `ontologyDatatypeKinds`. */
export const OntologyDatatypeKinds = [
  'string',
  'list',
  'int',
  'integer',
  'float',
  'number',
  'bool',
  'boolean',
  'date',
];

export const OntologyKind = {
  Object: 'object',
  Datatype: 'datatype',
} as const;

/** One field of a section, as the form stores it (every value a string). */
export type OntologyField = Record<string, string>;

export interface OntologySection {
  description?: string;
  fields?: OntologyField[];
}

export interface OntologyDraft {
  entities: OntologyField[];
  properties: OntologyField[];
}

export interface OntologyIssue {
  code: string;
  message: string;
  /** The entity or property the issue is about. */
  subject: string;
}

/** Mirrors the service's `splitTypeList`: "|"-separated, trimmed, empties dropped. */
export const splitTypeList = (value: string | undefined): string[] =>
  String(value ?? '')
    .split('|')
    .map((item) => item.trim())
    .filter(Boolean);

/**
 * canonicalField puts a field's keys in the declared order and gives every
 * missing one an empty string — that is the shape the builtin template uses, and
 * what makes the saved config diff-clean against it.
 *
 * A key this editor does not model is KEPT, after the known ones: a template
 * may have been authored by hand (or by a future version) and silently dropping
 * unknown keys on save is data loss, not normalisation.
 */
export const canonicalField = (
  field: OntologyField,
  keys: readonly string[],
): OntologyField => {
  const out: OntologyField = {};
  keys.forEach((key) => {
    out[key] = field[key] ?? '';
  });
  Object.entries(field).forEach(([key, value]) => {
    if (!(key in out)) out[key] = value ?? '';
  });
  return out;
};

export const canonicalEntities = (fields?: OntologyField[]): OntologyField[] =>
  (fields ?? []).map((field) => canonicalField(field, OntologyEntityKeys));

export const canonicalProperties = (
  fields?: OntologyField[],
): OntologyField[] =>
  (fields ?? []).map((field) => canonicalField(field, OntologyPropertyKeys));

/**
 * valueListText renders a declared list (`aliases`, `enum`) for a text input.
 *
 * A template saved by the YAML editor carries an array; the input edits the
 * "|"-separated form, which the service reads as the same closed set. Rendering
 * an array with `String()` would join it with commas and turn two aliases into
 * one string, so the distinction has to be made here.
 */
export const valueListText = (value: unknown): string =>
  Array.isArray(value) ? value.map(String).join(' | ') : String(value ?? '');

/** valueListTruthy reads a declared flag that may arrive as a bool or a string. */
export const valueListTruthy = (value: unknown): boolean =>
  value === true ||
  String(value ?? '')
    .trim()
    .toLowerCase() === 'true';

/** Reads the two sections the ontology canvas edits. */
export const draftFromSections = (
  entity?: OntologySection,
  relation?: OntologySection,
): OntologyDraft => ({
  entities: canonicalEntities(entity?.fields),
  properties: canonicalProperties(relation?.fields),
});

/** A section as it is written back: `fields` is always present. */
export interface OntologySectionDraft {
  description?: string;
  fields: OntologyField[];
  /** Anything else the section carried (`entity.output_fields`, …). */
  [key: string]: unknown;
}

/**
 * Writes a draft back as the two sections, in the template's shape.
 *
 * `current` is the section as it stands in the form, and its OTHER keys are
 * carried through: the editor owns `fields`, not the whole section — `entity`
 * also declares `output_fields` in the builtin ontology template, and replacing
 * the section wholesale would drop it on the next save.
 */
export const sectionsFromDraft = (
  draft: OntologyDraft,
  current?: { entity?: OntologySection; relation?: OntologySection },
): { entity: OntologySectionDraft; relation: OntologySectionDraft } => ({
  entity: {
    ...(current?.entity ?? {}),
    fields: canonicalEntities(draft.entities),
  },
  relation: {
    ...(current?.relation ?? {}),
    fields: canonicalProperties(draft.properties),
  },
});

export const entityNames = (draft: OntologyDraft): Set<string> => {
  const names = new Set<string>();
  draft.entities.forEach((item) => {
    const name = String(item.type ?? '').trim();
    if (name) names.add(name);
  });
  return names;
};

/**
 * effectiveKind is how the SERVICE reads a property's kind, including its
 * inference when `kind` is empty (a range naming a declared entity means an
 * object property, anything else a datatype property). Mirroring it here means
 * the editor never disagrees with what the backend will conclude on save.
 */
export const effectiveKind = (
  field: OntologyField,
  entities: Set<string>,
): string => {
  const declared = String(field.kind ?? '')
    .trim()
    .toLowerCase();
  if (declared) return declared;
  return splitTypeList(field.range).some((item) => entities.has(item))
    ? OntologyKind.Object
    : OntologyKind.Datatype;
};

const uniqueName = (base: string, taken: Set<string>): string => {
  if (!taken.has(base)) return base;
  let suffix = 2;
  while (taken.has(`${base}_${suffix}`)) suffix += 1;
  return `${base}_${suffix}`;
};

const emptyField = (keys: readonly string[]): OntologyField =>
  Object.fromEntries(keys.map((key) => [key, '']));

/** Adds an entity with a free name, ready to be renamed in the panel. */
export const addEntity = (draft: OntologyDraft): OntologyDraft => {
  const taken = entityNames(draft);
  return {
    ...draft,
    entities: [
      ...draft.entities,
      {
        ...emptyField(OntologyEntityKeys),
        type: uniqueName('new_entity', taken),
        label: uniqueName('New entity', taken),
      },
    ],
  };
};

/** Adds a property of the given kind, with no endpoints yet (flagged as such). */
export const addProperty = (
  draft: OntologyDraft,
  kind: string = OntologyKind.Object,
): OntologyDraft => {
  const taken = new Set(
    draft.properties
      .map((item) => String(item.type ?? '').trim())
      .filter(Boolean),
  );
  const isDatatype = kind === OntologyKind.Datatype;
  return {
    ...draft,
    properties: [
      ...draft.properties,
      {
        ...emptyField(OntologyPropertyKeys),
        type: uniqueName(isDatatype ? 'new_attribute' : 'new_relation', taken),
        kind: isDatatype ? OntologyKind.Datatype : OntologyKind.Object,
        datatype: isDatatype ? 'string' : '',
      },
    ],
  };
};

export const updateField = (
  list: OntologyField[],
  index: number,
  patch: OntologyField,
): OntologyField[] =>
  list.map((field, at) => (at === index ? { ...field, ...patch } : field));

/** Renames an entity and rewrites every reference to the old name. */
export const renameEntity = (
  draft: OntologyDraft,
  index: number,
  name: string,
): OntologyDraft => {
  const previous = String(draft.entities[index]?.type ?? '').trim();
  const entities = updateField(draft.entities, index, { type: name });
  if (!previous || previous === name) return { ...draft, entities };
  const swap = (value: string | undefined) =>
    splitTypeList(value)
      .map((item) => (item === previous ? name : item))
      .join('|');
  const properties = draft.properties.map((field) => ({
    ...field,
    domain: swap(field.domain),
    range: swap(field.range),
  }));
  return { entities, properties };
};

/**
 * Deletes an entity and CLEARS every reference to it (a parent, a domain, a
 * range) instead of deleting the properties that used it: the reader asked to
 * remove one entity, not to silently lose the properties that mentioned it. The
 * references it clears then show up in the issue list, which is where the
 * decision about those properties belongs.
 */
export const removeEntity = (
  draft: OntologyDraft,
  index: number,
): OntologyDraft => {
  const removed = String(draft.entities[index]?.type ?? '').trim();
  const entities = draft.entities.filter((_, at) => at !== index);
  if (!removed) return { ...draft, entities };
  const drop = (value: string | undefined) =>
    splitTypeList(value)
      .filter((item) => item !== removed)
      .join('|');
  const properties = draft.properties.map((field) => ({
    ...field,
    domain: drop(field.domain),
    range: drop(field.range),
  }));
  return { entities, properties };
};

export const removeProperty = (
  draft: OntologyDraft,
  index: number,
): OntologyDraft => ({
  ...draft,
  properties: draft.properties.filter((_, at) => at !== index),
});

/** Mirrors `ValidateTemplatePayload`'s rules for the ontology kind. */
export const validateDraft = (draft: OntologyDraft): OntologyIssue[] => {
  const issues: OntologyIssue[] = [];
  const entities = new Set<string>();

  draft.entities.forEach((field) => {
    const name = String(field.type ?? '').trim();
    if (!name) {
      issues.push({
        code: 'entity_without_type',
        message: 'An entity needs a name (type).',
        subject: '',
      });
      return;
    }
    if (entities.has(name)) {
      issues.push({
        code: 'duplicate_entity',
        message: `Two entities are both named ${name}.`,
        subject: name,
      });
      return;
    }
    entities.add(name);
  });

  if (entities.size === 0) {
    issues.push({
      code: 'no_entity',
      message: 'An ontology template must declare at least one entity.',
      subject: '',
    });
  }

  draft.entities.forEach((field) => {
    const name = String(field.type ?? '').trim();
    if (!name) return;
    if (!String(field.description ?? '').trim()) {
      issues.push({
        code: 'entity_without_description',
        message: `Entity ${name} needs a description.`,
        subject: name,
      });
    }
    splitTypeList(field.parent).forEach((parent) => {
      if (!entities.has(parent)) {
        issues.push({
          code: 'unknown_parent',
          message: `Entity ${name} declares unknown parent ${parent}.`,
          subject: name,
        });
      }
    });
  });

  // A cyclic chain would make the inherited-attribute walk never terminate.
  const parentOf = new Map<string, string>();
  draft.entities.forEach((field) => {
    const name = String(field.type ?? '').trim();
    const parent = splitTypeList(field.parent)[0];
    if (name && parent) parentOf.set(name, parent);
  });
  parentOf.forEach((_, child) => {
    const seen = new Set<string>([child]);
    let cursor = parentOf.get(child);
    while (cursor) {
      if (seen.has(cursor)) {
        issues.push({
          code: 'cyclic_parent',
          message: `Entity ${child} has a cyclic parent chain.`,
          subject: child,
        });
        return;
      }
      seen.add(cursor);
      cursor = parentOf.get(cursor);
    }
  });

  const propertyNames = new Set<string>();
  draft.properties.forEach((field) => {
    const name = String(field.type ?? '').trim();
    if (!name) {
      issues.push({
        code: 'property_without_type',
        message: 'A property needs a name (type).',
        subject: '',
      });
      return;
    }
    if (propertyNames.has(name)) {
      issues.push({
        code: 'duplicate_property',
        message: `Two properties are both named ${name}.`,
        subject: name,
      });
      return;
    }
    propertyNames.add(name);

    if (!String(field.description ?? '').trim()) {
      issues.push({
        code: 'property_without_description',
        message: `Property ${name} needs a description.`,
        subject: name,
      });
    }

    const kind = effectiveKind(field, entities);
    const domains = splitTypeList(field.domain);
    const ranges = splitTypeList(field.range);
    const datatype = String(field.datatype ?? '')
      .trim()
      .toLowerCase();

    if (kind !== OntologyKind.Object && kind !== OntologyKind.Datatype) {
      issues.push({
        code: 'invalid_kind',
        message: `Property ${name} has invalid kind "${kind}" (want object or datatype).`,
        subject: name,
      });
      return;
    }

    if (domains.length === 0) {
      issues.push({
        code: 'missing_domain',
        message: `${name} declares no domain.`,
        subject: name,
      });
    }
    domains.forEach((domain) => {
      if (!entities.has(domain)) {
        issues.push({
          code: 'unknown_domain',
          message: `Property ${name} declares unknown domain entity ${domain}.`,
          subject: name,
        });
      }
    });

    if (kind === OntologyKind.Object) {
      // A closed value set and a mandatory value are both datatype-property
      // ideas. The service refuses them on an object property instead of
      // ignoring them, so mirroring it here catches the mistake before saving.
      if (splitTypeList(String(field.enum ?? '')).length > 0) {
        issues.push({
          code: 'enum_on_object',
          message: `Object property ${name} declares an enum, which only applies to datatype properties.`,
          subject: name,
        });
      }
      if (valueListTruthy(field.required)) {
        issues.push({
          code: 'required_on_object',
          message: `Object property ${name} declares required, which only applies to datatype properties.`,
          subject: name,
        });
      }
      if (ranges.length === 0) {
        issues.push({
          code: 'missing_range',
          message: `Object property ${name} declares no range.`,
          subject: name,
        });
      }
      ranges.forEach((range) => {
        if (!entities.has(range)) {
          issues.push({
            code: 'unknown_range',
            message: `Property ${name} declares unknown range entity ${range}.`,
            subject: name,
          });
        }
      });
      return;
    }

    if (datatype && !OntologyDatatypeKinds.includes(datatype)) {
      issues.push({
        code: 'unsupported_datatype',
        message: `Property ${name} declares unsupported datatype "${datatype}".`,
        subject: name,
      });
    }
  });

  return issues;
};
