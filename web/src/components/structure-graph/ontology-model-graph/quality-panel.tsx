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
  IOntologyDroppedRelation,
  IOntologyGraph,
} from '@/interfaces/database/document-structure';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { pitfallCategoryLabel, pitfallLabel } from './pitfall-labels';

/**
 * One edit the reader confirmed on the ledger. It carries the documents the
 * finding came from, so the caller can re-compile exactly those instead of
 * guessing a scope.
 */
export type OntologyFixAction = {
  op: 'widen' | 'declare';
  property: string;
  side?: 'domain' | 'range';
  class?: string;
  domain?: string;
  range?: string;
  datatype?: string;
  docIds: string[];
};

/**
 * OntologyQualityPanel is the compile-quality ledger: what to change in the
 * TEMPLATE before this scope is good enough.
 *
 * It is a work queue, so it answers "what do I change" and not "how bad is it":
 * five rejected assertions of one property are ONE edit, and an observed
 * property is a declaration to add. A list of sentences would leave the reader
 * to work out the edit themselves, which is the same as no panel at all.
 *
 * It is also not a dashboard: the page keeps one line of it by default, because
 * the ontology view exists to be read as a graph.
 */
export function OntologyQualityPanel({
  graph,
  onFocusClass,
  onApplyFixes,
}: {
  graph?: IOntologyGraph;
  /** Optional: focus a class on the canvas, when the host can drive that. */
  onFocusClass?: (classType: string) => void;
  /**
   * Optional: apply the selected edits to the template, as one batch. The host
   * owns it because only the host knows the template, the scope and what to
   * re-compile; without it the ledger stays advice and shows no controls.
   *
   * A batch rather than one call per line, because what follows is a re-compile
   * and that is the expensive step — selecting five lines must not mean five
   * compiles.
   */
  onApplyFixes?: (fixes: OntologyFixAction[]) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // The selection holds the edits themselves rather than keys looked up again at
  // apply time: a lookup that misses is a batch that writes nothing, re-compiles
  // nothing, and says nothing.
  const [selected, setSelected] = useState<OntologyFixAction[]>([]);
  const [applying, setApplying] = useState(false);
  const [applyError, setApplyError] = useState('');
  const [applyNotice, setApplyNotice] = useState('');
  const quality = graph?.quality;
  if (!graph || !quality) return null;

  const classes = graph.classes ?? [];
  const properties = graph.properties ?? [];
  const classNames = new Set(classes.map((item) => item.type));

  // What the template declares for each property, per side. This is what turns a
  // rejection into an instruction: the observed class that the declaration does
  // not admit is the class the declaration is missing.
  const declaredSides = new Map<string, { domains: string[]; ranges: string[] }>();
  // Declared edges only. An observed edge the template does not declare would
  // otherwise feed its own endpoint back in as if it were declared, which hides
  // exactly the gap these maps are read to find.
  properties
    .filter((item) => item.declared)
    .forEach((item) => {
      const entry = declaredSides.get(item.type) ?? { domains: [], ranges: [] };
      if (item.source && !entry.domains.includes(item.source)) {
        entry.domains.push(item.source);
      }
      if (item.target && !entry.ranges.includes(item.target)) {
        entry.ranges.push(item.target);
      }
      declaredSides.set(item.type, entry);
    });

  // A property whose name the template declares is not an undeclared property,
  // even when the observed endpoint pair is not declared: offering "declare" for
  // it hands the reader an edit the writer refuses ("the template already
  // declares …"), which fails the whole batch.
  const undeclared = properties.filter(
    (item) => !item.declared && !item.declared_name,
  );
  const declaredClasses = new Set(
    classes.filter((item) => item.declared).map((item) => item.type),
  );
  const uncoveredClasses = classes.filter(
    (item) => item.declared && item.entities === 0,
  );
  // graph.properties carries one entry per (property, source, target), so the
  // same declaration shows up several times. The ledger names a declaration
  // once — a list that repeats `part_of` five times reads as five problems.
  // A property is unexercised only when NONE of its declared endpoint pairs was
  // seen. Reading the per-pair rows and naming the property reported
  // `participant_in` — the most used property of one document, 23 assertions —
  // as "never exercised", because fifteen of its sixteen declared pairs happened
  // to be empty. Pairs are coverage; the property is the verdict.
  const coverage = (() => {
    const declaredPairs = new Map<string, number>();
    const exercisedPairs = new Map<string, number>();
    const assertions = new Map<string, number>();
    const domains = new Map<string, string[]>();
    properties.forEach((item) => {
      if (!item.declared) return;
      declaredPairs.set(item.type, (declaredPairs.get(item.type) ?? 0) + 1);
      assertions.set(
        item.type,
        (assertions.get(item.type) ?? 0) + (item.relations ?? 0),
      );
      if ((item.relations ?? 0) > 0) {
        exercisedPairs.set(item.type, (exercisedPairs.get(item.type) ?? 0) + 1);
      }
      const seen = domains.get(item.type) ?? [];
      if (item.source && !seen.includes(item.source)) seen.push(item.source);
      domains.set(item.type, seen);
    });
    const total = (map: Map<string, number>) =>
      Array.from(map.values()).reduce((sum, value) => sum + value, 0);
    return {
      declaredPairs: total(declaredPairs),
      exercisedPairs: total(exercisedPairs),
      domains,
      assertions,
    };
  })();
  const uncoveredProperties = Array.from(coverage.domains.keys())
    .filter((type) => (coverage.assertions.get(type) ?? 0) === 0)
    .map((type) => ({ type, sources: coverage.domains.get(type) ?? [] }));

  const dropped = quality.dropped_samples ?? [];
  const pitfalls = graph.pitfalls ?? [];

  /**
   * One entry per EDIT, not per rejected assertion: five assertions of
   * `part_of` whose target class the declaration does not admit are a single
   * "add work to part_of's range". The side comes from comparing the observed
   * endpoint class against the declared ones, so nothing is parsed out of the
   * reason sentence.
   */
  const droppedActions = (() => {
    const groups = new Map<
      string,
      {
        key: string;
        property: string;
        side: 'domain' | 'range';
        observed: string;
        declared: string[];
        count: number;
        docIds: string[];
        example: IOntologyDroppedRelation;
      }
    >();
    dropped.forEach((item) => {
      const declared = declaredSides.get(item.property) ?? {
        domains: [],
        ranges: [],
      };
      const domainMissed =
        item.from_type !== undefined && !declared.domains.includes(item.from_type);
      const side: 'domain' | 'range' = domainMissed ? 'domain' : 'range';
      const observed = (side === 'domain' ? item.from_type : item.to_type) ?? '';
      const key = `${item.property}|${side}|${observed}`;
      const group = groups.get(key) ?? {
        key,
        property: item.property,
        side,
        observed,
        declared: side === 'domain' ? declared.domains : declared.ranges,
        count: 0,
        docIds: [],
        example: item,
      };
      group.count += 1;
      if (item.doc_id && !group.docIds.includes(item.doc_id)) {
        group.docIds.push(item.doc_id);
      }
      groups.set(key, group);
    });
    return Array.from(groups.values()).sort((a, b) => b.count - a.count);
  })();

  /**
   * A declared property observed with a pair the template does not declare. The
   * template is not missing the property, it is missing one endpoint of this
   * pair, so the edit that can be applied is the widen — the same edit the
   * rejected assertions get, which is why both feed the same `widen` payload.
   *
   * Widening is only offered when the class itself is declared: adding a class
   * the template does not have is refused as well, and the honest thing to show
   * is the gap without a button.
   */
  const endpointGaps = (() => {
    const groups = new Map<
      string,
      {
        key: string;
        property: string;
        side: 'domain' | 'range';
        klass: string;
        declared: string[];
        widenable: boolean;
        count: number;
      }
    >();
    properties
      .filter((item) => !item.declared && item.declared_name)
      .forEach((item) => {
        const sides = declaredSides.get(item.type) ?? {
          domains: [],
          ranges: [],
        };
        (
          [
            ['domain', item.source, sides.domains],
            ['range', item.target, sides.ranges],
          ] as const
        ).forEach(([side, klass, declared]) => {
          if (!klass || declared.includes(klass)) return;
          const key = `${item.type}|${side}|${klass}`;
          const group = groups.get(key) ?? {
            key,
            property: item.type,
            side,
            klass,
            declared,
            widenable: declaredClasses.has(klass),
            count: 0,
          };
          group.count += item.relations;
          groups.set(key, group);
        });
      });
    return Array.from(groups.values()).sort((a, b) => b.count - a.count);
  })();

  // How many subjects are spelled out per row. The numbers above stay complete.
  const listedLimit = 20;
  const uncovered =
    uncoveredClasses.length + uncoveredProperties.length + endpointGaps.length;
  // A zero is only evidence of quality when something was measured. When the
  // relation half of the graph is empty, "0 rejected · 0 undeclared" is an empty
  // set, not a pass — the reader would otherwise read a failed extraction as a
  // clean compile. A document normally yields relations; zero is a signal.
  const compiledInstances = classes.reduce(
    (total, item) => total + (item.entities ?? 0),
    0,
  );
  // Every edge with a count is an assertion that got compiled — declared or not,
  // because an undeclared pair is still something to look at.
  const measuredAssertions = properties.reduce(
    (sum, item) => sum + (item.relations ?? 0),
    0,
  );
  const nothingMeasured =
    measuredAssertions === 0 &&
    quality.dropped_relations === 0 &&
    quality.undeclared_properties === 0;
  const total =
    quality.undeclared_properties +
    quality.dropped_relations +
    quality.untyped_entities +
    quality.pitfalls +
    uncovered;

  const selectedFixes = selected;
  const hasActions =
    droppedActions.length > 0 ||
    undeclared.length > 0 ||
    endpointGaps.some((gap) => gap.widenable);

  const runBatch = async () => {
    if (!onApplyFixes || selected.length === 0) return;
    setApplying(true);
    setApplyError('');
    setApplyNotice('');
    try {
      await onApplyFixes(selectedFixes);
      setSelected([]);
      setApplyNotice(
        t('knowledgeCompilation.ontologyQualityApplied', {
          count: selectedFixes.length,
          defaultValue:
            'Template updated with {{count}} change(s). The affected documents are re-compiling; the numbers move when that finishes.',
        }),
      );
    } catch (error) {
      // The service refuses an edit the template cannot take, so the reader has
      // to see why rather than a button that quietly does nothing.
      setApplyError(String((error as { message?: string })?.message ?? error));
    } finally {
      setApplying(false);
    }
  };

  /** Identity of one edit, for de-duplicating a selection. */
  const fixKey = (fix: OntologyFixAction) =>
    `${fix.op}|${fix.property}|${fix.side ?? ''}|${fix.class ?? ''}|${
      fix.domain ?? ''
    }|${fix.range ?? ''}`;

  /**
   * Selecting IS the confirmation: the reader ticks every line they agree with,
   * then compiles once. There is no second step, because a second step that can
   * be interrupted — or reset by a refetch — is what turns a deliberate
   * selection into a click that does nothing.
   */
  const selectControl = (key: string, fix: OntologyFixAction): ReactNode => {
    if (!onApplyFixes) return null;
    const checked = selected.some((item) => fixKey(item) === fixKey(fix));
    return (
      <label className="flex cursor-pointer items-center gap-1 self-start text-text-secondary">
        <input
          type="checkbox"
          data-testid={`ontology-quality-select-${key}`}
          checked={checked}
          onChange={() =>
            setSelected((current) =>
              current.some((item) => fixKey(item) === fixKey(fix))
                ? current.filter((item) => fixKey(item) !== fixKey(fix))
                : [...current, fix],
            )
          }
        />
        {t('knowledgeCompilation.ontologyQualitySelect', {
          defaultValue: 'Select',
        })}
      </label>
    );
  };

  const batchBar = (): ReactNode => {
    if (!onApplyFixes || !hasActions) return null;
    const pending = selected.length > 0;
    return (
      <div
        className={`flex flex-wrap items-center gap-2 rounded border border-border-button px-2 py-1 ${
          pending ? 'bg-bg-base' : ''
        }`}
      >
        <span className={pending ? 'text-text-primary' : 'text-text-secondary'}>
          {t('knowledgeCompilation.ontologyQualitySelected', {
            count: selected.length,
            defaultValue: '{{count}} selected',
          })}
        </span>
        <button
          type="button"
          data-testid="ontology-quality-apply-batch"
          disabled={selected.length === 0 || applying}
          className="underline decoration-dotted disabled:opacity-50"
          onClick={() => void runBatch()}
        >
          {applying
            ? t('knowledgeCompilation.ontologyQualityApplying', {
                defaultValue: 'Writing the template…',
              })
            : t('knowledgeCompilation.ontologyQualityApplyBatch', {
                defaultValue: 'Apply and re-compile',
              })}
        </button>
        {pending && (
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyQualityApplyBatchHint', {
              defaultValue:
                '— writes the template, then re-compiles; re-compiling the document alone would change nothing.',
            })}
          </span>
        )}
        {selected.length > 0 && (
          <button
            type="button"
            data-testid="ontology-quality-clear-selection"
            className="text-text-secondary underline decoration-dotted"
            onClick={() => setSelected([])}
          >
            {t('knowledgeCompilation.ontologyQualityClearSelection', {
              defaultValue: 'Clear',
            })}
          </button>
        )}
      </div>
    );
  };

  const subject = (name: string, target?: string): ReactNode =>
    onFocusClass && target && classNames.has(target) ? (
      <button
        type="button"
        className="text-left underline decoration-dotted"
        onClick={() => onFocusClass(target)}
      >
        {name}
      </button>
    ) : (
      <span>{name}</span>
    );

  const row = (
    key: string,
    label: string,
    count: number,
    hint: string,
    body: ReactNode,
  ) => (
    <li
      key={key}
      data-testid={`ontology-quality-${key}`}
      className="flex flex-col"
    >
      <div className="flex flex-wrap items-baseline gap-x-2">
        <span className={count > 0 ? 'text-[#F0A020]' : 'text-text-secondary'}>
          {label}
        </span>
        <span className="font-medium text-text-primary">{count}</span>
        <span className="text-text-secondary">{hint}</span>
      </div>
      {count > 0 && (
        <div className="mt-0.5 flex max-h-24 flex-col gap-1 overflow-y-auto pl-3">
          {body}
        </div>
      )}
    </li>
  );

  const empty = (
    <span className="text-text-secondary">
      {t('knowledgeCompilation.ontologyQualityNoSubjects', {
        defaultValue: 'nothing to list',
      })}
    </span>
  );

  return (
    <section className="flex flex-col gap-1 text-xs">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <span
          data-testid="ontology-quality-line"
          className="font-medium text-text-primary"
        >
          {t('knowledgeCompilation.ontologyQualityLine', {
            undeclared: quality.undeclared_properties,
            rejected: quality.dropped_relations,
            untyped: quality.untyped_entities,
            findings: quality.pitfalls,
            uncovered,
            defaultValue:
              'Compile quality: {{undeclared}} undeclared · {{rejected}} rejected · {{untyped}} untyped · {{findings}} findings · {{uncovered}} never exercised',
          })}
        </span>
        {total === 0 && (
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyQualityPass', {
              defaultValue: '— nothing to fix in this scope.',
            })}
          </span>
        )}
        {nothingMeasured && (
          <span
            data-testid="ontology-quality-nothing-measured"
            className="text-[#F0A020]"
          >
            {t('knowledgeCompilation.ontologyQualityNothingMeasured', {
              entities: compiledInstances,
              defaultValue:
                '— no object-property assertion was compiled in this scope ({{entities}} instance(s) typed, 0 assertions): the zeros above are an empty set, not a clean compile.',
            })}
          </span>
        )}
        {selected.length > 0 && (
          // The selection outlives the detail being closed, and a reader who
          // selects and then re-compiles from the page would otherwise wait for
          // an effect that cannot happen: nothing was applied yet.
          <span
            data-testid="ontology-quality-pending"
            className="text-[#F0A020]"
          >
            {t('knowledgeCompilation.ontologyQualityPending', {
              count: selected.length,
              defaultValue: '· {{count}} edit(s) selected, not applied yet',
            })}
          </span>
        )}
        <button
          type="button"
          data-testid="ontology-quality-toggle"
          className="underline decoration-dotted"
          onClick={() => setOpen((current) => !current)}
        >
          {open
            ? t('knowledgeCompilation.ontologyQualityHide', {
                defaultValue: 'Hide what to change',
              })
            : t('knowledgeCompilation.ontologyQualityShow', {
                defaultValue: 'Show what to change',
              })}
        </button>
      </div>

      {open && (
        <div
          data-testid="ontology-quality-details"
          // The detail is a bounded surface of its own: it scrolls inside this
          // height instead of growing the header, which is what keeps the graph
          // measurable however long the lists get.
          className="flex max-h-[32vh] flex-col gap-1 overflow-y-auto rounded border border-border-button px-3 py-2"
        >
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyQualityWhereToFix', {
              defaultValue:
                'Tick the lines you accept, then press “Apply and re-compile”: these lines are edits to the template, and compiling is what moves the numbers. Re-compiling the document on its own changes nothing here.',
            })}
          </span>

          {batchBar()}

          <ul className="flex flex-col gap-1.5">
            {row(
              'dropped',
              t('knowledgeCompilation.ontologyQualityDropped', {
                defaultValue: 'Rejected assertions',
              }),
              quality.dropped_relations,
              t('knowledgeCompilation.ontologyQualityDroppedHint', {
                defaultValue:
                  '— the declaration is too narrow; each line is the edit that admits them.',
              }),
              droppedActions.length > 0
                ? [
                    ...droppedActions.map((action) => (
                      <div
                        key={action.key}
                        data-testid={`ontology-quality-action-${action.key}`}
                        // The compiler's own sentence stays as the tooltip, so a
                        // reader can check the finding without losing the reason.
                        title={action.example.reason}
                        className="flex flex-col"
                      >
                        <span>
                          {t(
                            'knowledgeCompilation.ontologyQualityActionWiden',
                            {
                              class: action.observed,
                              property: action.property,
                              side: action.side,
                              declared:
                                action.declared.join('|') ||
                                t(
                                  'knowledgeCompilation.ontologyQualityActionNothingDeclared',
                                  { defaultValue: 'nothing declared' },
                                ),
                              count: action.count,
                              defaultValue:
                                'Add “{{class}}” to {{property}}’s {{side}} (declared: {{declared}}) — covers {{count}} assertion(s)',
                            },
                          )}
                        </span>
                        <span className="text-text-secondary">
                          {action.example.from || '?'} →{' '}
                          {action.example.to || '?'}
                          {action.count > 1
                            ? ` +${action.count - 1}`
                            : ''}
                          {action.example.doc_id
                            ? ` · ${action.example.doc_id}`
                            : ''}
                        </span>
                        {selectControl(`widen|${action.key}`, {
                          op: 'widen',
                          property: action.property,
                          side: action.side,
                          class: action.observed,
                          docIds: action.docIds,
                        })}
                      </div>
                    )),
                    (quality.samples_truncated ||
                      dropped.length > listedLimit) && (
                      <span key="more" className="text-text-secondary">
                        {t('knowledgeCompilation.ontologyQualityDroppedMore', {
                          defaultValue:
                            'Only the first page of rejections is listed; the count above is complete.',
                        })}
                      </span>
                    ),
                  ]
                : empty,
            )}

            {row(
              'undeclared',
              t('knowledgeCompilation.ontologyQualityUndeclared', {
                defaultValue: 'Undeclared properties',
              }),
              quality.undeclared_properties,
              t('knowledgeCompilation.ontologyQualityUndeclaredHint', {
                defaultValue:
                  '— observed in the data; each line is the declaration to add.',
              }),
              undeclared.length > 0
                ? undeclared.slice(0, listedLimit).map((item) => (
                    <div key={item.type} className="flex flex-col">
                      <span>
                        {/* The subject is rendered, not interpolated: i18next
                            would stringify a node into "[object Object]". */}
                        {t('knowledgeCompilation.ontologyQualityActionDeclare', {
                          defaultValue: 'Declare',
                        })}{' '}
                        {subject(item.type, item.source)}{' '}
                        {t(
                          'knowledgeCompilation.ontologyQualityActionDeclareAs',
                          {
                            domain: item.source || '?',
                            range: item.target || '?',
                            defaultValue: '— observed as {{domain}} → {{range}}',
                          },
                        )}
                      </span>
                      {item.relations > 0 && (
                        <span className="text-text-secondary">
                          {t(
                            'knowledgeCompilation.ontologyQualityActionSeenTimes',
                            {
                              count: item.relations,
                              defaultValue:
                                '{{count}} assertion(s) already carry it',
                            },
                          )}
                        </span>
                      )}
                      {selectControl(`declare|${item.type}`, {
                        op: 'declare',
                        property: item.type,
                        domain: item.source,
                        range: item.target,
                        docIds: [],
                      })}
                    </div>
                  ))
                : empty,
            )}

            {row(
              'endpointGaps',
              t('knowledgeCompilation.ontologyQualityEndpointGaps', {
                defaultValue: 'Undeclared endpoints',
              }),
              endpointGaps.length,
              t('knowledgeCompilation.ontologyQualityEndpointGapsHint', {
                defaultValue:
                  '— the property is declared, this endpoint pair is not; each line widens a side.',
              }),
              endpointGaps.length > 0
                ? endpointGaps.slice(0, listedLimit).map((gap) => (
                    <div key={gap.key} className="flex flex-col">
                      <span>
                        {t(
                          'knowledgeCompilation.ontologyQualityActionWiden',
                          {
                            class: gap.klass,
                            property: gap.property,
                            side: gap.side,
                            declared:
                              gap.declared.join('|') ||
                              t(
                                'knowledgeCompilation.ontologyQualityActionNothingDeclared',
                                { defaultValue: 'nothing declared' },
                              ),
                            count: gap.count,
                            defaultValue:
                              'Add “{{class}}” to {{property}}’s {{side}} (declared: {{declared}})',
                          },
                        )}
                      </span>
                      {!gap.widenable && (
                        <span className="text-text-secondary">
                          {t('knowledgeCompilation.ontologyQualityActionClassUndeclared', {
                            class: gap.klass,
                            defaultValue:
                              '{{class}} is not declared either, so declare the class first.',
                          })}
                        </span>
                      )}
                      {gap.widenable &&
                        selectControl(`widenpair|${gap.key}`, {
                          op: 'widen',
                          property: gap.property,
                          side: gap.side,
                          class: gap.klass,
                          docIds: [],
                        })}
                    </div>
                  ))
                : empty,
            )}

            {row(
              'pitfalls',
              t('knowledgeCompilation.ontologyQualityPitfalls', {
                defaultValue: 'Declaration findings',
              }),
              quality.pitfalls,
              t('knowledgeCompilation.ontologyQualityPitfallsHint', {
                defaultValue:
                  '— problems in the template itself; fixed in the editor, no re-compile needed.',
              }),
              pitfalls.length > 0
                ? pitfalls.map((item) => (
                    <div key={item.code} className="flex flex-col">
                      <span className="text-text-secondary">
                        {pitfallCategoryLabel(item.category ?? 'structural', t)}
                      </span>
                      <span>{pitfallLabel(item.code, t)}</span>
                      {item.subjects?.length > 0 && (
                        <span className="text-text-secondary">
                          {item.subjects.map((name, at) => (
                            <span key={`${name}-${at}`}>
                              {at > 0 ? ', ' : ''}
                              {subject(name, classNames.has(name) ? name : undefined)}
                            </span>
                          ))}
                        </span>
                      )}
                    </div>
                  ))
                : empty,
            )}

            {row(
              'untyped',
              t('knowledgeCompilation.ontologyQualityUntyped', {
                defaultValue: 'Entities without a class',
              }),
              quality.untyped_entities,
              t('knowledgeCompilation.ontologyQualityUntypedHint', {
                defaultValue:
                  '— the extractor gave no class, so the rows cannot be placed. Sharpen the class’s rule, then re-compile.',
              }),
              empty,
            )}

            {row(
              'coverage',
              t('knowledgeCompilation.ontologyQualityCoverage', {
                defaultValue: 'Never exercised',
              }),
              // Counted from the lists below, not from the backend totals: the
              // backend counts a property once per declared (source, target)
              // pair, so its number would not match the names the reader sees.
              uncovered,
              t('knowledgeCompilation.ontologyQualityCoverageHint', {
                exercised: coverage.exercisedPairs,
                declared: coverage.declaredPairs,
                defaultValue:
                  '— {{exercised}} of {{declared}} declared endpoint pairs were seen here. The names below had no assertion at all; a single document normally has no facts for most of them, so check the dataset scope before changing anything.',
              }),
              uncoveredClasses.length + uncoveredProperties.length > 0
                ? [
                    ...uncoveredClasses.map((item) => (
                      <span key={`c-${item.type}`}>
                        {subject(item.type, item.type)}
                      </span>
                    )),
                    ...uncoveredProperties.slice(0, listedLimit).map((item) => (
                      <span key={`p-${item.type}`}>
                        {subject(item.type, item.sources[0])}
                      </span>
                    )),
                  ]
                : empty,
            )}
          </ul>

          {applyNotice && (
            <span className="text-text-secondary">{applyNotice}</span>
          )}

          {applyError && (
            <span className="text-[#F0A020]">
              {t('knowledgeCompilation.ontologyQualityApplyFailed', {
                message: applyError,
                defaultValue: 'The template was not changed: {{message}}',
              })}
            </span>
          )}
        </div>
      )}
    </section>
  );
}

export default OntologyQualityPanel;
