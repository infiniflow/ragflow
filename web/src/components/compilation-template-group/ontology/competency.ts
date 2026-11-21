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
import { IOntologyCompetencyQuestionDraft } from '@/interfaces/database/ontology-insight';

/**
 * The config section competency questions live in.
 *
 * They are a section of the model rather than a table of their own because they
 * are a statement about the model — "this is what it must be able to answer" —
 * and the version snapshot has to carry them for the publish gate to judge a
 * version by what it promised.
 */
export const CompetencySectionKey = 'competency';

const readSection = (
  config?: ICompilationTemplateConfig,
): IOntologyCompetencyQuestionDraft[] => {
  const section = config?.[CompetencySectionKey];
  const fields = (section as { fields?: unknown } | undefined)?.fields;
  if (!Array.isArray(fields)) {
    return [];
  }
  return fields.flatMap((raw) => {
    if (!raw || typeof raw !== 'object') {
      return [];
    }
    const entry = raw as Record<string, unknown>;
    const type = String(entry.type ?? '').trim();
    const question = String(entry.question ?? '').trim();
    if (!type && !question) {
      return [];
    }
    return [
      {
        type: type || question,
        question,
        elements: String(entry.elements ?? '').trim(),
      },
    ];
  });
};

export const parseCompetencyQuestions = (
  config?: ICompilationTemplateConfig,
): IOntologyCompetencyQuestionDraft[] => readSection(config);

/**
 * Returns the config with its competency section replaced.
 *
 * The whole section is rewritten rather than patched in place: the array is the
 * unit the backend diffs by `type`, and a half-updated section would let two
 * questions disagree about which one owns a `type` value.
 */
export const withCompetencyQuestions = (
  config: ICompilationTemplateConfig | undefined,
  questions: IOntologyCompetencyQuestionDraft[],
): ICompilationTemplateConfig => ({
  ...(config ?? {}),
  [CompetencySectionKey]: {
    fields: questions.map((question) => ({
      type: question.type,
      question: question.question,
      elements: question.elements,
    })),
  },
});

/** One parsed `kind:name` clause of a question's required elements. */
export type CompetencyElementClause = {
  kind: 'class' | 'relation' | 'property';
  name: string;
  optional: boolean;
};

const ELEMENT_KINDS = new Set(['class', 'relation', 'property']);

/**
 * Parses the element clause back into rows.
 *
 * The coverage numbers come from the backend, which measures the corpus. This
 * parser exists for the screen before a measurement exists: the elements a
 * question needs are already known (they were derived from its text), so the
 * detail view can list them with "not measured" instead of an empty box. An
 * unknown kind is dropped for the same reason the backend drops it — a typo
 * must not make the whole question unreadable.
 */
export const parseCompetencyElements = (
  elements: string,
): CompetencyElementClause[] =>
  elements
    .split(/[|\n]/)
    .map((part) => part.trim())
    .flatMap((part) => {
      if (!part) {
        return [];
      }
      const optional = part.startsWith('?');
      const clause = optional ? part.slice(1).trim() : part;
      const colon = clause.indexOf(':');
      if (colon <= 0) {
        return [];
      }
      const kind = clause.slice(0, colon).trim().toLowerCase();
      const name = clause.slice(colon + 1).trim();
      if (!ELEMENT_KINDS.has(kind) || !name) {
        return [];
      }
      return [
        { kind: kind as CompetencyElementClause['kind'], name, optional },
      ];
    });

/**
 * Counts the required elements a question declares.
 *
 * An element prefixed with `?` is optional and is not counted. Zero means the
 * question was never traced, which the backend treats as unanswerable rather
 * than as a pass.
 */
export const requiredElementCount = (elements: string): number =>
  parseCompetencyElements(elements).filter((element) => !element.optional)
    .length;

/**
 * Suggests the next question id.
 *
 * Ids are the diff key, so they have to be unique and stable. Reusing the
 * highest number keeps a deleted question from being silently resurrected by a
 * later insert that picked the same id.
 */
