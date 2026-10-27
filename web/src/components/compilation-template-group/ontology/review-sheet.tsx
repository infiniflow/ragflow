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
  BulkOperateBar,
  BulkOperateItemType,
} from '@/components/bulk-operate-bar';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import message from '@/components/ui/message';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Textarea } from '@/components/ui/textarea';
import {
  useFetchAssertionReview,
  useReviewAssertions,
} from '@/hooks/use-compilation-model-request';
import {
  AssertionReviewAction,
  AssertionReviewStatus,
  IAssertionReviewItem,
} from '@/interfaces/database/compilation-model';
import { IAssertionReviewRequestParams } from '@/interfaces/request/compilation-model';
import { cn } from '@/lib/utils';
import { Check, RotateCcw, X } from 'lucide-react';
import {
  ChangeEvent,
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';

import { OntologyEvidenceView } from './evidence-sheet';

interface OntologyReviewSheetProps {
  /** The saved template's id. A model that was never saved has no assertions. */
  templateId?: string;
  /** Controlled usage: a caller that passes `open` owns the state. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** Rendered inside the sheet's trigger, so the sheet owns its own opener. */
  trigger?: ReactNode;
  /** Declared classes / properties, so the filters can offer what is not on screen. */
  classes?: string[];
  properties?: string[];
}

/**
 * Which rows the queue should hold. Not the same as what a row carries:
 * "unreviewed" is a filter, never a verdict.
 */
const DefaultReviewStatus: AssertionReviewStatus = 'unreviewed';

const ReviewPageSize = 50;

/** Stable empty page, so an unloaded queue does not look like a new page each render. */
const EmptyItems: IAssertionReviewItem[] = [];

/**
 * A row's identity in the queue.
 *
 * It is the dataset and the id together: an assertion id is unique inside its
 * dataset, and the same subject is compiled into every dataset that mentions it,
 * so keying by the id alone would make two rows share a checkbox.
 */
const rowKey = (item: IAssertionReviewItem) =>
  `${item.dataset_id}\u0000${item.id}`;

interface AssertionGroup {
  key: string;
  /** What the rows share — one subject and one property. */
  heading?: string;
  items: IAssertionReviewItem[];
}

/**
 * Groups adjacent rows that assert the same (from, property) pair.
 *
 * Conflict only means something inside such a group: two rows that disagree are
 * only decidable side by side, and separated by unrelated rows the reader would
 * have to hold both in mind at once. Grouping neighbours rather than sorting the
 * whole page is what keeps the queue's own order — by dataset, then by version —
 * intact, so a row does not move when a verdict is recorded.
 */
const groupItems = (items: IAssertionReviewItem[]): AssertionGroup[] => {
  const groups: AssertionGroup[] = [];
  items.forEach((item, index) => {
    const shared = item.from && item.property;
    const key = shared ? `${item.from}\u0000${item.property}` : `row-${index}`;
    const previous = groups[groups.length - 1];
    if (previous && previous.key === key) {
      previous.items.push(item);
      return;
    }
    groups.push({
      key,
      heading: shared ? `${item.from} · ${item.property}` : undefined,
      items: [item],
    });
  });
  return groups;
};

const uniqueValues = (values: string[]) => Array.from(new Set(values)).sort();

/**
 * Judges the assertions a model extracted.
 *
 * The queue answers a question the inspection panel cannot: an extracted row is
 * not evidence that the model is right, and whether it is right can only be
 * decided against the text it was read from. So the two panels are the same
 * reading of the corpus with different work attached — one shows what the model
 * says, this one records what a reviewer decided about it.
 *
 * Verdicts are recorded per row and the server answers per row, so a batch that
 * partly lands leaves exactly the failed rows selected with the reason beside
 * them: retrying them must not mean finding them again.
 */
export function OntologyReviewSheet({
  templateId,
  open,
  onOpenChange,
  trigger,
  classes,
  properties,
}: OntologyReviewSheetProps) {
  const { t } = useTranslation();
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const [status, setStatus] =
    useState<AssertionReviewStatus>(DefaultReviewStatus);
  const [classType, setClassType] = useState('');
  const [property, setProperty] = useState('');
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [activeKey, setActiveKey] = useState('');
  const [note, setNote] = useState('');
  /** Reasons from the last batch, kept per row so the reader can see which failed. */
  const [failures, setFailures] = useState<Record<string, string>>({});

  // A caller that passes `open` owns it; one that only passes a trigger does not.
  const sheetOpen = open ?? uncontrolledOpen;
  const handleOpenChange = useCallback(
    (next: boolean) => {
      setUncontrolledOpen(next);
      onOpenChange?.(next);
    },
    [onOpenChange],
  );

  const params = useMemo<IAssertionReviewRequestParams>(
    () => ({
      status,
      class: classType || undefined,
      property: property || undefined,
      limit: ReviewPageSize,
    }),
    [status, classType, property],
  );

  const { queue, loading } = useFetchAssertionReview(
    templateId,
    params,
    sheetOpen,
  );
  const items = queue?.items ?? EmptyItems;

  const { loading: submitting, reviewAssertions: submitReview } =
    useReviewAssertions();

  // Rows leave the queue when they are judged, so a selection can name rows the
  // current page no longer holds. Dropping them keeps "select all" meaning the
  // rows in front of the reader.
  useEffect(() => {
    const present = new Set(items.map(rowKey));
    setSelectedKeys((current) => {
      const kept = current.filter((key) => present.has(key));
      return kept.length === current.length ? current : kept;
    });
  }, [items]);

  const activeItem = useMemo(
    () => items.find((item) => rowKey(item) === activeKey) ?? items[0],
    [items, activeKey],
  );

  const selectedItems = useMemo(
    () => items.filter((item) => selectedKeys.includes(rowKey(item))),
    [items, selectedKeys],
  );

  /**
   * The model's declared classes and properties, plus whatever this page shows.
   *
   * The declared list is what a filter is for — finding a class that is not on
   * the current page — while the page's own values keep the filter usable when
   * the sheet was opened without the declaration.
   */
  const classOptions = useMemo(
    () =>
      uniqueValues([
        ...(classes ?? []),
        ...items
          .filter((item) => item.kind === 'entity')
          .map((item) => item.type ?? '')
          .filter(Boolean),
      ]).map((name) => ({ label: name, value: name })),
    [classes, items],
  );

  const propertyOptions = useMemo(
    () =>
      uniqueValues([
        ...(properties ?? []),
        ...items
          .filter((item) => item.kind === 'relation')
          .map((item) => item.property ?? '')
          .filter(Boolean),
      ]).map((name) => ({ label: name, value: name })),
    [properties, items],
  );

  const statusOptions = useMemo(
    () => [
      {
        label: t('knowledgeCompilation.ontologyInspect.reviewUnreviewed', {
          defaultValue: 'Unreviewed',
        }),
        value: 'unreviewed',
      },
      {
        label: t('knowledgeCompilation.ontologyInspect.reviewAccepted', {
          defaultValue: 'Accepted',
        }),
        value: 'accepted',
      },
      {
        label: t('knowledgeCompilation.ontologyInspect.reviewRejected', {
          defaultValue: 'Rejected',
        }),
        value: 'rejected',
      },
      {
        label: t('knowledgeCompilation.ontologyInspect.reviewAll', {
          defaultValue: 'All',
        }),
        value: 'all',
      },
    ],
    [t],
  );

  const groups = useMemo(() => groupItems(items), [items]);

  const submit = useCallback(
    async (action: AssertionReviewAction, targets: IAssertionReviewItem[]) => {
      if (!templateId || targets.length === 0) return;
      const outcome = await submitReview({
        id: templateId,
        items: targets.map((item) => ({
          id: item.id,
          dataset_id: item.dataset_id,
          action,
          // A note belongs to a rejection: an acceptance has nothing to explain,
          // and a reset takes the explanation away with the verdict.
          note: action === 'reject' ? note.trim() || undefined : undefined,
        })),
      });

      if (!outcome.ok) {
        message.error(
          outcome.message ??
            t('knowledgeCompilation.ontologyInspect.reviewFailed', {
              count: targets.length,
              defaultValue: '{{count}} assertion(s) could not be reviewed',
            }),
        );
        return;
      }

      const failed = outcome.result?.failed ?? [];
      const failedIds = new Set(failed.map((entry) => entry.id));
      // The response names rows by id alone, so a failure is attributed to every
      // sent row carrying that id — two datasets may hold the same row name.
      setSelectedKeys((current) =>
        current.filter((key) =>
          targets.some(
            (item) => rowKey(item) === key && failedIds.has(item.id),
          ),
        ),
      );
      setFailures((current) => {
        const next = { ...current };
        targets.forEach((item) => {
          delete next[rowKey(item)];
        });
        failed.forEach((entry) => {
          const row = targets.find((item) => item.id === entry.id);
          if (row) {
            next[rowKey(row)] = entry.reason;
          }
        });
        return next;
      });

      if (failed.length === 0) {
        message.success(
          t('knowledgeCompilation.ontologyInspect.reviewSucceeded', {
            count: outcome.result?.updated ?? targets.length,
            defaultValue: 'Reviewed {{count}} assertion(s)',
          }),
        );
        return;
      }
      message.error(
        t('knowledgeCompilation.ontologyInspect.reviewFailed', {
          count: failed.length,
          defaultValue: '{{count}} assertion(s) could not be reviewed',
        }),
      );
    },
    [templateId, submitReview, note, t],
  );

  const handleStatusChange = useCallback(
    (value: string) => setStatus(value as AssertionReviewStatus),
    [],
  );
  const handleClassChange = useCallback(
    (value: string) => setClassType(value),
    [],
  );
  const handlePropertyChange = useCallback(
    (value: string) => setProperty(value),
    [],
  );
  const handleNoteChange = useCallback(
    (event: ChangeEvent<HTMLTextAreaElement>) => setNote(event.target.value),
    [],
  );

  // Row callbacks are built here so the JSX passes a named handler instead of an
  // inline arrow; the builders themselves are stable across renders.
  const handleSelectRow = useCallback(
    (key: string) => () => setActiveKey(key),
    [],
  );
  const handleToggleRow = useCallback(
    (key: string) => () =>
      setSelectedKeys((current) =>
        current.includes(key)
          ? current.filter((item) => item !== key)
          : [...current, key],
      ),
    [],
  );
  const handleRowAction = useCallback(
    (item: IAssertionReviewItem, action: AssertionReviewAction) => () => {
      void submit(action, [item]);
    },
    [submit],
  );

  const allSelected = items.length > 0 && selectedItems.length === items.length;
  const handleSelectAll = useCallback(
    (checked: boolean | 'indeterminate') => {
      setSelectedKeys(checked === true ? items.map(rowKey) : []);
    },
    [items],
  );

  const handleBulkAccept = useCallback(() => {
    void submit('accept', selectedItems);
  }, [submit, selectedItems]);
  const handleBulkReject = useCallback(() => {
    void submit('reject', selectedItems);
  }, [submit, selectedItems]);
  const handleBulkReset = useCallback(() => {
    void submit('reset', selectedItems);
  }, [submit, selectedItems]);

  const bulkItems = useMemo<BulkOperateItemType[]>(
    () => [
      {
        id: 'accept',
        label: t('knowledgeCompilation.ontologyInspect.reviewAccept', {
          defaultValue: 'Accept',
        }),
        icon: <Check className="size-4" />,
        onClick: handleBulkAccept,
      },
      {
        id: 'reject',
        label: t('knowledgeCompilation.ontologyInspect.reviewReject', {
          defaultValue: 'Reject',
        }),
        icon: <X className="size-4" />,
        onClick: handleBulkReject,
      },
      {
        id: 'reset',
        label: t('knowledgeCompilation.ontologyInspect.reviewReset', {
          defaultValue: 'Reset',
        }),
        icon: <RotateCcw className="size-4" />,
        onClick: handleBulkReset,
      },
    ],
    [t, handleBulkAccept, handleBulkReject, handleBulkReset],
  );

  const renderRow = (item: IAssertionReviewItem) => {
    const key = rowKey(item);
    const isActive = !!activeItem && rowKey(activeItem) === key;
    const evidenceCount =
      item.evidence?.length ?? item.source_chunk_ids?.length ?? 0;
    const failure = failures[key];
    return (
      <article
        key={key}
        className={cn(
          'rounded border p-2 flex flex-col gap-1',
          isActive ? 'border-text-accent' : 'border-border-button',
        )}
      >
        <div className="flex items-start gap-2">
          <Checkbox
            className="mt-0.5 shrink-0"
            checked={selectedKeys.includes(key)}
            onCheckedChange={handleToggleRow(key)}
          />
          <button
            type="button"
            className="min-w-0 flex-1 text-start"
            onClick={handleSelectRow(key)}
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">
                {item.kind === 'entity'
                  ? (item.name ?? item.id)
                  : `${item.from ?? ''} → ${item.to ?? ''}`}
              </span>
              <span className="text-text-secondary">
                {item.kind === 'entity' ? item.type : item.property}
              </span>
              <Badge variant="secondary">
                {item.scope === 'dataset'
                  ? t(
                      'knowledgeCompilation.ontologyInspect.reviewScopeDataset',
                      {
                        defaultValue: 'Dataset',
                      },
                    )
                  : t('knowledgeCompilation.ontologyInspect.reviewScopeDoc', {
                      defaultValue: 'Document',
                    })}
              </Badge>
              <span className="text-text-secondary">
                {item.dataset_name ?? item.dataset_id}
              </span>
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyInspect.evidenceQuotes', {
                  count: evidenceCount,
                  defaultValue: 'Source quotes ({{count}})',
                })}
              </span>
              {item.review_status === 'accepted' && (
                <Badge variant="success">
                  {t('knowledgeCompilation.ontologyInspect.reviewAccepted', {
                    defaultValue: 'Accepted',
                  })}
                </Badge>
              )}
              {item.review_status === 'rejected' && (
                <Badge variant="destructive">
                  {t('knowledgeCompilation.ontologyInspect.reviewRejected', {
                    defaultValue: 'Rejected',
                  })}
                </Badge>
              )}
              {item.conflict && (
                <Badge
                  variant="secondary"
                  className="text-state-warning border-state-warning"
                >
                  {t('knowledgeCompilation.ontologyInspect.conflict', {
                    defaultValue: 'Conflict',
                  })}
                </Badge>
              )}
            </div>

            {(item.description || item.review_note) && (
              <p className="text-text-secondary">
                {item.review_note ?? item.description}
              </p>
            )}

            {failure && <p className="text-state-error">{failure}</p>}
          </button>

          <div className="flex shrink-0 items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              disabled={submitting}
              onClick={handleRowAction(item, 'accept')}
            >
              <Check className="size-4" />
              {t('knowledgeCompilation.ontologyInspect.reviewAccept', {
                defaultValue: 'Accept',
              })}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={submitting}
              onClick={handleRowAction(item, 'reject')}
            >
              <X className="size-4" />
              {t('knowledgeCompilation.ontologyInspect.reviewReject', {
                defaultValue: 'Reject',
              })}
            </Button>
          </div>
        </div>
      </article>
    );
  };

  const activeQuotes = activeItem?.evidence ?? [];
  const activeHasSource =
    activeQuotes.length > 0 || (activeItem?.source_chunk_ids?.length ?? 0) > 0;

  return (
    <Sheet open={sheetOpen} onOpenChange={handleOpenChange} modal={false}>
      {trigger && <SheetTrigger asChild>{trigger}</SheetTrigger>}

      <SheetContent
        className="w-[80%] max-w-[1200px] flex flex-col"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>
            {t('knowledgeCompilation.ontologyInspect.reviewTitle', {
              defaultValue: 'Assertion review',
            })}
          </SheetTitle>
        </SheetHeader>

        <div className="mt-2 flex flex-col gap-3 text-xs">
          <p className="text-text-secondary">
            {t('knowledgeCompilation.ontologyInspect.reviewIntro', {
              defaultValue:
                'What this model extracted, with the text it was read from. A row is only as good as the sentence behind it: accept what the source supports, reject what it does not, and reset a verdict that was made too quickly.',
            })}
          </p>

          <div className="flex flex-wrap items-center gap-2">
            <SelectWithSearch
              value={status}
              options={statusOptions}
              onChange={handleStatusChange}
            />
            <SelectWithSearch
              value={classType}
              options={[{ label: '—', value: '' }, ...classOptions]}
              onChange={handleClassChange}
              allowClear
            />
            <SelectWithSearch
              value={property}
              options={[{ label: '—', value: '' }, ...propertyOptions]}
              onChange={handlePropertyChange}
              allowClear
            />

            {queue && (
              <span className="ml-auto text-text-secondary">
                {t('knowledgeCompilation.ontologyInspect.reviewProgress', {
                  unreviewed: queue.counts.unreviewed,
                  accepted: queue.counts.accepted,
                  rejected: queue.counts.rejected,
                  conflicts: queue.conflicts,
                  defaultValue:
                    '{{unreviewed}} unreviewed · {{conflicts}} conflict(s) on this page',
                })}
              </span>
            )}
          </div>

          {queue?.truncated && (
            <div className="rounded border border-border-button p-2 text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.truncated', {
                defaultValue:
                  'Showing a capped window, not everything in scope. Narrow the datasets or the class to see the rest.',
              })}
              {(queue.skipped_dataset_ids ?? []).length > 0 && (
                <div className="mt-1 break-all">
                  {t('knowledgeCompilation.ontologyInspect.skipped', {
                    ids: (queue.skipped_dataset_ids ?? []).join(', '),
                    defaultValue: 'Not read: {{ids}}',
                  })}
                </div>
              )}
            </div>
          )}
        </div>

        <ResizablePanelGroup
          direction="horizontal"
          className="mt-3 flex-1 min-h-0"
        >
          <ResizablePanel defaultSize={45} minSize={25} className="min-h-0">
            <div className="h-full overflow-auto rounded border border-border-button p-2 flex flex-col gap-2 text-xs">
              {activeItem ? (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {activeItem.kind === 'entity'
                        ? (activeItem.name ?? activeItem.id)
                        : `${activeItem.from ?? ''} → ${activeItem.to ?? ''}`}
                    </span>
                    <span className="text-text-secondary">
                      {activeItem.kind === 'entity'
                        ? activeItem.type
                        : activeItem.property}
                    </span>
                    <span className="text-text-secondary">
                      {activeItem.dataset_name ?? activeItem.dataset_id}
                    </span>
                  </div>

                  {activeHasSource ? (
                    <OntologyEvidenceView
                      open={sheetOpen}
                      datasetId={activeItem.dataset_id}
                      documentIds={activeItem.document_ids}
                      chunkIds={activeItem.source_chunk_ids}
                      evidence={activeItem.evidence}
                    />
                  ) : (
                    // Without text there is nothing to judge against, which is a
                    // different situation from "the text is gone": the row was
                    // compiled before evidence was kept at all.
                    <span className="text-text-secondary">
                      {t(
                        'knowledgeCompilation.ontologyInspect.reviewNoEvidence',
                        {
                          defaultValue:
                            'No source text was recorded for this assertion, so it cannot be judged from here.',
                        },
                      )}
                    </span>
                  )}
                </>
              ) : (
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyInspect.reviewQueueEmpty', {
                    defaultValue:
                      'Nothing to review here. Pick another status, or clear the filters.',
                  })}
                </span>
              )}
            </div>
          </ResizablePanel>

          <ResizableHandle withHandle className="mx-2" />

          <ResizablePanel defaultSize={55} minSize={25} className="min-h-0">
            <div className="h-full flex flex-col gap-2 text-xs">
              <div className="flex items-center gap-2">
                <Checkbox
                  checked={
                    allSelected
                      ? true
                      : selectedItems.length > 0
                        ? 'indeterminate'
                        : false
                  }
                  onCheckedChange={handleSelectAll}
                />
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyInspect.reviewSelectAll', {
                    defaultValue: 'Select all on this page',
                  })}
                </span>
              </div>

              <div className="flex-1 min-h-0 overflow-auto flex flex-col gap-2">
                {loading && (
                  <span className="text-text-secondary">
                    {t('knowledgeCompilation.ontologyInspect.loading', {
                      defaultValue: 'Reading compiled rows…',
                    })}
                  </span>
                )}

                {!loading && items.length === 0 && (
                  <span className="text-text-secondary">
                    {t(
                      'knowledgeCompilation.ontologyInspect.reviewQueueEmpty',
                      {
                        defaultValue:
                          'Nothing to review here. Pick another status, or clear the filters.',
                      },
                    )}
                  </span>
                )}

                {groups.map((group) => {
                  const conflicted = group.items.some((item) => item.conflict);
                  const targets = uniqueValues(
                    group.items.flatMap((item) => item.conflict_targets ?? []),
                  );
                  const types = uniqueValues(
                    group.items.flatMap((item) => item.conflict_types ?? []),
                  );
                  return (
                    <div
                      key={group.key}
                      className={cn(
                        'flex flex-col gap-2',
                        group.items.length > 1 &&
                          'rounded border border-border-button p-2',
                      )}
                    >
                      {(group.items.length > 1 || conflicted) && (
                        <div className="flex flex-wrap items-center gap-2 text-text-secondary">
                          {group.heading && <span>{group.heading}</span>}
                          {conflicted && (
                            <Badge
                              variant="secondary"
                              className="text-state-warning border-state-warning"
                            >
                              {t(
                                'knowledgeCompilation.ontologyInspect.conflict',
                                { defaultValue: 'Conflict' },
                              )}
                            </Badge>
                          )}
                          {targets.length > 0 && (
                            <span className="break-all">
                              {t(
                                'knowledgeCompilation.ontologyInspect.conflictTargets',
                                {
                                  targets: targets.join(', '),
                                  defaultValue: 'Disagrees with: {{targets}}',
                                },
                              )}
                            </span>
                          )}
                          {types.length > 0 && (
                            <span className="break-all">
                              {t(
                                'knowledgeCompilation.ontologyInspect.conflictTypes',
                                {
                                  types: types.join(', '),
                                  defaultValue: 'Over: {{types}}',
                                },
                              )}
                            </span>
                          )}
                        </div>
                      )}

                      {group.items.map(renderRow)}
                    </div>
                  );
                })}
              </div>
            </div>
          </ResizablePanel>
        </ResizablePanelGroup>

        <div className="mt-3 flex flex-col gap-2">
          <div className="flex flex-col gap-1">
            <span className="text-xs text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.reviewNote', {
                defaultValue: 'Rejection note',
              })}
            </span>
            <Textarea
              className="min-h-9 text-xs"
              value={note}
              onChange={handleNoteChange}
              placeholder={t(
                'knowledgeCompilation.ontologyInspect.reviewNotePlaceholder',
                {
                  defaultValue:
                    'Why these are rejected — recorded on every row the batch rejects.',
                },
              )}
            />
          </div>

          <BulkOperateBar
            list={bulkItems}
            count={selectedItems.length}
            unit={t('knowledgeCompilation.ontologyInspect.assertions', {
              defaultValue: 'Assertions',
            })}
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}

export default OntologyReviewSheet;
