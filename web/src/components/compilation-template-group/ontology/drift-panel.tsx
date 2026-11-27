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

import { useState, type MouseEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  IOntologyInsight,
  IOntologyInsightClass,
  IOntologyInsightField,
} from '@/interfaces/database/ontology-insight';
import { IOntologyModelPendingChanges } from '@/interfaces/database/ontology-model';

/** The measurement time, rendered the way a reader checks it. */
const formatAsOf = (milliseconds: number): string =>
  milliseconds ? new Date(milliseconds).toLocaleString() : '—';

const percent = (rate: number): string => `${Math.round((rate || 0) * 100)}%`;

/**
 * The field that explains the finding: the one filled in least often, because
 * that is the attribute the corpus has values for but the model never asked for.
 */
const weakestField = (
  fields: IOntologyInsightField[] | undefined,
): IOntologyInsightField | undefined =>
  (fields ?? []).reduce<IOntologyInsightField | undefined>(
    (worst, field) =>
      !worst || field.fill_rate < worst.fill_rate ? field : worst,
    undefined,
  );

/** A row the reader can point at, across the three findings the panel shows. */
const rowKey = (kind: string, name: string) => `${kind}:${name}`;

/**
 * Below this fill rate an attribute is a finding. The draft's own example is a
 * field the corpus fills about two thirds of the time, so the line sits under
 * that: the panel exists to surface gaps, not to reprint every rate.
 */
const FILL_PROBLEM_THRESHOLD = 0.5;

type Props = {
  insight: IOntologyInsight | null;
  loading: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  /**
   * Declares the class, with the attributes the corpus already fills.
   *
   * The row knows both halves of the finding, so the write does too: declaring
   * the name alone would leave the same attribute to be added by hand, and the
   * panel would report it again on the next measurement.
   */
  onDeclare?: (className: string, attributes: string[]) => void;
  /** Removes a declared class the corpus never produced. */
  onRemoveClass?: (className: string) => void;
  /** Removes a declared property that produced no assertion. */
  onRemoveProperty?: (propertyType: string) => void;
  /** Opens the description / value-set fix for an attribute filled too rarely. */
  onDescribeProperty?: (className: string, propertyType: string) => void;
  /** Opens the description fix for a declared class the corpus never produced. */
  onDescribeClass?: (className: string) => void;
  /** Opens the class in the canvas. */
  onLocate?: (className: string) => void;
  /**
   * Rows of the class the reader just located. Present only for that one class:
   * the sample block answers "what do these 18,401 rows look like", which is a
   * question about one finding, not a list to preload for every row.
   */
  samples?: {
    className: string;
    rows: { id: string; name: string; description?: string }[];
    total: number;
  } | null;
  /** Records "seen and accepted as-is"; the reason is required by the caller. */
  onDismiss?: (className: string) => void;
  /**
   * Classes already judged, filtered out of the lists. The findings stay in the
   * stored measurement — this is a display decision, so lifting a dismissal
   * brings the row back without waiting for a recompile.
   */
  dismissedClasses?: Set<string>;
  /** The same rows as `dismissedClasses`, with the reason they were set aside. */
  dismissals?: { className: string; reason: string }[];
  /** Undoes a dismissal, putting the finding back in its list. */
  onRestore?: (className: string) => void;
  /** Opens the review queue filtered to the class. */
  onReview?: (className: string) => void;
  /**
   * The draft against the published version. The panel uses it for one thing:
   * saying which stage the declarations just made are in.
   */
  pending?: IOntologyModelPendingChanges | null;
  /** Opens the change list. Omitted where there is nowhere to show it. */
  onViewChanges?: () => void;
  /** Moves the draft to review. Omitted for readers who cannot submit. */
  onSubmitReview?: () => void;
  disabled?: boolean;
};

/**
 * The draft-against-published bar.
 *
 * It says which stage the declarations just made are in; without it a Declare
 * looks like it did nothing, because the measurement keeps describing the
 * published version.
 */
