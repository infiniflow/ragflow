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

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  compilationModelStatusLabel,
  useCheckCompilationModel,
  useFetchCompilationModelImpact,
  useFetchCompilationModelVersionDiff,
  useFetchCompilationModelVersions,
  useFetchCompilationModels,
  usePublishCompilationModel,
  useRejectCompilationModel,
  useRetireCompilationModel,
  useSubmitCompilationModelForReview,
  useTrialCompilationModel,
} from '@/hooks/use-compilation-model-request';
import {
  CompilationModelPublishStatus,
  ICompilationModel,
  ICompilationModelIssue,
  ICompilationModelPublishCheck,
  ICompilationModelTrial,
  ICompilationModelTrialAssertion,
  ICompilationModelVersion,
} from '@/interfaces/database/compilation-model';
import { Routes } from '@/routes';
import { Layers } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import message from '@/components/ui/message';

interface CompilationModelsSheetProps {
  /**
   * The template being edited, when the sheet is opened from the editor.
   * Opening lands on that model's versions: the reader came to ask "is the
   * model on screen the one the corpus ran on", not to search a list.
   */
  templateId?: string;
}

/**
 * The model library: every compilation template of the tenant with the version
 * of it that is in force, who put it there, and how many datasets compile
 * against it.
 *
 * Editing a model happens in the template editor. This answers the two
 * questions the editor cannot: is the model on screen the one the corpus ran on,
 * and what will publishing the next version change.
 *
 * It is a sheet rather than a page because both callers ask it about the model
 * they already have open — the editor and the dataset's compiled output — and
 * navigating away from that context to read the answer is what the sheet
 * exists to avoid.
 */