export const nextCompetencyId = (
  questions: IOntologyCompetencyQuestionDraft[],
): string => {
  let highest = 0;
  for (const question of questions) {
    const match = /^CQ-(\d+)$/i.exec(question.type.trim());
    if (match) {
      highest = Math.max(highest, Number(match[1]));
    }
  }
  return `CQ-${highest + 1}`;
};

/**
 * The element clause grammar, in the one place both the editor and the page read
 * it: `kind:name`, separated by `|` or a newline, `?` prefix for optional.
 */
export const CompetencyElementGrammar =
  'class:Contract | relation:Contract->Vendor | property:Contract.amount | ?class:Person';

/** One term a class or property answers to: its `type`, `label` or an alias. */
const termsOf = (entry: Record<string, unknown>): string[] => {
  const out: string[] = [];
  const push = (value: unknown) => {
    if (typeof value !== 'string') {
      return;
    }
    const trimmed = value.trim();
    if (trimmed && !out.includes(trimmed)) {
      out.push(trimmed);
    }
  };
  push(entry.type);
  push(entry.label);
  const aliases = entry.aliases;
  if (Array.isArray(aliases)) {
    aliases.forEach(push);
  } else if (typeof aliases === 'string') {
    aliases.split(/[,，|]/).forEach(push);
  }
  return out;
};

/** The endpoints a property declares. `|` separates a polymorphic endpoint. */
const endpointsOf = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value.flatMap((item) => endpointsOf(item));
  }
  if (typeof value !== 'string') {
    return [];
  }
  return value
    .split('|')
    .map((item) => item.trim())
    .filter((item) => item !== '');
};

export type CompetencyVocabulary = {
  classes: { type: string; terms: string[] }[];
  properties: {
    type: string;
    object: boolean;
    terms: string[];
    domains: string[];
    ranges: string[];
  }[];
};

/**
 * The vocabulary a question can be traced against.
 *
 * Read from the working config, not from the measured graph: the author writes
 * the question while editing the model, and an element the model declares but
 * the corpus has never filled is a finding ("declared, no rows"), not an
 * unmatched word.
 */
export const vocabularyOf = (
  config?: ICompilationTemplateConfig,
): CompetencyVocabulary => {
  const section = config as Record<string, unknown> | undefined;
  const fieldList = (key: string): Record<string, unknown>[] => {
    const list = (section?.[key] as { fields?: unknown } | undefined)?.fields;
    if (!Array.isArray(list)) {
      return [];
    }
    return list.filter(
      (item): item is Record<string, unknown> =>
        !!item && typeof item === 'object',
    );
  };

  const classes = fieldList('entity').flatMap((entry) => {
    const type = typeof entry.type === 'string' ? entry.type.trim() : '';
    return type ? [{ type, terms: termsOf(entry) }] : [];
  });
  const properties = fieldList('relation').flatMap((entry) => {
    const type = typeof entry.type === 'string' ? entry.type.trim() : '';
    if (!type) {
      return [];
    }
    const ranges = endpointsOf(entry.range);
    const kind = typeof entry.kind === 'string' ? entry.kind.trim() : '';
    const object =
      kind === 'object'
        ? true
        : kind === 'datatype'
          ? false
          : ranges.length > 0;
    return [
      {
        type,
        object,
        terms: termsOf(entry),
        domains: endpointsOf(entry.domain),
        ranges,
      },
    ];
  });
  return { classes, properties };
};

const STOP_WORDS = new Set([
  'about',
  'after',
  'before',
  'does',
  'each',
  'from',
  'has',
  'have',
  'into',
  'many',
  'much',
  'such',
  'than',
  'that',
  'their',
  'them',
  'then',
  'there',
  'these',
  'they',
  'this',
  'those',
  'what',
  'when',
  'where',
  'which',
  'while',
  'with',
  'whose',
]);

const CJK = /[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uac00-\ud7af]/;

const hasCjk = (value: string) => CJK.test(value);

