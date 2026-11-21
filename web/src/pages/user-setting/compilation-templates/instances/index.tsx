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

import BackButton from '@/components/back-button';
import { OntologyAboxPanel } from '@/components/ontology-abox';
import { useOntologyRows } from '@/components/ontology-abox/hooks';
import {
  withClassDescription,
  withDeclaredClassAndAttributes,
  withPropertyNotes,
  withoutDeclaredClass,
  withoutDeclaredProperty,
} from '@/components/compilation-template-group/ontology/declaration';
import { OntologyDriftPanel } from '@/components/compilation-template-group/ontology/drift-panel';
import { OntologyReviewSheet } from '@/components/compilation-template-group/ontology/review-sheet';
import { buildGroupUpdatePayload } from '@/components/compilation-template-group/utils';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import message from '@/components/ui/message';
import {
  useDismissDriftFinding,
  useFetchDriftDismissals,
  useFetchOntologyInsight,
  useFetchOntologyModelChanges,
  useFetchOntologyModelImpact,
  useRefreshOntologyInsight,
  useRestoreDriftFinding,
  useSubmitOntologyModelForReview,
} from '@/hooks/use-ontology-model-request';
import {
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import type {
  ICompilationTemplate,
  ICompilationTemplateConfig,
} from '@/interfaces/database/compilation-template';
import { CompilationTemplateKind } from '@/constants/compilation';
import { Routes } from '@/routes';
import {
  ontologyTemplateIdOf,
  ReviewSourceActions,
  ReviewSourceParam,
} from '@/utils/compilation-template-util';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, useSearchParams } from 'react-router';

/**
 * The ABox of one model, as a page.
 *
 * The rows live in a dataset, so the page opens on one and lets the reader
 * switch. It used to ask a model-scoped endpoint that fanned out over every
 * bound dataset and then cut the merged list in memory — a page of 50 cost 50
 * rows from each of up to 20 datasets, the total was those per-dataset row
 * counts added together (document rows and the dataset-level folded row both
 * counting), and there was no way to ask for the second page. Reading one
 * dataset at a time is the same read the dataset and document pages make, so
 * there is one path and the numbers on it add up.
 *
 * Editing is deliberately absent. A compiled row is an assertion about a
 * document; changing it in place would leave a claim whose source no longer
 * says what the row says. Judging rows belongs to the review queue.
 */
export default function CompilationTemplateInstancesPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  /**
   * The actions page links here with `?review=1&class=action&from=actions`: a
   * row that says "awaiting review" must lead to the place where it can be
   * reviewed, or the badge is a dead end.
   */
  const [reviewOpen, setReviewOpen] = useState(
    searchParams.get('review') === '1',
  );
  /**
   * The class the queue is filtered to.
   *
   * It is state, not a URL read: the drift panel opens the queue in place, and a
   * `setSearchParams` there would change the URL of a page that never remounts —
   * the sheet would stay shut. The initial value still comes from the URL so the
   * deep link from the actions page arrives filtered.
   */
  const [reviewFilter, setReviewFilter] = useState(
    () => searchParams.get('class') ?? undefined,
  );
  const reviewSource = searchParams.get(ReviewSourceParam);
  /**
   * Closing the queue returns the reader where they came from.
   *
   * The queue is the *only* place a verdict can be recorded, which is why the
   * actions page links into it; but this page is not where that reader works.
   * Leaving them here would strand them on a workspace they never chose, with
   * the queue already closed.
   *
   * `replace` keeps the review URL out of history: Back must not reopen a sheet
   * the reader just dismissed.
   */
  const handleReviewOpenChange = useCallback(
    (open: boolean) => {
      setReviewOpen(open);
      if (open || reviewSource !== ReviewSourceActions || !id) return;
      navigate(`${Routes.CompilationTemplatesEditNext}/${id}/actions`, {
        replace: true,
      });
    },
    [id, navigate, reviewSource],
  );
  const { data: group } = useFetchCompilationTemplateGroup(id);
  // `:id` addresses the group (the editor route's unit); every model-scoped read
  // below is keyed by the template row inside it.
  const templateId = ontologyTemplateIdOf(group);
  /**
   * The declared vocabulary, as a stable value.
   *
   * The class canvas turns its data into a force simulation; handing it a fresh
   * array on every render restarts that simulation, and the panel that opened the
   * drift drawer would re-render often enough to leave the page unresponsive.
   */
  const declaredClassOptions = useMemo(
    () => declaredClasses(group?.templates, templateId),
    [group?.templates, templateId],
  );
  const declaredClassNames = useMemo(
    () => declaredClassOptions?.map((item) => item.type),
    [declaredClassOptions],
  );
  const { impact } = useFetchOntologyModelImpact(templateId);
  const datasets = useMemo(() => impact?.datasets ?? [], [impact]);
  const [datasetId, setDatasetId] = useState('');
  /**
   * The draft against the published version. Drift numbers describe the
   * published version, so a declaration made here would otherwise look like it
   * did nothing; this is what tells the reader it landed and where it goes next.
   */
  const { pending, refetch: refetchChanges } =
    useFetchOntologyModelChanges(templateId);
  /**
   * Findings already judged, filtered out of the panel. Without this the same
   * undeclared class returns after every compile and the list stops being read.
   */
  const { dismissed, dismissals } = useFetchDriftDismissals(
    templateId,
    datasetId,
  );
  const { dismiss: dismissFinding } = useDismissDriftFinding();
  const { restore: restoreFinding } = useRestoreDriftFinding();
  const [dismissTarget, setDismissTarget] = useState('');
  const [dismissReason, setDismissReason] = useState('');
  /**
   * The two findings whose fix needs an answer before it writes: an attribute
   * needs a description / value set, and a deletion needs a confirmation —
   * everything else is one click into the draft.
   */
  const [describeTarget, setDescribeTarget] = useState<{
    /** A class the corpus never produced, or one attribute filled too rarely. */
    kind: 'class' | 'attribute';
    className: string;
    property?: string;
  } | null>(null);
  const [describeDraft, setDescribeDraft] = useState({
    description: '',
    enumValues: '',
  });
  const [removeTarget, setRemoveTarget] = useState<{
    kind: 'class' | 'property';
    name: string;
  } | null>(null);
  /**
   * Which class the last located drift row is about. Two consumers: the class
   * graph highlight, and the inline sample block (one class at a time — a sample
   * preloaded for every row would be eight queries for one answer).
   */
  const [locatedClass, setLocatedClass] = useState('');
  const { submitForReview, loading: submittingForReview } =
    useSubmitOntologyModelForReview();
  const [changesOpen, setChangesOpen] = useState(false);

  // Drift is open when the URL says so, so a finding can be linked to directly
  // from the publish check or a version diff.
  const [driftOpen, setDriftOpen] = useState(
    searchParams.get('panel') === 'drift',
  );
  /**
   * `?focus=class:Vendor` opens the ABox already filtered to that class.
   *
   * It is the landing half of every deep link the design promises (a drift row,
   * a publish blocker, a competency element): the issue says which class it is
   * about, and the reader must arrive looking at it rather than at "all
   * classes" and have to re-find it. The `class` parameter is a different thing
   * — that is the review queue's initial class — so this one is namespaced.
   */
  const focusParam = searchParams.get('focus') ?? '';
  const focusClass = focusParam.startsWith('class:')
    ? focusParam.slice('class:'.length)
    : undefined;
  const {
    insight,
    loading: insightLoading,
    refetch: refetchInsight,
  } = useFetchOntologyInsight(templateId, datasetId, driftOpen);
  const { refreshInsight, loading: refreshing } = useRefreshOntologyInsight();
  const { updateGroup, loading: savingGroup } =
    useUpdateCompilationTemplateGroup();

  /**
   * Runs the measurement and reports its outcome.
   *
   * The result is surfaced rather than left to the numbers: a dataset with no
   * compiled rows produces a payload identical to the previous one, so a
   * successful run that only re-renders is indistinguishable from a button that
   * does nothing.
   */
  const handleRefreshDrift = useCallback(async () => {
    if (!templateId || !datasetId) {
      return;
    }
    const result = await refreshInsight({ id: templateId, datasetId });
    if (!result.ok) {
      message.error(
        result.message ??
          t('knowledgeCompilation.ontologyDriftFailed', {
            defaultValue: 'Measurement failed.',
          }),
      );
      return;
    }
    const payload = result.insight?.payload;
    message.success(
      t('knowledgeCompilation.ontologyDriftDone', {
        defaultValue:
          'Measured {{entities}} entities and {{relations}} relations.',
        entities: payload?.drift?.total_entities ?? 0,
        relations: payload?.drift?.total_relations ?? 0,
      }),
    );
    await refetchInsight();
  }, [datasetId, refetchInsight, refreshInsight, t, templateId]);

  /**
   * Every drift fix is the same write: the model's config changes and the group
   * is saved as a unit.
   *
   * The measurement is deliberately NOT re-run. It describes the published
   * version, so re-measuring after a draft edit would return the same rows and
   * scan the dataset for nothing; what changed is what the draft now contains,
   * which is what the change bar and the publish check read.
   */
  const applyModelFix = useCallback(
    async (
      fix: (
        config: ICompilationTemplateConfig | undefined,
      ) => ICompilationTemplateConfig,
    ) => {
      if (!id || !group?.templates || !templateId) {
        return;
      }
      const target = group.templates.find((item) => item.id === templateId);
      const current = target?.config as ICompilationTemplateConfig | undefined;
      const next = fix(current);
      // Identity covers the helpers' no-op path; the deep compare covers a fix
      // that rebuilt the object without changing anything, and the empty case
      // covers a removal on a model that had no config at all.
      const unchanged = current
        ? next === current || JSON.stringify(next) === JSON.stringify(current)
        : Object.keys(next).length === 0;
      if (unchanged) {
        // The fix was already in place (declared twice, deleted twice). Saving
        // an identical config would still bump the draft and make the change bar
        // claim work that nobody did.
        message.info(
          t('knowledgeCompilation.ontologyDriftFixNoChange', {
            defaultValue: 'The draft already says this; nothing was written.',
          }),
        );
        return;
      }
      // The whole template list travels because the group is saved as a unit;
      // each entry keeps its own name/description, which the endpoint validates.
      await updateGroup(
        id,
        buildGroupUpdatePayload(group.templates, templateId, () => next),
      );
      // The panel keeps showing the published measurement, so the write would
      // otherwise look like it did nothing.
      message.success(
        t('knowledgeCompilation.ontologyDriftFixApplied', {
          defaultValue:
            'Written to the draft. Publish it and recompile this dataset — these rows describe the published version.',
        }),
      );
      await refetchChanges();
    },
    [group?.templates, id, refetchChanges, t, templateId, updateGroup],
  );

  // Declaring carries both halves of the row: the class, and the attributes the
  // corpus already fills. Writing the name alone would leave the attribute to be
  // declared by hand and the finding would come back on the next measurement.
  const handleDeclareClass = useCallback(
    (className: string, attributes: string[]) =>
      applyModelFix((config) =>
        withDeclaredClassAndAttributes(config, className, attributes),
      ),
    [applyModelFix],
  );

  const handleRemoveClass = useCallback(
    (className: string) =>
      applyModelFix((config) => withoutDeclaredClass(config, className)),
    [applyModelFix],
  );

  const handleRemoveProperty = useCallback(
    (propertyType: string) =>
      applyModelFix((config) => withoutDeclaredProperty(config, propertyType)),
    [applyModelFix],
  );

  // The fill-rate fix writes the two keys that reach the extraction prompt; a
  // low rate is otherwise a number the reader can only stare at.
  const confirmDescribe = useCallback(async () => {
    if (!describeTarget) return;
    const description = describeDraft.description.trim();
    const enumValues = describeDraft.enumValues.trim();
    const isAttribute = describeTarget.kind === 'attribute';
    if (isAttribute ? !description && !enumValues : !description) {
      message.error(
        t('knowledgeCompilation.ontologyDriftDescribeRequired', {
          defaultValue:
            'Write a description or a value set: nothing else on an attribute reaches the prompt.',
        }),
      );
      return;
    }
    await applyModelFix((config) =>
      isAttribute && describeTarget.property
        ? withPropertyNotes(config, describeTarget.property, {
            description,
            enumValues,
          })
        : withClassDescription(config, describeTarget.className, description),
    );
    setDescribeTarget(null);
    setDescribeDraft({ description: '', enumValues: '' });
  }, [applyModelFix, describeDraft, describeTarget, t]);

  const confirmRemove = useCallback(async () => {
    if (!removeTarget) return;
    if (removeTarget.kind === 'class') {
      await handleRemoveClass(removeTarget.name);
    } else {
      await handleRemoveProperty(removeTarget.name);
    }
    setRemoveTarget(null);
  }, [handleRemoveClass, handleRemoveProperty, removeTarget]);

  // Submitting is the second step of the drift loop: the declaration is in the
  // draft, and this hands it to a reviewer. It can be refused — blockers are not
  // a judgement call — and the reason belongs on screen.
  const handleSubmitReview = useCallback(async () => {
    if (!templateId) return;
    const result = await submitForReview(templateId);
    if (result.ok) {
      message.success(
        t('knowledgeCompilation.ontologyChangeSubmitted', {
          defaultValue: 'Submitted for review',
        }),
      );
      await refetchChanges();
      return;
    }
    message.error(
      result.message ??
        t('knowledgeCompilation.ontologyChangeSubmitFailed', {
          defaultValue: 'Could not submit for review',
        }),
    );
  }, [refetchChanges, submitForReview, t, templateId]);

  // Dismissing is a decision, not a verdict on the row: it is recorded with a
  // reason and scoped to this dataset, and it can be lifted without recompiling.
  const confirmDismiss = useCallback(async () => {
    if (!templateId || !datasetId || !dismissTarget) return;
    const reason = dismissReason.trim();
    if (!reason) {
      message.error(
        t('knowledgeCompilation.ontologyDriftDismissReasonRequired', {
          defaultValue:
            'A reason is required: without one nobody can tell whether the decision still holds.',
        }),
      );
      return;
    }
    const result = await dismissFinding({
      id: templateId,
      datasetId,
      className: dismissTarget,
      reason,
    });
    if (!result.ok) {
      message.error(
        result.message ??
          t('knowledgeCompilation.ontologyDriftDismissFailed', {
            defaultValue: 'Could not record the dismissal',
          }),
      );
      return;
    }
    setDismissTarget('');
    setDismissReason('');
  }, [datasetId, dismissFinding, dismissReason, dismissTarget, t, templateId]);

  // Lifting is what makes a dismissal a decision rather than a deletion: the
  // reason stays readable until someone deliberately undoes it, and the row
  // comes back without waiting for a recompile.
  const handleRestoreDismissal = useCallback(
    async (className: string) => {
      if (!templateId || !datasetId) return;
      const result = await restoreFinding({
        id: templateId,
        datasetId,
        className,
      });
      if (!result.ok) {
        message.error(
          result.message ??
            t('knowledgeCompilation.ontologyDriftRestoreFailed', {
              defaultValue: 'Could not lift the decision',
            }),
        );
      }
    },
    [datasetId, restoreFinding, t, templateId],
  );

  const dismissedRows = useMemo(
    () =>
      dismissals.map((row) => ({
        className: row.class_name,
        reason: row.reason,
      })),
    [dismissals],
  );

  // The rows behind "18,401" on a drift finding — the same read the ABox makes,
  // for the one class the reader located.
  const sampleRowsQuery = useOntologyRows({
    datasetId,
    templateId,
    kind: 'entity',
    class: locatedClass,
    offset: 0,
    enabled: !!locatedClass && !!datasetId,
  });
  const samplePage = sampleRowsQuery.data?.data?.data as
    | {
        entities?: { id?: string; name?: string; description?: string }[];
        total?: number;
      }
    | undefined;
  const samples = locatedClass
    ? {
        className: locatedClass,
        rows: (samplePage?.entities ?? []).map((row, index) => ({
          id: row.id ?? `${locatedClass}-${index}`,
          name: row.name ?? '',
          description: row.description,
        })),
        total: samplePage?.total ?? 0,
      }
    : null;

  // A model without a dataset picked reads its largest corpus first: "what did
  // this model produce" is the question the page opens with, and an empty
  // picker would answer "nothing".
  useEffect(() => {
    if (!datasetId && datasets.length > 0) {
      setDatasetId(datasets[0].id);
    }
  }, [datasetId, datasets]);

  const datasetOptions = useMemo(
    () =>
      datasets.map((dataset) => ({
        label: `${dataset.name} (${dataset.doc_num})`,
        value: dataset.id,
      })),
    [datasets],
  );

  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;

  return (
    <section className="flex h-full flex-col bg-bg-base">
      <header className="flex shrink-0 items-center gap-3 border-b border-border-button px-5 py-4">
        <BackButton to={backTo} />
        <h2 className="font-medium text-text-secondary">
          {t('knowledgeCompilation.ontologyInstancesPage', {
            name: group?.name ?? '',
            defaultValue: 'Instances & assertions — {{name}}',
          })}
        </h2>
        <div className="ml-auto flex items-center gap-2">
          <div className="w-72">
            <SelectWithSearch
              value={datasetId}
              options={datasetOptions}
              onChange={setDatasetId}
            />
          </div>
          {/* Comparing the rows with the declaration is the other question this
              page answers, and it is scoped to the dataset on screen — which is
              why the panel lives here rather than in the canvas editor, where no
              dataset is chosen. */}
          <Button
            variant={driftOpen ? 'default' : 'outline'}
            size="sm"
            disabled={!datasetId}
            onClick={() => setDriftOpen((open) => !open)}
          >
            {t('knowledgeCompilation.ontologyDrift', { defaultValue: 'Drift' })}
          </Button>
          {/* Judging a row and browsing rows are the same session's two halves,
              so the queue opens from here rather than from a separate page. */}
          <OntologyReviewSheet
            templateId={templateId}
            open={reviewOpen}
            onOpenChange={handleReviewOpenChange}
            initialClass={reviewFilter}
            entities={declaredClassNames}
            trigger={
              <Button variant="outline" size="sm">
                {t('knowledgeCompilation.ontologyOpenReview', {
                  defaultValue: 'Review',
                })}
              </Button>
            }
          />
        </div>
      </header>

      <div className="flex min-h-0 flex-1 gap-3 p-4">
        <div className="min-w-0 flex-1">
          {datasetId ? (
            <OntologyAboxPanel
              // Remounting on a new focus is what makes "locate" work twice in
              // a row: the panel reads its class once, so a second drift row
              // would otherwise only change the URL.
              key={focusClass ?? 'abox'}
              datasetId={datasetId}
              templateId={templateId}
              classes={declaredClassOptions}
              initialClass={focusClass}
              // A locate keeps the surrounding picture and marks the bubble,
              // instead of remounting the panel on a narrower filter.
              highlightClass={locatedClass || undefined}
            />
          ) : (
            <span className="text-xs text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesNoDataset', {
                defaultValue:
                  'No dataset compiles with this model yet, so there are no rows to read.',
              })}
            </span>
          )}
        </div>
        {driftOpen && datasetId && (
          // The panel reads the stored measurement, so opening it costs one row
          // lookup rather than a dataset scan; the refresh inside it is the
          // explicit way to spend that scan.
          <div className="w-[292px] shrink-0 overflow-hidden rounded border border-border-button">
            <OntologyDriftPanel
              insight={insight}
              loading={insightLoading}
              refreshing={refreshing}
              disabled={savingGroup}
              onRefresh={handleRefreshDrift}
              onDeclare={handleDeclareClass}
              onRemoveClass={(className) =>
                setRemoveTarget({ kind: 'class', name: className })
              }
              onRemoveProperty={(propertyType) =>
                setRemoveTarget({ kind: 'property', name: propertyType })
              }
              onDescribeProperty={(className, property) => {
                setDescribeDraft({ description: '', enumValues: '' });
                setDescribeTarget({
                  kind: 'attribute',
                  className,
                  property,
                });
              }}
              onDescribeClass={(className) => {
                setDescribeDraft({ description: '', enumValues: '' });
                setDescribeTarget({ kind: 'class', className });
              }}
              samples={samples}
              onReview={(className) => {
                setReviewFilter(className);
                setReviewOpen(true);
              }}
              pending={pending}
              onViewChanges={() => setChangesOpen(true)}
              onSubmitReview={
                submittingForReview ? undefined : handleSubmitReview
              }
              onLocate={(className) => setLocatedClass(className)}
              onDismiss={(className) => {
                setDismissReason('');
                setDismissTarget(className);
              }}
              dismissedClasses={dismissed}
              dismissals={dismissedRows}
              onRestore={handleRestoreDismissal}
            />
          </div>
        )}
      </div>

      {/* The change bar says "N changes in the draft"; this is that list. It is
          the same draft-against-published diff the publish dialog shows, read
          without reviewer rights — declaring a class is not a review act. */}
      <Dialog open={changesOpen} onOpenChange={setChangesOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyChangeTitle', {
                defaultValue: 'Changes in the draft',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyChangeDescription', {
                defaultValue:
                  'The draft against the published version. Publishing makes them the version the next compile reads; this dataset still holds the rows the previous one produced.',
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[420px] overflow-auto">
            {(pending?.changes?.length ?? 0) === 0 ? (
              <p className="text-sm text-text-secondary">
                {t('knowledgeCompilation.ontologyChangeEmpty', {
                  defaultValue: 'The draft has no unpublished changes.',
                })}
              </p>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-text-disabled">
                    <th className="pb-1 pr-3 font-normal">
                      {t('knowledgeCompilation.ontologyChangePath', {
                        defaultValue: 'Path',
                      })}
                    </th>
                    <th className="pb-1 pr-3 font-normal">
                      {t('knowledgeCompilation.ontologyChangeKind', {
                        defaultValue: 'Change',
                      })}
                    </th>
                    <th className="pb-1 font-normal">
                      {t('knowledgeCompilation.ontologyChangeSource', {
                        defaultValue: 'Authorised by',
                      })}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {(pending?.changes ?? []).map((change) => (
                    <tr
                      key={`${change.path}-${change.change}`}
                      className="border-t border-border-button"
                    >
                      <td className="py-1.5 pr-3 font-mono text-xs">
                        {change.path}
                      </td>
                      <td className="py-1.5 pr-3">{change.change}</td>
                      <td className="py-1.5 text-xs text-text-secondary">
                        {change.source
                          ? `${change.source}${
                              change.source_ref ? ` · ${change.source_ref}` : ''
                            }`
                          : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* The reason is the record. Without it the next reader cannot tell a
          decision from an oversight, and the finding comes back next compile. */}
      <Dialog
        open={!!dismissTarget}
        onOpenChange={(open) => {
          if (!open) setDismissTarget('');
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyDriftDismissTitle', {
                defaultValue: 'Set aside “{{name}}”',
                name: dismissTarget,
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyDriftDismissDescription', {
                defaultValue:
                  'It stays in the measurement and is only hidden in this dataset. Recompiling does not clear the decision; it is listed under “Set aside” so you can lift it again.',
              })}
            </DialogDescription>
          </DialogHeader>
          <textarea
            className="min-h-[88px] w-full rounded border border-border-button bg-bg-base p-2 text-sm"
            value={dismissReason}
            onChange={(event) => setDismissReason(event.target.value)}
            placeholder={t(
              'knowledgeCompilation.ontologyDriftDismissPlaceholder',
              {
                defaultValue:
                  'Why is this not a gap? e.g. the rows are invoices inside contracts, not a separate type',
              },
            )}
          />
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setDismissTarget('')}
            >
              {t('common.cancel', { defaultValue: 'Cancel' })}
            </Button>
            <Button size="sm" onClick={confirmDismiss}>
              {t('knowledgeCompilation.ontologyDriftDismissConfirm', {
                defaultValue: 'Record decision',
              })}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* A low fill rate is fixed by telling extraction what to look for; the
          two keys below are the only ones on a property that reach the prompt. */}
      <Dialog
        open={!!describeTarget}
        onOpenChange={(open) => {
          if (!open) setDescribeTarget(null);
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {describeTarget?.kind === 'class'
                ? t('knowledgeCompilation.ontologyDriftDescribeClassTitle', {
                    defaultValue: 'Describe “{{name}}”',
                    name: describeTarget.className,
                  })
                : t('knowledgeCompilation.ontologyDriftDescribeTitle', {
                    defaultValue: 'Describe {{name}}',
                    name: describeTarget
                      ? `${describeTarget.className}.${describeTarget.property}`
                      : '',
                  })}
            </DialogTitle>
            <DialogDescription>
              {describeTarget?.kind === 'class'
                ? t(
                    'knowledgeCompilation.ontologyDriftDescribeClassDescription',
                    {
                      defaultValue:
                        'A class with no instances is usually under-described: the description reaches the extraction prompt, so extraction starts looking for it. Delete the class instead if the corpus really has none.',
                    },
                  )
                : t('knowledgeCompilation.ontologyDriftDescribeDescription', {
                    defaultValue:
                      'Fill rate rises when extraction knows what the value looks like. Both fields below reach the prompt; required does not.',
                  })}
            </DialogDescription>
          </DialogHeader>
          <label className="text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyDriftDescribeText', {
              defaultValue: 'Description',
            })}
            <textarea
              className="mt-1 min-h-[72px] w-full rounded border border-border-button bg-bg-base p-2 text-sm"
              value={describeDraft.description}
              onChange={(event) =>
                setDescribeDraft((draft) => ({
                  ...draft,
                  description: event.target.value,
                }))
              }
              placeholder={t(
                'knowledgeCompilation.ontologyDriftDescribePlaceholder',
                {
                  defaultValue:
                    'e.g. the date payment is due, as written in the contract',
                },
              )}
            />
          </label>
          {/* A class has no value set; only an attribute does. */}
          {describeTarget?.kind === 'attribute' && (
            <label className="text-xs text-text-secondary">
              {t('knowledgeCompilation.ontologyDriftEnumLabel', {
                defaultValue: 'Allowed values (comma separated, optional)',
              })}
              <input
                className="mt-1 w-full rounded border border-border-button bg-bg-base p-2 text-sm"
                value={describeDraft.enumValues}
                onChange={(event) =>
                  setDescribeDraft((draft) => ({
                    ...draft,
                    enumValues: event.target.value,
                  }))
                }
                placeholder="draft, signed, active, terminated"
              />
            </label>
          )}
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setDescribeTarget(null)}
            >
              {t('common.cancel', { defaultValue: 'Cancel' })}
            </Button>
            <Button size="sm" onClick={confirmDescribe}>
              {t('knowledgeCompilation.ontologyDriftDescribeConfirm', {
                defaultValue: 'Write to draft',
              })}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Deleting a declaration is the one fix that can lose work someone else
          did, so it asks first and says what the draft will look like after. */}
      <Dialog
        open={!!removeTarget}
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null);
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              {removeTarget?.kind === 'class'
                ? t('knowledgeCompilation.ontologyDriftRemoveClassTitle', {
                    defaultValue: 'Delete class “{{name}}”?',
                    name: removeTarget?.name,
                  })
                : t('knowledgeCompilation.ontologyDriftRemovePropertyTitle', {
                    defaultValue: 'Delete relation “{{name}}”?',
                    name: removeTarget?.name,
                  })}
            </DialogTitle>
            <DialogDescription>
              {removeTarget?.kind === 'class'
                ? t('knowledgeCompilation.ontologyDriftRemoveClassBody', {
                    defaultValue:
                      'Properties that pointed at it keep their other end; they will show up as having a missing endpoint instead.',
                  })
                : t('knowledgeCompilation.ontologyDriftRemovePropertyBody', {
                    defaultValue:
                      'Only the declaration goes; assertions already in the corpus stay as they are.',
                  })}
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setRemoveTarget(null)}
            >
              {t('common.cancel', { defaultValue: 'Cancel' })}
            </Button>
            <Button size="sm" onClick={confirmRemove}>
              {t('knowledgeCompilation.ontologyDriftRemoveConfirm', {
                defaultValue: 'Delete from draft',
              })}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}

/**
 * declaredClasses reads the model's class list out of its working config.
 *
 * The filter should offer the vocabulary the model promises, not the classes
 * that happened to land on the first page — otherwise a declared class with no
 * instances yet is unreachable, and its emptiness is exactly what the reader
 * came to check.
 */
function declaredClasses(
  templates: ICompilationTemplate[] | undefined,
  templateId?: string,
) {
  const template =
    templates?.find((item) => item.id === templateId) ??
    templates?.find((item) => item.kind === CompilationTemplateKind.Ontology);
  const entity = template?.config?.entity as
    | { fields?: { type?: string; label?: string }[] }
    | undefined;
  const fields = (entity?.fields ?? []).filter((field) => !!field?.type);
  return fields.length > 0
    ? fields.map((field) => ({
        type: field.type as string,
        label: field.label,
      }))
    : undefined;
}
