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

const sectionFields = (
  config: ICompilationTemplateConfig,
  section: string,
): Record<string, string>[] => {
  const raw = config?.[section] as { fields?: unknown } | undefined;
  const fields = raw?.fields;
  if (!Array.isArray(fields)) {
    return [];
  }
  return fields.filter(
    (field): field is Record<string, string> =>
      !!field && typeof field === 'object',
  );
};

const withSectionFields = (
  config: ICompilationTemplateConfig,
  section: string,
  fields: Record<string, string>[],
): ICompilationTemplateConfig => {
  // The section keeps whatever else it carried (a description, a `use_blueprint`
  // flag) and only its field list is replaced.
  const existing = config?.[section];
  const base =
    existing && typeof existing === 'object' && !Array.isArray(existing)
      ? (existing as Record<string, unknown>)
      : {};
  return { ...(config ?? {}), [section]: { ...base, fields } };
};

/**
 * Adds one class to the declaration.
 *
 * The drift panel offers this because it is the only finding whose fix is a
 * declaration and nothing else: the corpus already produced the rows, so naming
 * the class is the whole change. Anything more (hierarchy, aliases, attributes)
 * is modelling and belongs in the canvas.
 *
 * A name that is already declared is returned unchanged rather than duplicated:
 * the caller is reacting to a stale panel as often as not.
 */
export const withDeclaredClass = (
  config: ICompilationTemplateConfig | undefined,
  className: string,
): ICompilationTemplateConfig => {
  const name = className.trim();
  const base = config ?? {};
  if (!name) {
    return base;
  }
  const fields = sectionFields(base, 'entity');
  if (fields.some((field) => String(field.type ?? '').trim() === name)) {
    return base;
  }
  return withSectionFields(base, 'entity', [
    ...fields,
    { type: name, label: name },
  ]);
};

/**
 * Adds one attribute to a declared class.
 *
 * `range` is left empty and the datatype is a plain string: the panel is
 * answering "the corpus has a field the model does not name", and widening the
 * type is a decision for whoever knows the data. An empty range still produces a
 * valid datatype property (see the service's declaration parser).
 */
export const withDeclaredAttribute = (
  config: ICompilationTemplateConfig | undefined,
  className: string,
  fieldName: string,
): ICompilationTemplateConfig => {
  const owner = className.trim();
  const name = fieldName.trim();
  const base = config ?? {};
  if (!owner || !name) {
    return base;
  }
  const fields = sectionFields(base, 'relation');
  if (
    fields.some(
      (field) =>
        String(field.type ?? '').trim() === name &&
        String(field.domain ?? '').trim() === owner,
    )
  ) {
    return base;
  }
  return withSectionFields(base, 'relation', [
    ...fields,
    { type: name, label: name, domain: owner, datatype: 'string' },
  ]);
};

/**
 * Declares the class AND the attributes the corpus already carries for it.
 *
 * The drift row knows both halves — "Invoice exists with 18,401 rows" and
 * "due_date is filled in 61% of them" — so a declaration that names only the
 * class throws away the second half and leaves the same attribute to be
 * declared by hand. The properties are added as datatype properties, which is
 * what the service's parser infers for a value with no declared range.
 */
export const withDeclaredClassAndAttributes = (
  config: ICompilationTemplateConfig | undefined,
  className: string,
  attributeNames: string[] = [],
): ICompilationTemplateConfig => {
  const declared = withDeclaredClass(config, className);
  return attributeNames.reduce(
    (next, attribute) => withDeclaredAttribute(next, className, attribute),
    declared,
  );
};

const splitRefs = (value: string | undefined): string[] =>
  String(value ?? '')
    .split('|')
    .map((item) => item.trim())
    .filter(Boolean);

/**
 * Removes a declared class and CLEARS every reference to it.
 *
 * Mirrors `removeEntity` in the canvas model: the properties that mentioned it
 * are kept, with the dangling endpoint emptied, because the reader asked to
 * delete one class — not to lose the relations that happened to point at it.
 * Those relations then show up as `missing_domain` / `missing_range` in the
 * canvas validation, which is where the decision about them belongs.
 */
export const withoutDeclaredClass = (
  config: ICompilationTemplateConfig | undefined,
  className: string,
): ICompilationTemplateConfig => {
  const base = config ?? {};
  const name = className.trim();
  if (!name) {
    return base;
  }
  const entities = sectionFields(base, 'entity');
  const kept = entities.filter(
    (field) => String(field.type ?? '').trim() !== name,
  );
  if (kept.length === entities.length) {
    return base;
  }
  const relations = sectionFields(base, 'relation').map((field) => {
    const domain = splitRefs(field.domain);
    const range = splitRefs(field.range);
    if (!domain.includes(name) && !range.includes(name)) {
      return field;
    }
    return {
      ...field,
      domain: domain.filter((item) => item !== name).join('|'),
      range: range.filter((item) => item !== name).join('|'),
    };
  });
  const removed = withSectionFields(base, 'relation', relations);
  return withSectionFields(removed, 'entity', kept);
};

/**
 * Removes a declared property by its name.
 *
 * A relation the corpus never produced is either mis-worded or not needed; this
 * is the "not needed" answer, and it is the only write the panel offers for that
 * finding that does not require opening the canvas.
 */
export const withoutDeclaredProperty = (
  config: ICompilationTemplateConfig | undefined,
  propertyType: string,
): ICompilationTemplateConfig => {
  const base = config ?? {};
  const name = propertyType.trim();
  if (!name) {
    return base;
  }
  const relations = sectionFields(base, 'relation');
  const kept = relations.filter(
    (field) => String(field.type ?? '').trim() !== name,
  );
  if (kept.length === relations.length) {
    return base;
  }
  return withSectionFields(base, 'relation', kept);
};

/**
 * Writes a description onto a declared class.
 *
 * This is the fix for "declared, but the corpus never produced one" that is not
 * a deletion: the description (and the aliases it implies) is what reaches the
 * extraction prompt, so an empty class is usually an under-described one, not a
 * wrong one.
 */
export const withClassDescription = (
  config: ICompilationTemplateConfig | undefined,
  className: string,
  description: string,
): ICompilationTemplateConfig => {
  const base = config ?? {};
  const name = className.trim();
  if (!name) {
    return base;
  }
  const entities = sectionFields(base, 'entity');
  const index = entities.findIndex(
    (field) => String(field.type ?? '').trim() === name,
  );
  if (index < 0) {
    return base;
  }
  const next = entities.map((field, at) =>
    at === index ? { ...field, description } : field,
  );
  return withSectionFields(base, 'entity', next);
};

/**
 * Writes the two things that raise a low fill rate: a description (what the
 * value looks like) and a closed value set (which values are allowed). Both
 * reach the extraction prompt; nothing else on a property does.
 */
export const withPropertyNotes = (
  config: ICompilationTemplateConfig | undefined,
  propertyType: string,
  notes: { description?: string; enumValues?: string },
): ICompilationTemplateConfig => {
  const base = config ?? {};
  const name = propertyType.trim();
  if (!name) {
    return base;
  }
  const relations = sectionFields(base, 'relation');
  const index = relations.findIndex(
    (field) => String(field.type ?? '').trim() === name,
  );
  if (index < 0) {
    return base;
  }
  const next = relations.map((field, at) => {
    if (at !== index) return field;
    const patched = { ...field };
    if (notes.description !== undefined) {
      patched.description = notes.description;
    }
    if (notes.enumValues !== undefined) {
      patched.enum = notes.enumValues;
    }
    return patched;
  });
  return withSectionFields(base, 'relation', next);
};