/**
 * Whether a vocabulary term is named by the question.
 *
 * Latin terms match on word boundaries with an optional plural `s`; CJK terms
 * match as substrings, because Chinese has no spaces to bound them. A term is a
 * hint, not a claim: the suggestion it produces stays editable.
 */
/** The words a vocabulary term is written with in prose. */
const termWords = (term: string): string[] =>
  term
    .trim()
    .toLowerCase()
    .split(/[_\-\s]+/)
    .filter((word) => word.length > 0 && !STOP_WORDS.has(word));

const namesTerm = (question: string, term: string): boolean => {
  const needle = term.trim().toLowerCase();
  if (!needle) {
    return false;
  }
  if (hasCjk(needle)) {
    return question.includes(needle);
  }
  // A term is a declaration spelling, not a prose spelling: `born_in` is written
  // "born in" and `subClassOf` as "sub class of". Matching the raw string made
  // every underscored property unmatchable, which is most of them.
  const words = termWords(needle);
  if (words.length === 0) {
    return false;
  }
  return words.every((word) => {
    const escaped = word.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp(`(^|[^a-z0-9])${escaped}s?([^a-z0-9]|$)`, 'i').test(
      question,
    );
  });
};

/**
 * Traces a question to the model elements an answer needs.
 *
 * This is the alternative to typing `class:`/`property:`/`relation:` clauses by
 * hand: the question already names what it is about, and the model already knows
 * every name that can exist. The result is written into the elements field and
 * stays editable, so a wrong guess costs one word, not a re-typed clause.
 *
 * `unmatched` reports the words that named nothing, which is the one thing a
 * silent empty result cannot tell the author: the question may be asking about
 * something the model has no class for.
 */
export const traceCompetencyElements = (
  question: string,
  vocabulary: CompetencyVocabulary,
): {
  elements: string;
  classes: string[];
  properties: string[];
  relations: string[];
  unmatched: string[];
} => {
  const text = question.trim().toLowerCase();
  if (!text) {
    return {
      elements: '',
      classes: [],
      properties: [],
      relations: [],
      unmatched: [],
    };
  }

  const classes = vocabulary.classes
    .filter((entry) => entry.terms.some((term) => namesTerm(text, term)))
    .map((entry) => entry.type);

  const properties: string[] = [];
  const relations: string[] = [];
  for (const property of vocabulary.properties) {
    const named = property.terms.some((term) => namesTerm(text, term));
    if (!named) {
      continue;
    }
    if (property.object) {
      continue;
    }
    const owner = property.domains.find((domain) => classes.includes(domain));
    if (!owner) {
      // A datatype property with no matched owner class would have to be
      // attached to an arbitrary one; say nothing instead.
      continue;
    }
    const element = `property:${owner}.${property.type}`;
    if (!properties.includes(element)) {
      properties.push(element);
    }
  }

  // An edge is named by the class pair it runs between. A property the question
  // names contributes its declared pairs even when only one endpoint is named:
  // "where was this person born in" reaches `person -> place` without the
  // question spelling out "place", because the declaration already said where
  // that property goes. Declaration order keeps the result stable.
  const seenRelations = new Set<string>();
  const pushRelation = (source: string, target: string) => {
    const key = `${source}->${target}`;
    if (seenRelations.has(key)) {
      return;
    }
    seenRelations.add(key);
    relations.push(`relation:${key}`);
  };
  for (const property of vocabulary.properties) {
    if (!property.object) {
      continue;
    }
    const named = property.terms.some((term) => namesTerm(text, term));
    if (named) {
      for (const source of property.domains) {
        if (!classes.includes(source)) {
          continue;
        }
        for (const target of property.ranges) {
          pushRelation(source, target);
        }
      }
      continue;
    }
    for (const source of classes) {
      for (const target of classes) {
        if (
          property.domains.includes(source) &&
          property.ranges.includes(target)
        ) {
          pushRelation(source, target);
        }
      }
    }
  }

  const words = Array.from(
    new Set(
      (text.match(/[a-z][a-z_]{3,}/g) ?? []).filter(
        (word) => !STOP_WORDS.has(word),
      ),
    ),
  );
  const known = [
    ...vocabulary.classes.flatMap((entry) => entry.terms),
    ...vocabulary.properties.flatMap((entry) => entry.terms),
  ].map((term) => term.toLowerCase());
  const unmatched = words.filter(
    (word) => !known.some((term) => term.includes(word)),
  );

  return {
    elements: [
      ...classes.map((name) => `class:${name}`),
      ...properties,
      // A question naming five classes admits many declared pairs; the first few
      // in declaration order are the ones its phrasing implies.
      ...relations.slice(0, 6),
    ].join(' | '),
    classes,
    properties,
    relations: relations.slice(0, 6),
    unmatched: unmatched.slice(0, 8),
  };
};