export const DriftChangeBar = ({
  pending,
  onViewChanges,
  onSubmitReview,
}: {
  pending: IOntologyModelPendingChanges;
  onViewChanges?: () => void;
  onSubmitReview?: () => void;
}) => {
  const { t } = useTranslation();
  const count = pending.changes?.length ?? 0;

  return (
    <div className="shrink-0 border-t border-border-button bg-bg-base px-3 py-2">
      {/* `flex-wrap` is load-bearing: the panel is 292px wide, and the two
          actions plus the count line do not fit on one row — without wrapping
          they overflowed past the panel's clipped edge, and "Submit for review"
          ended up outside the viewport where nothing can click it. */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded bg-state-warning/10 px-1.5 py-0.5 text-[11px] text-state-warning">
          {t('knowledgeCompilation.ontologyChangeUnpublished', {
            defaultValue: 'unpublished',
          })}
        </span>
        <span className="text-xs text-text-secondary">
          {t('knowledgeCompilation.ontologyChangeCount', {
            defaultValue: '{{count}} change(s) in the draft',
            count,
          })}
        </span>
        <div className="ml-auto flex items-center gap-1">
          {onViewChanges && (
            <Button variant="ghost" size="sm" onClick={onViewChanges}>
              {t('knowledgeCompilation.ontologyChangeView', {
                defaultValue: 'View changes',
              })}
            </Button>
          )}
          {onSubmitReview && (
            <Button size="sm" onClick={onSubmitReview}>
              {t('knowledgeCompilation.ontologyChangeSubmit', {
                defaultValue: 'Submit for review',
              })}
            </Button>
          )}
        </div>
      </div>
      {/* The sentence that stops "Declare did nothing" from being the reading:
          rows are compiled, and only a published version is compiled. */}
      <p className="mt-1 text-[11px] leading-4 text-text-disabled">
        {t('knowledgeCompilation.ontologyChangeRecompileNote', {
          defaultValue:
            'Publishing is not enough: this dataset has to be recompiled before these rows change.',
        })}
      </p>
    </div>
  );
};

/**
 * What the corpus says about the declaration, in the two directions it can
 * disagree.
 *
 * The panel is a monitor, not an editor: it can say "this class is in the rows
 * and not in the model" or "this class is in the model and in no row". It
 * cannot say what should be built — that is the competency questions' job — so
 * the only write it performs is the one the finding already names: declare the
 * class, or record why it is not a gap.
 *
 * The structure follows the review draft: group headers with counts, one row
 * per finding (a dot, the name, one line of evidence), and the samples inline
 * under the row the reader selected — never preloaded for every row, because
 * that would be one query per finding to answer a question about one.
 */
export const OntologyDriftPanel = ({
  insight,
  loading,
  refreshing,
  onRefresh,
  onDeclare,
  onRemoveClass,
  onRemoveProperty,
  onDescribeProperty,
  onDescribeClass,
  onLocate,
  samples,
  onDismiss,
  dismissedClasses,
  dismissals,
  onRestore,
  onReview,
  pending,
  onViewChanges,
  onSubmitReview,
  disabled,
}: Props) => {
  const { t } = useTranslation();
  const drift = insight?.payload?.drift;
  // Which finding the reader is looking at. It drives three things at once: the
  // highlighted row, the class located on the canvas, and the subject of the
  // two footer actions — a button with no subject is worse than a disabled one.
  // A relation row is not dismissable: a dismissal is recorded per class, and
  // writing a relation's name into that slot would silence a class of the same
  // name in this dataset.
  const [active, setActive] = useState<{
    key: string;
    className: string;
    dismissable: boolean;
  }>({ key: '', className: '', dismissable: false });

  const renderHead = (count?: number) => (
    <div className="flex shrink-0 items-center gap-1.5 border-b border-border-button px-3 py-2.5 text-xs font-semibold">
      <span className="text-text-primary">
        {t('knowledgeCompilation.ontologyDriftTitle', {
          defaultValue: 'Schema drift',
        })}
      </span>
      {!!count && (
        <span className="rounded border border-state-warning/30 bg-state-warning/10 px-1.5 text-[10.5px] font-normal text-state-warning">
          {t('knowledgeCompilation.ontologyDriftFindingsCount', {
            defaultValue: '{{count}} findings',
            count,
          })}
        </span>
      )}
      {drift?.truncated && (
        <span className="rounded border border-state-warning/30 bg-state-warning/10 px-1.5 text-[10.5px] font-normal text-state-warning">
          {t('knowledgeCompilation.ontologyDriftTruncated', {
            defaultValue: 'partial scan',
          })}
        </span>
      )}
      <span className="ml-auto flex items-center gap-1.5">
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-[11px] font-normal"
          disabled={refreshing}
          onClick={onRefresh}
        >
          {refreshing
            ? t('knowledgeCompilation.ontologyDriftRunning', {
                defaultValue: 'Measuring...',
              })
            : t('knowledgeCompilation.ontologyCqRunCheck', {
                defaultValue: 'Run check',
              })}
        </Button>
      </span>
    </div>
  );

  if (!drift) {
    return (
      <div className="flex h-full min-h-0 flex-col bg-bg-component">
        {renderHead()}
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 text-center">
          <p className="text-xs leading-5 text-text-secondary">
            {loading
              ? t('common.loading', { defaultValue: 'Loading...' })
              : t('knowledgeCompilation.ontologyDriftUnmeasured', {
                  defaultValue:
                    'This dataset has not been measured against the model yet. A check compares the compiled rows with the declaration.',
                })}
          </p>
        </div>
      </div>
    );
  }

  const hidden = (type: string) => !!dismissedClasses?.has(type);

  const undeclared = (drift.undeclared_classes ?? []).filter(
    (item) => !hidden(item.type),
  );
  const unusedClasses = (drift.unused_classes ?? []).filter(
    (item) => !hidden(item.type),
  );
  // A declared relation with no assertion is the same finding as a declared
  // class with no instance — the corpus never exercised the declaration — so it
  // shares that group instead of adding a fourth one the draft does not have.
  const unusedRelations = drift.unused_properties ?? [];
  // "Fill problem" is a threshold, not a table: an attribute the corpus fills
  // for most instances is not news, and listing every rate would bury the rows
  // that are.
  const fillProblems = (drift.classes ?? [])
    .filter((item) => !hidden(item.type))
    .map((item) => ({
      item,
      weak: (item.fields ?? []).filter(
        (field) => field.fill_rate < FILL_PROBLEM_THRESHOLD,
      ),
    }))
    .filter((row) => row.weak.length > 0);

  const findings =
    undeclared.length +
    unusedClasses.length +
    unusedRelations.length +
    fillProblems.length;

  const group = (title: string, count: number) => (
    <div className="px-3 pb-0.5 pt-1.5 text-[10.5px] tracking-wide text-text-disabled">
      {title} ({count})
    </div>
  );

  const select = (
    key: string,
    className: string,
    dismissable: boolean = true,
  ) => {
    setActive({ key, className, dismissable });
    onLocate?.(className);
  };

  const row = (
    key: string,
    className: string,
    meta: string,
    why: string,
    tone: 'amber' | 'gray' = 'amber',
    locate: string = className,
    dismissable: boolean = true,
    action?: ReactNode,
  ) => (
    <div
      key={key}
      role="button"
      tabIndex={0}
      className={cn(
        'flex cursor-pointer items-start gap-[7px] px-3 py-1.5 text-[11.5px] hover:bg-bg-card',
        active.key === key &&
          'bg-state-warning/5 shadow-[inset_2px_0_0_rgb(var(--state-warning))]',
      )}
      onClick={() => select(key, locate, dismissable)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          select(key, locate, dismissable);
        }
      }}
    >
      <span
        className={cn(
          'mt-1.5 size-1.5 shrink-0 rounded-full',
          tone === 'amber' ? 'bg-state-warning' : 'bg-text-disabled',
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-1.5">
          <span className="truncate font-semibold text-text-primary">
            {className}
          </span>
          <span className="shrink-0 text-[10.5px] text-text-disabled">
            {meta}
          </span>
          {/* The fix lives on the row, not behind a selection: a finding the
              reader cannot act on is a finding they will stop reading. */}
          {action && <span className="ml-auto shrink-0">{action}</span>}
        </div>
        <div className="text-[10.5px] leading-4 text-text-secondary">{why}</div>
      </div>
    </div>
  );

  const samplesBlock = (item: IOntologyInsightClass) => {
    if (samples?.className !== item.type) {
      return null;
    }
    return (
      <div className="mx-3 mb-2 ml-[25px] rounded-md border border-border-button bg-bg-base px-2 py-1.5 text-[10.5px] leading-[1.7] text-text-secondary">
        {samples.rows.slice(0, 3).map((sample) => (
          <div key={sample.id} className="flex justify-between gap-2">
            <span className="truncate">{sample.name}</span>
            <span className="shrink-0 text-text-disabled">
              {sample.description?.slice(0, 24) ?? ''}
            </span>
          </div>
        ))}
        {samples.rows.length === 0 && (
          <div className="text-text-disabled">
            {t('knowledgeCompilation.ontologyDriftSampleEmpty', {
              defaultValue: 'No rows loaded for this class.',
            })}
          </div>
        )}
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
          <Button
            variant="outline"
            size="sm"
            className="h-6 px-2 text-[11px]"
            onClick={() => onLocate?.(item.type)}
          >
            {t('knowledgeCompilation.ontologyDriftSeeSamples', {
              defaultValue: 'See these {{count}}',
              count: samples.total,
            })}
          </Button>
        </div>
      </div>
    );
  };

  return (
    <div className="flex h-full min-h-0 flex-col bg-bg-base">
      {renderHead(findings)}
      {/* The loop, in one line: a finding is only worth listing if the reader can
          turn it into a draft edit, and a draft edit only matters after publish. */}
      <div className="shrink-0 border-b border-border-button px-3 py-1 text-[10.5px] leading-4 text-text-disabled">
        {t('knowledgeCompilation.ontologyDriftFixHint', {
          defaultValue:
            'Each action writes to the draft. These rows keep describing the published version until it is published and recompiled.',
        })}
      </div>
      <div className="min-h-0 flex-1 overflow-auto py-1.5">
        {group(
          t('knowledgeCompilation.ontologyDriftGroupUndeclared', {
            defaultValue: 'Not declared',
          }),
          undeclared.length,
        )}
        {undeclared.map((item) => {
          const weak = weakestField(item.fields);
          return (
            <div key={item.type}>
              {row(
                rowKey('undeclared', item.type),
                item.type,
                t('knowledgeCompilation.ontologyDriftRowEntities', {
                  defaultValue: '{{count}} rows',
                  count: item.entities,
                }),
                weak
                  ? t('knowledgeCompilation.ontologyDriftUndeclaredField', {
                      defaultValue:
                        'Field {{field}} filled in {{percent}} · the class is not declared',
                      field: weak.type,
                      percent: percent(weak.fill_rate),
                    })
                  : t('knowledgeCompilation.ontologyDriftUndeclaredNoField', {
                      defaultValue:
                        'These rows carry a type the model does not declare',
                    }),
                'amber',
                item.type,
                true,
                onDeclare && (
                  <Button
                    size="sm"
                    className="h-6 px-2 text-[11px]"
                    disabled={disabled}
                    onClick={(event: MouseEvent) => {
                      event.stopPropagation();
                      onDeclare(
                        item.type,
                        (item.fields ?? [])
                          .map((field) => field.type)
                          .filter(Boolean),
                      );
                    }}
                  >
                    {t('knowledgeCompilation.ontologyDriftDeclare', {
                      defaultValue: 'Declare {{name}}',
                      name: item.type,
                    })}
                  </Button>
                ),
              )}
              {samplesBlock(item)}
            </div>
          );
        })}

        {group(
          t('knowledgeCompilation.ontologyDriftGroupUnused', {
            defaultValue: 'Declared, zero instances',
          }),
          unusedClasses.length + unusedRelations.length,
        )}
        {unusedClasses.map((item) =>
          row(
            rowKey('unused', item.type),
            item.type,
            t('knowledgeCompilation.ontologyDriftRowEntities', {
              defaultValue: '{{count}} rows',
              count: 0,
            }),
            t('knowledgeCompilation.ontologyDriftUnusedWhy', {
              defaultValue: 'Declared, but none of the {{count}} rows is one',
              count: drift.total_entities,
            }),
            'gray',
            item.type,
            true,
            (onDescribeClass || onRemoveClass) && (
              <span className="flex items-center gap-1">
                {onDescribeClass && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-6 px-2 text-[11px]"
                    disabled={disabled}
                    onClick={(event: MouseEvent) => {
                      event.stopPropagation();
                      onDescribeClass(item.type);
                    }}
                  >
                    {t('knowledgeCompilation.ontologyDriftDescribe', {
                      defaultValue: 'Describe',
                    })}
                  </Button>
                )}
                {onRemoveClass && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 px-2 text-[11px] text-state-error"
                    disabled={disabled}
                    onClick={(event: MouseEvent) => {
                      event.stopPropagation();
                      onRemoveClass(item.type);
                    }}
                  >
                    {t('knowledgeCompilation.ontologyDriftRemoveClass', {
                      defaultValue: 'Delete class',
                    })}
                  </Button>
                )}
              </span>
            ),
          ),
        )}
        {unusedRelations.map((item) =>
          row(
            rowKey('relation', `${item.source}-${item.type}-${item.target}`),
            `${item.source} → ${item.target}`,
            item.type,
            t('knowledgeCompilation.ontologyDriftRelationUnusedWhy', {
              defaultValue: 'Declared, but produced no assertion',
            }),
            'gray',
            // A relation has no class to dismiss; locating the source class is
            // the closest honest answer for the click.
            item.source,
            false,
            onRemoveProperty && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-[11px] text-state-error"
                disabled={disabled}
                onClick={(event: MouseEvent) => {
                  event.stopPropagation();
                  onRemoveProperty(item.type);
                }}
              >
                {t('knowledgeCompilation.ontologyDriftRemoveProperty', {
                  defaultValue: 'Delete relation',
                })}
              </Button>
            ),
          ),
        )}

        {group(
          t('knowledgeCompilation.ontologyDriftGroupFields', {
            defaultValue: 'Attribute fill problems',
          }),
          fillProblems.length,
        )}
        {fillProblems.map(({ item, weak }) => (
          <div key={item.type}>
            {row(
              rowKey('fields', item.type),
              item.type,
              t('knowledgeCompilation.ontologyDriftRowEntities', {
                defaultValue: '{{count}} rows',
                count: item.entities,
              }),
              weak
                .slice(0, 2)
                .map((field) => `${field.type} ${percent(field.fill_rate)}`)
                .join(' · '),
            )}
            {/* The fix for a low fill rate is per attribute: a description of
                what the value looks like, or a closed set of allowed values.
                Both are the only things on a property that reach the prompt. */}
            {active.key === rowKey('fields', item.type) && (
              <div className="mx-3 mb-2 ml-[25px] flex flex-col gap-1 rounded-md border border-border-button bg-bg-base px-2 py-1.5 text-[10.5px] text-text-secondary">
                {weak.map((field) => (
                  <div key={field.type} className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono">
                      {item.type}.{field.type}
                    </span>
                    <span className="shrink-0 text-text-disabled">
                      {percent(field.fill_rate)}
                    </span>
                    {onDescribeProperty && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-6 shrink-0 px-2 text-[11px]"
                        disabled={disabled}
                        onClick={() =>
                          onDescribeProperty(item.type, field.type)
                        }
                      >
                        {t('knowledgeCompilation.ontologyDriftDescribe', {
                          defaultValue: 'Describe',
                        })}
                      </Button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}

        {/* The classes removed from the lists above are listed here with their
            reason: hiding a row without a way back makes the panel's numbers
            unaccountable, and a decision nobody can read back is not a record. */}
        {(dismissals?.length ?? 0) > 0 && (
          <>
            {group(
              t('knowledgeCompilation.ontologyDriftDismissed', {
                defaultValue: 'Set aside',
              }),
              dismissals?.length ?? 0,
            )}
            <div className="px-3 pb-1 text-[10.5px] leading-4 text-text-disabled">
              {t('knowledgeCompilation.ontologyDriftDismissedHint', {
                defaultValue:
                  'Still in the measurement, hidden in this dataset only; recompiling does not clear them. Lift one to see its row again.',
              })}
            </div>
            {(dismissals ?? []).map((item) => (
              <div
                key={item.className}
                className="flex items-start gap-[7px] px-3 py-1.5 text-[11.5px]"
              >
                <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-text-disabled" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline gap-1.5">
                    <span
                      className="truncate font-semibold text-text-secondary"
                      title={item.className}
                    >
                      {item.className}
                    </span>
                    {onRestore && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="ml-auto h-6 px-2 text-[11px] font-normal"
                        disabled={disabled}
                        onClick={() => onRestore(item.className)}
                      >
                        {t('knowledgeCompilation.ontologyDriftRestore', {
                          defaultValue: 'Lift',
                        })}
                      </Button>
                    )}
                  </div>
                  <div className="text-[10.5px] leading-4 text-text-disabled">
                    {item.reason}
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      {/* What the numbers describe, in one line, because "as of this compile"
          has to be checkable rather than assumed. */}
      <div className="shrink-0 border-t border-border-button px-3 py-1.5 text-[10.5px] leading-4 text-text-disabled">
        <div>
          {t('knowledgeCompilation.ontologyDriftSnapshot', {
            defaultValue: 'Snapshot v{{version}}',
            version: insight.version || '—',
          })}
          {' · '}
          {formatAsOf(insight.as_of)}
        </div>
        <div>
          {t('knowledgeCompilation.ontologyDriftTotals', {
            defaultValue: '{{entities}} entities · {{relations}} relations',
            entities: drift.total_entities,
            relations: drift.total_relations,
          })}
          {drift.dropped_relations > 0 &&
            ` · ${t('knowledgeCompilation.ontologyDriftDropped', {
              defaultValue: '{{count}} dropped',
              count: drift.dropped_relations,
            })}`}
          {drift.untyped_entities > 0 &&
            ` · ${t('knowledgeCompilation.ontologyDriftUntyped', {
              defaultValue: '{{count}} untyped',
              count: drift.untyped_entities,
            })}`}
        </div>
        <div>
          {t('knowledgeCompilation.ontologyDriftBasis', {
            defaultValue:
              'basis: entity type / (from, relation, to) / field fill rate',
          })}
        </div>
      </div>

      {/* The one thing that makes this panel look broken: compilation reads the
          published snapshot, so an unpublished edit changes no row and the
          numbers stay put no matter how often the check is run. */}
      {insight.draft_pending && (
        <div className="shrink-0 border-t border-border-button bg-state-warning/10 px-3 py-1.5 text-[10.5px] leading-4 text-state-warning">
          {t('knowledgeCompilation.ontologyDriftDraftPending', {
            defaultValue:
              'The draft has unpublished edits. Only a published version is compiled, so recompiling changes nothing until you publish.',
          })}
        </div>
      )}

      {(onDismiss || onReview) && (
        <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-t border-border-button px-3 py-2.5">
          {onDismiss && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={!active.dismissable || disabled}
              onClick={() => active.dismissable && onDismiss(active.className)}
            >
              {t('knowledgeCompilation.ontologyDriftDismiss', {
                defaultValue: 'Ignore with a reason',
              })}
            </Button>
          )}
          {onReview && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={!active.key}
              onClick={() => active.key && onReview(active.className)}
            >
              {t('knowledgeCompilation.ontologyDriftReview', {
                defaultValue: 'Open review queue',
              })}
            </Button>
          )}
        </div>
      )}

      {pending?.draft_pending && (
        <DriftChangeBar
          pending={pending}
          onViewChanges={onViewChanges}
          onSubmitReview={onSubmitReview}
        />
      )}
    </div>
  );
};