export function CompilationModelsSheet({
  templateId,
}: CompilationModelsSheetProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { models, loading } = useFetchCompilationModels();

  const [publishTarget, setPublishTarget] = useState<ICompilationModel | null>(
    null,
  );
  const [check, setCheck] = useState<ICompilationModelPublishCheck | null>(
    null,
  );
  const [note, setNote] = useState('');
  const [historyTarget, setHistoryTarget] = useState<ICompilationModel | null>(
    null,
  );
  const [diffVersions, setDiffVersions] = useState<{
    from: number;
    to: number;
  } | null>(null);
  const [impactTarget, setImpactTarget] = useState<ICompilationModel | null>(
    null,
  );
  // A rehearsal is an action, not a report: the dialog opens on a warning that
  // says what it will cost, and only then runs.
  const [trialTarget, setTrialTarget] = useState<ICompilationModel | null>(
    null,
  );
  const [trial, setTrial] = useState<ICompilationModelTrial | null>(null);

  const { checkModel, loading: checking } = useCheckCompilationModel();
  const { publishModel, loading: publishing } = usePublishCompilationModel();
  const { submitForReview, loading: submitting } =
    useSubmitCompilationModelForReview();
  const { rejectModel, loading: rejecting } = useRejectCompilationModel();
  const { retireModel, loading: retiring } = useRetireCompilationModel();

  const { versions, loading: versionsLoading } =
    useFetchCompilationModelVersions(historyTarget?.id);
  const { diff, loading: diffLoading } = useFetchCompilationModelVersionDiff(
    historyTarget?.id,
    diffVersions?.from,
    diffVersions?.to,
  );
  const { impact, loading: impactLoading } = useFetchCompilationModelImpact(
    impactTarget?.id,
  );
  const { runTrial, loading: trialing } = useTrialCompilationModel();

  // Opening from the editor lands on the edited model once the list has
  // arrived. The ref keeps that to one automatic open per sheet opening: after
  // the reader closes the versions to look at the list, a late model response
  // must not reopen what they just dismissed.
  const autoOpenedRef = useRef(false);
  useEffect(() => {
    if (!open) {
      autoOpenedRef.current = false;
      return;
    }
    if (autoOpenedRef.current || !templateId) {
      return;
    }
    const target = models.find((model) => model.id === templateId);
    if (!target) {
      return;
    }
    autoOpenedRef.current = true;
    setHistoryTarget(target);
    setDiffVersions(null);
  }, [open, templateId, models]);

  // The rehearsal is opened rather than run: the dialog states what it will
  // cost, and the operator decides. Nothing expensive happens on a hover.
  const openTrial = useCallback((model: ICompilationModel) => {
    setTrialTarget(model);
    setTrial(null);
  }, []);

  // Closing drops the deep link: otherwise the dialog would reopen on every
  // refresh, and the reader could never get back to the list.
  const closeHistory = useCallback(() => {
    setHistoryTarget(null);
    setDiffVersions(null);
  }, []);

  const confirmTrial = useCallback(async () => {
    if (!trialTarget) return;
    const result = await runTrial(trialTarget.id);
    if (!result.ok) {
      message.error(
        result.message ?? t('knowledgeCompilation.ontologyModel.trialFailed'),
      );
      return;
    }
    setTrial(result.trial ?? null);
  }, [runTrial, t, trialTarget]);

  // Opening the publish dialog reads the findings first: the dialog has to show
  // what stands in the way before the button is pressed, not after.
  const openPublish = useCallback(
    async (model: ICompilationModel) => {
      setPublishTarget(model);
      setCheck(null);
      setNote('');
      const result = await checkModel(model.id);
      setCheck(result.check ?? null);
    },
    [checkModel],
  );

  const confirmPublish = useCallback(async () => {
    if (!publishTarget) return;
    const hasWarnings = (check?.warnings?.length ?? 0) > 0;
    const result = await publishModel({
      id: publishTarget.id,
      note: note.trim() || undefined,
      // Warnings need the explicit confirmation; the button that reaches this
      // path is the one labelled "publish anyway".
      force: hasWarnings,
    });
    if (result.ok) {
      message.success(
        t('knowledgeCompilation.ontologyModel.publishSucceeded', {
          version: result.check?.current_version ?? 0,
        }),
      );
      setPublishTarget(null);
      setCheck(null);
      return;
    }
    // A refusal carries the findings back, so the dialog stays open with the
    // reason in it rather than closing on an unexplained failure.
    if (result.check) {
      setCheck(result.check);
    }
    message.error(
      result.message ?? t('knowledgeCompilation.ontologyModel.publishFailed'),
    );
  }, [check, note, publishModel, publishTarget, t]);

  const review = useCallback(
    async (model: ICompilationModel) => {
      const result = await submitForReview(model.id);
      if (result.ok) {
        message.success(
          t('knowledgeCompilation.ontologyModel.reviewSubmitted'),
        );
      } else {
        message.error(
          result.message ??
            t('knowledgeCompilation.ontologyModel.reviewSubmitFailed'),
        );
      }
    },
    [submitForReview, t],
  );

  const reject = useCallback(
    async (model: ICompilationModel) => {
      const reason = window.prompt(
        t('knowledgeCompilation.ontologyModel.rejectPrompt'),
      );
      if (!reason || !reason.trim()) return;
      const result = await rejectModel({ id: model.id, reason });
      if (result.ok) {
        message.success(t('knowledgeCompilation.ontologyModel.rejected'));
      } else {
        message.error(
          result.message ??
            t('knowledgeCompilation.ontologyModel.rejectFailed'),
        );
      }
    },
    [rejectModel, t],
  );

  const retire = useCallback(
    async (model: ICompilationModel) => {
      const result = await retireModel(model.id);
      if (result.ok) {
        message.success(t('knowledgeCompilation.ontologyModel.retired'));
      } else {
        message.error(
          result.message ??
            t('knowledgeCompilation.ontologyModel.retireFailed'),
        );
      }
    },
    [retireModel, t],
  );

  const busy = checking || publishing || submitting || rejecting || retiring;

  return (
    <>
      <Sheet open={open} onOpenChange={setOpen} modal={false}>
        <Tooltip>
          <TooltipTrigger asChild>
            <SheetTrigger asChild>
              <Button variant="ghost" size="sm" className="whitespace-nowrap">
                <Layers className="size-4" />
                {t('knowledgeCompilation.ontologyModel.modelLibraryLink')}
              </Button>
            </SheetTrigger>
          </TooltipTrigger>
          <TooltipContent>
            {t('knowledgeCompilation.ontologyModel.description')}
          </TooltipContent>
        </Tooltip>

        <SheetContent
          className="w-1/2 max-w-[760px] flex flex-col"
          onInteractOutside={(e) => e.preventDefault()}
        >
          <SheetHeader>
            <SheetTitle>
              {t('knowledgeCompilation.ontologyModel.title')}
            </SheetTitle>
          </SheetHeader>

          <p className="mt-2 text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyModel.description')}
          </p>

          <div className="mt-4 flex-1 min-h-0 overflow-auto flex flex-col gap-3">
            {loading && models.length === 0 && (
              <span className="text-sm text-text-secondary">
                {t('knowledgeCompilation.ontologyModel.loading')}
              </span>
            )}

            {!loading && models.length === 0 && (
              <span className="text-sm text-text-secondary">
                {t('knowledgeCompilation.ontologyModel.empty')}
              </span>
            )}

            {models.map((model) => (
              <ModelRow
                key={model.id}
                model={model}
                busy={busy}
                onPublish={openPublish}
                onReview={review}
                onReject={reject}
                onRetire={retire}
                onHistory={(target) => {
                  setHistoryTarget(target);
                  setDiffVersions(null);
                }}
                onImpact={setImpactTarget}
                onTrial={openTrial}
              />
            ))}
          </div>
        </SheetContent>
      </Sheet>

      <Dialog
        open={!!trialTarget}
        onOpenChange={(open) => {
          if (!open) {
            setTrialTarget(null);
            setTrial(null);
          }
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyModel.trialTitle', {
                name: trialTarget?.name ?? '',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyModel.trialDescription', {
                current: trialTarget?.version ?? 0,
                next: (trialTarget?.version ?? 0) + 1,
              })}
            </DialogDescription>
          </DialogHeader>

          {!trial && (
            <div className="text-sm text-text-secondary">
              {t('knowledgeCompilation.ontologyModel.trialWarning')}
            </div>
          )}

          {trial && <ModelTrialBody trial={trial} />}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setTrialTarget(null);
                setTrial(null);
              }}
            >
              {t('knowledgeCompilation.ontologyModel.cancel')}
            </Button>
            <Button disabled={trialing} onClick={confirmTrial}>
              {trialing
                ? t('knowledgeCompilation.ontologyModel.trialRunning')
                : trial
                  ? t('knowledgeCompilation.ontologyModel.trialRerun')
                  : t('knowledgeCompilation.ontologyModel.trialRun')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!impactTarget}
        onOpenChange={(open) => {
          if (!open) {
            setImpactTarget(null);
          }
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyModel.impactTitle', {
                name: impactTarget?.name ?? '',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyModel.impactDescription')}
            </DialogDescription>
          </DialogHeader>

          {impactLoading && (
            <div className="text-sm text-text-secondary">
              {t('knowledgeCompilation.ontologyModel.loading')}
            </div>
          )}

          {!impactLoading && (impact?.datasets?.length ?? 0) === 0 && (
            <div className="text-sm text-text-secondary">
              {t('knowledgeCompilation.ontologyModel.impactEmpty')}
            </div>
          )}

          <ScrollArea className="max-h-[50vh]">
            <div className="flex flex-col gap-2">
              {impact?.datasets?.map((dataset) => (
                <div
                  key={dataset.id}
                  className="flex items-center justify-between gap-3 rounded-md border border-border-default px-3 py-2"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">
                      {dataset.name}
                    </div>
                    <div className="text-xs text-text-secondary">
                      {t('knowledgeCompilation.ontologyModel.impactDocs', {
                        count: dataset.doc_num,
                      })}
                    </div>
                  </div>
                  {/* A link out, not an action: the change itself is made in
                      the dataset, by whoever owns it. */}
                  <Link
                    className="shrink-0 text-sm text-text-accent"
                    to={`${Routes.Dataset}/${dataset.id}`}
                  >
                    {t('knowledgeCompilation.ontologyModel.openDataset')}
                  </Link>
                </div>
              ))}
            </div>
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!publishTarget}
        onOpenChange={(open) => {
          if (!open) {
            setPublishTarget(null);
            setCheck(null);
          }
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyModel.publishTitle', {
                name: publishTarget?.name ?? '',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyModel.publishDescription', {
                current: check?.current_version ?? 0,
                next: check?.next_version ?? 0,
              })}
            </DialogDescription>
          </DialogHeader>

          <PublishCheckBody check={check} loading={checking} />

          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium" htmlFor="model-publish-note">
              {t('knowledgeCompilation.ontologyModel.publishNote')}
            </label>
            <Textarea
              id="model-publish-note"
              value={note}
              onChange={(event) => setNote(event.target.value)}
              placeholder={t(
                'knowledgeCompilation.ontologyModel.publishNotePlaceholder',
              )}
            />
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setPublishTarget(null);
                setCheck(null);
              }}
            >
              {t('knowledgeCompilation.ontologyModel.cancel')}
            </Button>
            <Button
              disabled={
                busy ||
                checking ||
                !check ||
                // A blocker is not a judgement the caller is allowed to
                // override, so the button is not offered at all.
                !check.publishable
              }
              onClick={confirmPublish}
            >
              {(check?.warnings?.length ?? 0) > 0
                ? t('knowledgeCompilation.ontologyModel.publishAnyway')
                : t('knowledgeCompilation.ontologyModel.publish')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!historyTarget}
        onOpenChange={(open) => {
          if (!open) {
            closeHistory();
          }
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {t('knowledgeCompilation.ontologyModel.historyTitle', {
                name: historyTarget?.name ?? '',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('knowledgeCompilation.ontologyModel.historyDescription')}
            </DialogDescription>
          </DialogHeader>

          {versionsLoading && versions.length === 0 && (
            <div className="text-sm text-text-secondary">
              {t('knowledgeCompilation.ontologyModel.loading')}
            </div>
          )}
          {!versionsLoading && versions.length === 0 && (
            <div className="text-sm text-text-secondary">
              {t('knowledgeCompilation.ontologyModel.noVersions')}
            </div>
          )}

          <ScrollArea className="max-h-[40vh]">
            <div className="flex flex-col gap-2 pr-3">
              {versions.map((version) => (
                <VersionRow
                  key={version.version}
                  version={version}
                  selected={diffVersions?.to === version.version}
                  onSelect={() =>
                    setDiffVersions(
                      version.version > 1
                        ? { from: version.version - 1, to: version.version }
                        : null,
                    )
                  }
                />
              ))}
            </div>
          </ScrollArea>

          {diffVersions && (
            <>
              <Separator />
              <VersionDiffBody
                loading={diffLoading}
                changes={diff?.changes ?? []}
                from={diffVersions.from}
                to={diffVersions.to}
              />
            </>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ModelRow({
  model,
  busy,
  onPublish,
  onReview,
  onReject,
  onRetire,
  onHistory,
  onImpact,
  onTrial,
}: {
  model: ICompilationModel;
  busy: boolean;
  onPublish: (model: ICompilationModel) => void;
  onReview: (model: ICompilationModel) => void;
  onReject: (model: ICompilationModel) => void;
  onRetire: (model: ICompilationModel) => void;
  onHistory: (model: ICompilationModel) => void;
  onImpact: (model: ICompilationModel) => void;
  onTrial: (model: ICompilationModel) => void;
}) {
  const { t } = useTranslation();
  const isReview =
    model.publish_status === CompilationModelPublishStatus.Review;
  const isPublished =
    model.publish_status === CompilationModelPublishStatus.Published;

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <CardTitle className="truncate">{model.name}</CardTitle>
            <Badge variant="secondary">{model.kind}</Badge>
            <Badge variant={isPublished ? 'success' : 'secondary'}>
              {compilationModelStatusLabel(model.publish_status)}
            </Badge>
            {model.unpublished_changes && (
              <Badge
                variant="outline"
                className="border-state-warning text-state-warning"
              >
                {t('knowledgeCompilation.ontologyModel.unpublishedChanges')}
              </Badge>
            )}
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Button variant="ghost" size="sm" onClick={() => onHistory(model)}>
              {t('knowledgeCompilation.ontologyModel.history')}
            </Button>
            {/* Only a model with a published version has a baseline to
                rehearse against; before that the rehearsal has nothing to say. */}
            {model.version > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onTrial(model)}
                disabled={busy}
              >
                {t('knowledgeCompilation.ontologyModel.trial')}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onPublish(model)}
              disabled={busy}
            >
              {t('knowledgeCompilation.ontologyModel.publishOrCheck')}
            </Button>
            {!isReview && !isPublished && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onReview(model)}
                disabled={busy}
              >
                {t('knowledgeCompilation.ontologyModel.submitForReview')}
              </Button>
            )}
            {isReview && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onReject(model)}
                disabled={busy}
              >
                {t('knowledgeCompilation.ontologyModel.reject')}
              </Button>
            )}
            {isPublished && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onRetire(model)}
                disabled={busy}
              >
                {t('knowledgeCompilation.ontologyModel.retire')}
              </Button>
            )}
          </div>
        </div>
        <CardDescription>
          {model.version > 0
            ? t('knowledgeCompilation.ontologyModel.versionLine', {
                version: model.version,
                at: model.published_at ?? '',
                by: model.published_by ?? '',
              })
            : t('knowledgeCompilation.ontologyModel.notPublished')}
          {' · '}
          {/* The count is where the publisher asks "what does this reach", so
              it is the control that answers it. With nothing to reach it stays
              plain text rather than an affordance that opens an empty list. */}
          {model.dataset_count > 0 ? (
            <button
              type="button"
              className="text-text-accent underline-offset-2 hover:underline"
              onClick={() => onImpact(model)}
            >
              {t('knowledgeCompilation.ontologyModel.datasetCount', {
                count: model.dataset_count,
              })}
            </button>
          ) : (
            t('knowledgeCompilation.ontologyModel.datasetCount', {
              count: model.dataset_count,
            })
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-0">
        {model.review_note && (
          <p className="text-sm text-text-secondary">
            {t('knowledgeCompilation.ontologyModel.reviewNote')}:{' '}
            {model.review_note}
          </p>
        )}
        {/* The editor edits a template *group*: it loads whatever id is in the
            path as a group id, so linking a template id there answers 404 for a
            group that does not exist. A template with no group has no editor
            page, and then there is nothing to link to. */}
        {model.group_id && (
          <Link
            className="text-sm text-text-accent"
            to={`${Routes.CompilationTemplatesEditNext}/${model.group_id}`}
          >
            {t('knowledgeCompilation.ontologyModel.editModel')}
          </Link>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * What the rehearsal found.
 *
 * The totals come first because they are the decision; the documents below
 * them are the evidence. A publisher who reads "2 added, 0 dropped" needs to
 * see which document and which assertions, or the number is not actionable.
 */
function ModelTrialBody({ trial }: { trial: ICompilationModelTrial }) {
  const { t } = useTranslation();
  const unchangedModel = trial.added === 0 && trial.removed === 0;

  return (
    <div className="flex flex-col gap-3">
      <div className="text-sm font-medium">
        {unchangedModel
          ? t('knowledgeCompilation.ontologyModel.trialNoChange', {
              runs: trial.runs,
            })
          : t('knowledgeCompilation.ontologyModel.trialSummary', {
              added: trial.added,
              removed: trial.removed,
              runs: trial.runs,
            })}
      </div>

      {trial.notes?.length > 0 && (
        <ul className="list-disc pl-5 text-xs text-text-secondary">
          {trial.notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}

      <ScrollArea className="max-h-[45vh]">
        <div className="flex flex-col gap-3 pr-3">
          {trial.documents.map((doc) => (
            <div
              key={`${doc.dataset_id}:${doc.document_id}`}
              className="rounded-md border border-border-default p-3"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="truncate text-sm font-medium">
                  {doc.document_name}
                </div>
                <div className="shrink-0 text-xs text-text-secondary">
                  {doc.dataset_name}
                </div>
              </div>
              {doc.error ? (
                // A document the trial could not run is reported, not hidden:
                // it is part of what the publisher would be deciding on.
                <p className="mt-1 text-xs text-state-error">{doc.error}</p>
              ) : (
                <>
                  <div className="mt-1 text-xs text-text-secondary">
                    {t('knowledgeCompilation.ontologyModel.trialUnchanged', {
                      count: doc.unchanged,
                    })}
                    {' · '}
                    {t('knowledgeCompilation.ontologyModel.trialChunks', {
                      count: doc.chunks,
                    })}
                  </div>
                  <TrialAssertionList
                    label={t('knowledgeCompilation.ontologyModel.trialAdded')}
                    assertions={doc.added}
                    added
                  />
                  <TrialAssertionList
                    label={t('knowledgeCompilation.ontologyModel.trialRemoved')}
                    assertions={doc.removed}
                  />
                </>
              )}
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

function TrialAssertionList({
  label,
  assertions,
  added,
}: {
  label: string;
  assertions: ICompilationModelTrialAssertion[];
  added?: boolean;
}) {
  if (assertions.length === 0) return null;
  return (
    <div className="mt-2">
      <div className="text-xs font-medium">{label}</div>
      <ul className="mt-1 flex flex-col gap-0.5">
        {assertions.map((assertion) => (
          <li key={assertion.key} className="text-xs">
            <span className="text-text-secondary">
              {assertion.type || assertion.kind}
            </span>{' '}
            <span className={added ? 'text-state-success' : 'text-state-error'}>
              {assertion.name}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function PublishCheckBody({
  check,
  loading,
}: {
  check: ICompilationModelPublishCheck | null;
  loading: boolean;
}) {
  const { t } = useTranslation();
  if (loading || !check) {
    return (
      <div className="text-sm text-text-secondary">
        {t('knowledgeCompilation.ontologyModel.checking')}
      </div>
    );
  }
  if (check.blockers.length === 0 && check.warnings.length === 0) {
    return (
      <div className="text-sm text-text-secondary">
        {t('knowledgeCompilation.ontologyModel.noFindings')}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      {check.blockers.length > 0 && (
        <IssueGroup
          heading={t('knowledgeCompilation.ontologyModel.blockers', {
            count: check.blockers.length,
          })}
          issues={check.blockers}
          tone="destructive"
        />
      )}
      {check.warnings.length > 0 && (
        <IssueGroup
          heading={t('knowledgeCompilation.ontologyModel.warnings', {
            count: check.warnings.length,
          })}
          issues={check.warnings}
          tone="warning"
        />
      )}
    </div>
  );
}

function IssueGroup({
  heading,
  issues,
  tone,
}: {
  heading: string;
  issues: ICompilationModelIssue[];
  tone: 'destructive' | 'warning';
}) {
  return (
    <div className="flex flex-col gap-1">
      <div
        className={
          tone === 'destructive'
            ? 'text-sm font-medium text-state-error'
            : 'text-sm font-medium text-state-warning'
        }
      >
        {heading}
      </div>
      <ul className="flex flex-col gap-1">
        {issues.map((issue, index) => (
          <li
            key={`${issue.code}-${index}`}
            className="text-sm text-text-secondary"
          >
            <span className="font-mono text-xs mr-2">{issue.code}</span>
            {issue.message}
          </li>
        ))}
      </ul>
    </div>
  );
}

function VersionRow({
  version,
  selected,
  onSelect,
}: {
  version: ICompilationModelVersion;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex flex-col items-start rounded-md border px-3 py-2 text-left ${
        selected ? 'border-text-accent' : 'border-border-default'
      }`}
    >
      <span className="flex items-center gap-2">
        <span className="font-medium">v{version.version}</span>
        {version.is_current && (
          <Badge variant="success">
            {t('knowledgeCompilation.ontologyModel.current')}
          </Badge>
        )}
        <span className="text-xs text-text-secondary">
          {version.published_at} {version.published_by}
        </span>
      </span>
      {version.note && (
        <span className="text-sm text-text-secondary mt-1">{version.note}</span>
      )}
    </button>
  );
}

function VersionDiffBody({
  loading,
  changes,
  from,
  to,
}: {
  loading: boolean;
  changes: { path: string; change: string; before?: string; after?: string }[];
  from: number;
  to: number;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="text-sm text-text-secondary">
        {t('knowledgeCompilation.ontologyModel.loading')}
      </div>
    );
  }
  if (changes.length === 0) {
    return (
      <div className="text-sm text-text-secondary">
        {t('knowledgeCompilation.ontologyModel.noChanges', { from, to })}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1">
      <div className="text-sm font-medium">
        {t('knowledgeCompilation.ontologyModel.diffHeading', { from, to })}
      </div>
      <ScrollArea className="max-h-[30vh]">
        <ul className="flex flex-col gap-1 pr-3">
          {changes.map((change) => (
            <li key={change.path} className="text-sm">
              <span className="font-mono text-xs">{change.path}</span>
              <span className="ml-2 text-text-secondary">{change.change}</span>
              {change.before && (
                <span className="ml-2 text-state-error line-through">
                  {change.before}
                </span>
              )}
              {change.after && (
                <span className="ml-2 text-state-success">{change.after}</span>
              )}
            </li>
          ))}
        </ul>
      </ScrollArea>
    </div>
  );
}

export default CompilationModelsSheet;