/** One element the editor offers as a clickable suggestion. */
export type CompetencyElementSuggestion = {
  /** The clause this suggestion inserts into the elements field. */
  clause: string;
  kind: 'class' | 'property' | 'relation';
  /** What the row reads as: the declared name, or `owner.attribute`. */
  label: string;
  /** Other names the element is declared under (label, aliases). */
  aliases: string[];
  /** True when the current question already names it. */
  matched: boolean;
};

const SUGGESTION_LIMIT = 24;

/**
 * The model's elements, offered as a pick list.
 *
 * Tracing answers "which of these did the question name". It cannot answer
 * "which of these is the one I meant" when the question names none of them —
 * that happens whenever the declaration is English and the question is not, and
 * it is exactly where a hand-typed clause list used to start. So the vocabulary
 * itself is the suggestion: every declared element is one click away, ordered
 * with the ones the question names first.
 */
export const suggestCompetencyElements = (
  question: string,
  vocabulary: CompetencyVocabulary,
): CompetencyElementSuggestion[] => {
  const text = question.trim().toLowerCase();
  const aliasesOf = (type: string, terms: string[]): string[] =>
    terms
      .filter((term) => term.toLowerCase() !== type.toLowerCase())
      .slice(0, 2);
  const namedClasses = new Set(
    vocabulary.classes
      .filter(
        (entry) =>
          text !== '' && entry.terms.some((term) => namesTerm(text, term)),
      )
      .map((entry) => entry.type),
  );

  const rows: CompetencyElementSuggestion[] = [];
  for (const entry of vocabulary.classes) {
    rows.push({
      clause: `class:${entry.type}`,
      kind: 'class',
      label: entry.type,
      aliases: aliasesOf(entry.type, entry.terms),
      matched: namedClasses.has(entry.type),
    });
  }
  for (const property of vocabulary.properties) {
    const named =
      text !== '' && property.terms.some((term) => namesTerm(text, term));
    if (property.object) {
      for (const source of property.domains) {
        for (const target of property.ranges) {
          rows.push({
            clause: `relation:${source}->${target}`,
            kind: 'relation',
            label: `${property.type}: ${source} → ${target}`,
            aliases: [property.type],
            matched:
              named || (namedClasses.has(source) && namedClasses.has(target)),
          });
        }
      }
      continue;
    }
    for (const owner of property.domains) {
      rows.push({
        clause: `property:${owner}.${property.type}`,
        kind: 'property',
        label: `${owner}.${property.type}`,
        aliases: [property.type],
        matched: namedClasses.has(owner),
      });
    }
  }

  const matched = rows.filter((row) => row.matched);
  const rest = rows.filter((row) => !row.matched);
  return [...matched, ...rest].slice(0, SUGGESTION_LIMIT);
};

/** Adds a clause to the element list, or removes it when already present. */
export const toggleCompetencyElement = (
  elements: string,
  clause: string,
): string => {
  const parts = elements
    .split(/[|\n]/)
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
  const same = (part: string) =>
    part.replace(/^\?/, '').trim() === clause.trim();
  const next = parts.some(same)
    ? parts.filter((part) => !same(part))
    : [...parts, clause.trim()];
  return next.join(' | ');
};
