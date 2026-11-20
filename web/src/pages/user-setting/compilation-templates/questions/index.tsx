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

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router';

import BackButton from '@/components/back-button';
import {
  CompetencyQuestionDialog,
  CompetencyQuestionsPanel,
} from '@/components/compilation-template-group/ontology/competency-questions';
import {
  parseCompetencyQuestions,
  traceCompetencyElements,
  vocabularyOf,
  withCompetencyQuestions,
} from '@/components/compilation-template-group/ontology/competency';
import { buildGroupUpdatePayload } from '@/components/compilation-template-group/utils';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import message from '@/components/ui/message';
import {
  useFetchCompilationTemplateGroup,
  useUpdateCompilationTemplateGroup,
} from '@/hooks/use-compilation-template-group-request';
import {
  useFetchOntologyInsight,
  useFetchOntologyModelImpact,
  useRefreshOntologyInsight,
} from '@/hooks/use-ontology-model-request';
import { ICompilationTemplateConfig } from '@/interfaces/database/compilation-template';
import { IOntologyCompetencyQuestionDraft } from '@/interfaces/database/ontology-insight';
import { Routes } from '@/routes';
import { ontologyTemplateIdOf } from '@/utils/compilation-template-util';

/**
 * The capability ledger for one model.
 *
 * It is a page rather than a tab because the numbers it shows are the ones a
 * reviewer signs off on at publish time: the same coverage the publish gate
 * blocks below 60% on. Putting it behind a tab in the editor would hide a
 * blocking finding one click deep in the screen that caused it.
 */
