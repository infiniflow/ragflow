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

import type {
  IArtifactGraph,
  IArtifactGraphEntity,
} from '@/interfaces/database/dataset';
import type { IOntologyGraphPage } from '@/interfaces/database/document-structure';

import type { AboxClassOption, AboxPropertyOption } from './types';

/**
 * Colour is information here, not decoration: it says which class a bubble
 * belongs to. The vocabulary is whatever the tenant's template declares, so the
 * palette cannot be enumerated — hashing the class name is what guarantees
 * "same class, same colour; different class, different colour" without a
 * registry, and keeps a colour stable across reloads.
 */
const Palette = [
  '#3b82f6',
  '#f59e0b',
  '#10b981',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#64748b',
  '#ef4444',
  '#84cc16',
  '#f97316',
  '#a855f7',
  '#14b8a6',
];

/** A class with no unit in scope (or no class at all) gets the neutral grey. */
const Unclassified = '#cbd5e1';

export const classColor = (type?: string): string => {
  if (!type) {
    return Unclassified;
  }
  let hash = 0;
  for (let i = 0; i < type.length; i += 1) {
    hash = (hash * 31 + type.charCodeAt(i)) >>> 0;
  }
  return Palette[hash % Palette.length];
};

/** Names repeat across classes, so the identity of an instance is type + name. */
export const entitySlug = (type: string | undefined, name: string): string =>
  `${type ?? ''}\u0000${name}`;

const normalizeName = (name?: string): string =>
  (name ?? '').trim().toLowerCase();

/**
 * L0: the declared vocabulary as a graph.
 *
 * One bubble per class, sized by how many instances the class collected, one
 * edge per declared object property. This needs no query of its own — the class
 * graph is the same data the template editor draws — which is why a dataset
 * with tens of thousands of rows can still open on something readable.
 */
export function classGraphFrom(
  classes: AboxClassOption[] = [],
  properties: AboxPropertyOption[] = [],
): IArtifactGraph {
  const entities: IArtifactGraphEntity[] = classes.map((item) => ({
    slug: item.type,
    name: item.label || item.type,
    aliases: [],
    description: '',
    type: item.type,
    // A class the caller only knows by name still has to be visible.
    weight: item.entities ?? 0,
  }));
  const declared = new Set(entities.map((entity) => entity.slug));
  const relations = properties
    .filter((item) => declared.has(item.source) && declared.has(item.target))
    .map((item) => ({
      from: item.source,
      to: item.target,
      type: item.label || item.type,
    }));

  return { entities, relations };
}

/**
 * L1/L2: the bounded subgraph the backend drew, as a force graph.
 *
 * The radius carries 来源文档数 rather than the mention count: a row that keeps
 * coming back in one document is one fact, a row that appears across five
 * documents is five chances to be found, and the reader is looking for the
 * latter.
 */
export function instanceGraphFrom(
  page?: IOntologyGraphPage,
): IArtifactGraph | undefined {
  if (!page) {
    return undefined;
  }
  const slugByName = new Map<string, string>();
  const entities: IArtifactGraphEntity[] = page.entities.map((entity) => {
    const slug = entitySlug(entity.type, entity.name ?? '');
    const key = normalizeName(entity.name);
    if (!slugByName.has(key)) {
      slugByName.set(key, slug);
    }
    return {
      slug,
      name: entity.name ?? '',
      aliases: entity.aliases ?? [],
      description: entity.description ?? '',
      type: entity.type ?? '',
      weight: entity.document_ids?.length || entity.mention_count || 1,
    };
  });

  const present = new Set(entities.map((entity) => entity.slug));
  const relations = page.relations
    .map((relation) => ({
      from: slugByName.get(normalizeName(relation.from)),
      to: slugByName.get(normalizeName(relation.to)),
      type: relation.type ?? '',
    }))
    .filter(
      (relation): relation is { from: string; to: string; type: string } =>
        !!relation.from &&
        !!relation.to &&
        present.has(relation.from) &&
        present.has(relation.to),
    );

  return {
    entities,
    relations,
    total_entities: page.total,
    returned_entities: page.drawn,
  };
}

/**
 * One hop around a node — taken from what is already drawn, not from the server.
 *
 * The canvas already holds the induced subgraph, so an ego view is a subset of
 * it: no request, no latency, and the same nodes the reader can see. The honest
 * limit is that it cannot invent an edge the budget never fetched. That limit is
 * why the caller keeps showing the canvas budget next to the result instead of
 * letting a filtered view imply it is the neighbourhood.
 *
 * Ties between the neighbours are kept: a 1-hop view that dropped them would
 * hide that two of the neighbours know each other, which is often the reason to
 * open one in the first place.
 */
/**
 * The property filter's options: one entry per property name.
 *
 * A polymorphic property is declared once per domain/range pair, so the
 * `properties` array legitimately carries `resides_in` four times (four
 * class pairs). The filter matches on the name alone — the pair is never sent —
 * so rendering the array directly offered four identical choices.
 *
 * The first declaration wins: its label is the one the model gave the property,
 * and the pair it happens to come from says nothing about the filter.
 */
export function propertyOptionsFrom(
  properties: AboxPropertyOption[] = [],
): { label: string; value: string }[] {
  const seen = new Set<string>();
  const options: { label: string; value: string }[] = [];
  properties.forEach((item) => {
    const value = item.type?.trim();
    const key = value?.toLowerCase();
    if (!value || !key || seen.has(key)) {
      return;
    }
    seen.add(key);
    options.push({ label: item.label || value, value });
  });
  return options;
}

export function egoSubgraph(
  graph: IArtifactGraph,
  slug?: string,
): IArtifactGraph {
  if (!slug) {
    return graph;
  }
  const keep = new Set([slug]);
  graph.relations.forEach((relation) => {
    if (relation.from === slug) {
      keep.add(relation.to);
    } else if (relation.to === slug) {
      keep.add(relation.from);
    }
  });
  return {
    ...graph,
    entities: graph.entities.filter((entity) => keep.has(entity.slug)),
    relations: graph.relations.filter(
      (relation) => keep.has(relation.from) && keep.has(relation.to),
    ),
  };
}
