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
 * The pitfalls panel reports CODES; the sentence it shows has to be the reader's
 * language. The service composes a message too, and that message is kept — as the
 * tooltip — so a code this file does not know yet still says something, and a
 * known code says the same thing in every language.
 */

/** The four groups, in the order the panel lists them. */
export const PitfallCategoryOrder = [
  'logical',
  'structural',
  'naming',
  'semantic',
] as const;

const CategoryFallback: Record<string, string> = {
  logical: 'Logical',
  structural: 'Structural',
  naming: 'Naming',
  semantic: 'Semantic',
};

/** English text used when a locale has not been translated (or fails to load). */
const PitfallFallback: Record<string, string> = {
  dangling_property:
    'A property names an endpoint class the template does not declare',
  dangling_parent: 'A class declares a parent that is not a declared class',
  redundant_parent:
    'A class declares a parent and that parent’s ancestor, so one edge says nothing',
  class_without_property: 'No property touches this class, own or inherited',
  orphan_class:
    'This class has no property and no compiled instance, so it only ever draws an isolated node',
  single_child_parent: 'A parent class has a single subclass',
  disconnected_hierarchy:
    'The hierarchy has more than one root, so it is split into separate trees',
  endpoint_ancestor_expansion:
    'A property names an endpoint and one of its ancestors, which the ancestor already covers',
  property_name_hierarchy:
    'One property’s name nests inside another’s between the same classes, so it reads as a sub-property',
  standard_vocabulary_property:
    'The name is already owned by RDF/OWL vocabulary',
  range_in_property_name: 'The property’s name repeats the class it points at',
  domain_in_property_name: 'The property’s name repeats the class it starts from',
  overly_generic_class:
    'The name is so general that an extraction model can file anything under it',
  class_property_name_collision: 'A class and a property share this name',
  datatype_is_class:
    'The declared datatype is a class of this ontology, so it was meant to be an object property',
};

export const pitfallCategoryLabel = (
  category: string | undefined,
  translate: (key: string, options: { defaultValue: string }) => string,
): string => {
  const known = category && CategoryFallback[category] ? category : 'structural';
  return translate(`knowledgeCompilation.ontologyPitfallCategory_${known}`, {
    defaultValue: CategoryFallback[known],
  });
};

export const pitfallLabel = (
  code: string,
  translate: (key: string, options: { defaultValue: string }) => string,
): string =>
  translate(`knowledgeCompilation.ontologyPitfall_${code}`, {
    defaultValue: PitfallFallback[code] ?? code,
  });