const CompetencyQuestionsPage = () => {
  const { t } = useTranslation();
  const { id } = useParams();
  const { data: group, loading: groupLoading } =
    useFetchCompilationTemplateGroup(id);
  const { updateGroup, loading: saving } = useUpdateCompilationTemplateGroup();

  const templateId = useMemo(() => ontologyTemplateIdOf(group), [group]);
  const template = useMemo(
    () => group?.templates?.find((item) => item.id === templateId),
    [group?.templates, templateId],
  );
  const questions = useMemo(
    () =>
      parseCompetencyQuestions(
        template?.config as ICompilationTemplateConfig | undefined,
      ),
    [template?.config],
  );
  // The names a question can be traced against: the draft's declarations, which
  // are the ones the author is editing and the ones the publish gate reads.
  const vocabulary = useMemo(
    () =>
      vocabularyOf(template?.config as ICompilationTemplateConfig | undefined),
    [template?.config],
  );

  const { impact } = useFetchOntologyModelImpact(templateId);
  const datasets = impact?.datasets ?? [];
  const [datasetId, setDatasetId] = useState<string>('');
  const { insight, loading, refetch } = useFetchOntologyInsight(
    templateId,
    datasetId,
  );
  const { refreshInsight, loading: refreshing } = useRefreshOntologyInsight();

  // A model without a pick reads its largest corpus first: coverage is a fact
  // about a corpus, so "no dataset" cannot answer "can this model answer it".
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

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] =
    useState<IOntologyCompetencyQuestionDraft | null>(null);

  const persist = useCallback(
    async (next: IOntologyCompetencyQuestionDraft[]) => {
      if (!id || !group?.templates) {
        return;
      }
      // The whole template list travels because a group is saved as a unit; only
      // the model's competency section changes, and every entry keeps the name
      // the endpoint validates.
      await updateGroup(
        id,
        buildGroupUpdatePayload(group.templates, templateId, (config) =>
          withCompetencyQuestions(config, next),
        ),
      );
      setDialogOpen(false);
    },
    [group?.templates, id, templateId, updateGroup],
  );

  const handleSubmit = useCallback(
    async (question: IOntologyCompetencyQuestionDraft) => {
      const next = editing
        ? questions.map((item) =>
            item.type === editing.type ? question : item,
          )
        : [...questions, question];
      await persist(next);
      setEditing(null);
    },
    [editing, persist, questions],
  );

  const handleDelete = useCallback(
    async (type: string) => {
      await persist(questions.filter((item) => item.type !== type));
    },
    [persist, questions],
  );

  // The repair for a question saved before its elements were derived: the text
  // already names them, so the fix is one trace rather than a clause list typed
  // by hand.
  const handleDerive = useCallback(
    async (question: IOntologyCompetencyQuestionDraft) => {
      const derived = traceCompetencyElements(
        question.question,
        vocabulary,
      ).elements;
      if (!derived) {
        message.warning(
          t('knowledgeCompilation.ontologyCqTraceEmpty', {
            defaultValue:
              'No element matched. Name a class the model declares (its name, label or an alias) in the question.',
          }),
        );
        return;
      }
      await persist(
        questions.map((item) =>
          item.type === question.type ? { ...item, elements: derived } : item,
        ),
      );
      message.success(
        t('knowledgeCompilation.ontologyCqDerived', {
          defaultValue: 'Required elements derived and saved.',
        }),
      );
    },
    [persist, questions, t, vocabulary],
  );

  // Same contract as the drift panel: report the outcome, because a dataset with
  // no compiled rows re-renders an identical payload and a silent run is
  // indistinguishable from a dead button.
  const handleRefresh = useCallback(async () => {
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
    const answered =
      result.insight?.payload?.questions?.filter((item) => item.answerable)
        .length ?? 0;
    const total = result.insight?.payload?.questions?.length ?? 0;
    message.success(
      t('knowledgeCompilation.ontologyQuestionsChecked', {
        defaultValue: 'Measured: {{answered}} of {{total}} answerable.',
        answered,
        total,
      }),
    );
    await refetch();
  }, [datasetId, refetch, refreshInsight, t, templateId]);

  const navigate = useNavigate();
  // The canvas spells an element the same way this page does, so the link is the
  // clause itself and the canvas reads it back as `?focus=`.
  const handleLocate = useCallback(
    (focus: string) => {
      navigate(
        `${Routes.CompilationTemplatesEditNext}/${id}?focus=${encodeURIComponent(focus)}`,
      );
    },
    [id, navigate],
  );
  const measuredLabel = insight
    ? t('knowledgeCompilation.ontologyCqSnapshot', {
        defaultValue: 'v{{version}} · {{asOf}}',
        version: insight.version,
        asOf: formatAsOf(insight.as_of),
      })
    : undefined;

  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;
  const answered = insight?.payload?.questions?.filter(
    (q) => q.answerable,
  ).length;
  const total = insight?.payload?.questions?.length;

  return (
    <section className="flex h-full flex-col bg-bg-base">
      <header className="flex shrink-0 items-center gap-3 border-b border-border-button px-5 py-4">
        <BackButton to={backTo} />
        <div className="min-w-0">
          <h2 className="truncate font-medium text-text-secondary">
            {t('knowledgeCompilation.ontologyQuestionsPage', {
              name: group?.name ?? '',
              defaultValue: 'Competency questions — {{name}}',
            })}
          </h2>
          <p className="text-[11px] text-text-secondary">
            {insight
              ? t('knowledgeCompilation.ontologyQuestionsSummary', {
                  defaultValue:
                    '{{answered}} of {{total}} answerable · as of {{asOf}}',
                  answered: answered ?? 0,
                  total: total ?? 0,
                  asOf: formatAsOf(insight.as_of),
                })
              : t('knowledgeCompilation.ontologyQuestionsUnmeasured', {
                  defaultValue:
                    'This dataset has not been measured against the model yet.',
                })}
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <div className="w-64">
            <SelectWithSearch
              value={datasetId}
              options={datasetOptions}
              onChange={setDatasetId}
            />
          </div>
          <Button
            variant="outline"
            size="sm"
            disabled={!datasetId || refreshing}
            onClick={handleRefresh}
          >
            {t('knowledgeCompilation.ontologyCqRunCheck', {
              defaultValue: 'Run check',
            })}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setEditing(null);
              setDialogOpen(true);
            }}
          >
            {t('knowledgeCompilation.ontologyCqCreate', {
              defaultValue: 'Add question',
            })}
          </Button>
          <Button size="sm" asChild>
            <Link to={backTo}>
              {t('knowledgeCompilation.ontologyOpenCanvas', {
                defaultValue: 'Open in canvas',
              })}
            </Link>
          </Button>
        </div>
      </header>

      {insight?.stale && (
        <div className="shrink-0 border-b border-border-button bg-state-warning/10 px-5 py-2 text-[11px] text-state-warning">
          {t('knowledgeCompilation.ontologyInsightStale', {
            defaultValue:
              'The model has been published since this measurement; the numbers describe version {{version}}.',
            version: insight.version,
          })}
        </div>
      )}
      {/* Coverage is a fact about a published version. An unpublished edit is
          not a weaker measurement — it is not compiled at all, so the number
          cannot move however often the check is run. */}
      {insight?.draft_pending && (
        <div className="shrink-0 border-b border-border-button bg-state-warning/10 px-5 py-2 text-[11px] text-state-warning">
          {t('knowledgeCompilation.ontologyDriftDraftPending', {
            defaultValue:
              'The draft has unpublished edits. Only a published version is compiled, so recompiling changes nothing until you publish.',
          })}
        </div>
      )}

      <div className="min-h-0 flex-1">
        {groupLoading ? (
          <div className="flex h-full items-center justify-center text-xs text-text-secondary">
            {t('common.loading', { defaultValue: 'Loading...' })}
          </div>
        ) : (
          <CompetencyQuestionsPanel
            questions={questions}
            insight={insight}
            loading={loading}
            onCreate={() => {
              setEditing(null);
              setDialogOpen(true);
            }}
            onEdit={(question) => {
              setEditing(question);
              setDialogOpen(true);
            }}
            onDelete={handleDelete}
            onDerive={handleDerive}
            onLocate={handleLocate}
            onRunCheck={handleRefresh}
            checking={refreshing}
            measuredLabel={measuredLabel}
            disabled={saving}
          />
        )}
      </div>

      <CompetencyQuestionDialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) {
            setEditing(null);
          }
        }}
        initial={editing}
        existing={questions}
        vocabulary={vocabulary}
        saving={saving}
        onSubmit={handleSubmit}
      />
    </section>
  );
};

/**
 * Renders the measurement time the way a reader checks it, not as a timestamp
 * they have to convert. The value is milliseconds since the epoch.
 */
const formatAsOf = (milliseconds: number): string => {
  if (!milliseconds) {
    return '—';
  }
  return new Date(milliseconds).toLocaleString();
};

export default CompetencyQuestionsPage;
