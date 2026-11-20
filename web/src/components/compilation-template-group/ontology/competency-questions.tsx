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

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  IOntologyCompetencyCoverage,
  IOntologyCompetencyQuestionDraft,
  IOntologyInsight,
} from '@/interfaces/database/ontology-insight';

import {
  CompetencyElementGrammar,
  nextCompetencyId,
  requiredElementCount,
} from './competency';

type Props = {
  questions: IOntologyCompetencyQuestionDraft[];
  insight: IOntologyInsight | null;
  loading: boolean;
  onEdit: (question: IOntologyCompetencyQuestionDraft) => void;
  onDelete: (type: string) => void;
  onCreate: () => void;
  disabled?: boolean;
};

/**
 * The four states a question can be in, in the order the list sorts them.
 *
 * `unmeasured` is not a failure and is never rendered as one: a model that has
 * not been compiled against a dataset yet has no coverage to report, and showing
 * it as "0%" would read as "it cannot answer anything", which is a claim about
 * the corpus that was never made.
 */
type QuestionState = 'blocked' | 'partial' | 'answerable' | 'unmeasured';

const stateOf = (
  coverage?: IOntologyCompetencyCoverage,
): QuestionState => {
  if (!coverage) {
    return 'unmeasured';
  }
  if (coverage.answerable) {
    return 'answerable';
  }
  if (coverage.satisfied === 0) {
    return 'blocked';
  }
  return 'partial';
};

const stateRank: Record<QuestionState, number> = {
  blocked: 0,
  partial: 1,
  unmeasured: 2,
  answerable: 3,
};

const StateBadge = ({ state }: { state: QuestionState }) => {
  const { t } = useTranslation();
  const styles: Record<QuestionState, string> = {
    blocked: 'text-state-error border-state-error/40',
    partial: 'text-state-warning border-state-warning/40',
    answerable: 'text-state-success border-state-success/40',
    unmeasured: 'text-text-secondary border-border-button',
  };
  const labels: Record<QuestionState, string> = {
    blocked: t('knowledgeCompilation.ontologyCqBlocked', {
      defaultValue: 'Cannot answer',
    }),
    partial: t('knowledgeCompilation.ontologyCqPartial', {
      defaultValue: 'Partial',
    }),
    answerable: t('knowledgeCompilation.ontologyCqAnswerable', {
      defaultValue: 'Answerable',
    }),
    unmeasured: t('knowledgeCompilation.ontologyCqUnmeasured', {
      defaultValue: 'Not measured',
    }),
  };
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded border px-1.5 py-0.5 text-[11px] leading-4 ${styles[state]}`}
    >
      {labels[state]}
    </span>
  );
};

/**
 * The competency question ledger: what the model promises to answer, next to
 * what the corpus lets it answer.
 *
 * The two columns are deliberately different artefacts — the left advances one
 * question at a time, the right explains why a required element is missing. That
 * explanation is the whole point: "Q3 2/3" is a score, and a score does not tell
 * an author whether to declare a class or to compile more documents.
 */
export const CompetencyQuestionsPanel = ({
  questions,
  insight,
  loading,
  onEdit,
  onDelete,
  onCreate,
  disabled,
}: Props) => {
  const { t } = useTranslation();
  const [activeId, setActiveId] = useState<string>('');

  const coverageById = useMemo(() => {
    const map = new Map<string, IOntologyCompetencyCoverage>();
    insight?.payload?.questions?.forEach((coverage) => {
      map.set(coverage.id, coverage);
    });
    return map;
  }, [insight]);

  const rows = useMemo(
    () =>
      questions
        .map((question) => ({
          question,
          coverage: coverageById.get(question.type),
          state: stateOf(coverageById.get(question.type)),
        }))
        .sort((a, b) => {
          const rank = stateRank[a.state] - stateRank[b.state];
          return rank !== 0 ? rank : a.question.type.localeCompare(b.question.type);
        }),
    [questions, coverageById],
  );

  useEffect(() => {
    if (rows.length === 0) {
      setActiveId('');
      return;
    }
    if (!rows.some((row) => row.question.type === activeId)) {
      setActiveId(rows[0].question.type);
    }
  }, [rows, activeId]);

  const active = rows.find((row) => row.question.type === activeId);

  if (loading && rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-text-secondary">
        {t('common.loading', { defaultValue: 'Loading...' })}
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <p className="text-sm text-text-secondary">
          {t('knowledgeCompilation.ontologyCqEmpty', {
            defaultValue:
              'This model has no competency questions yet. A question is what makes the model testable: it names the elements an answer needs.',
          })}
        </p>
        <Button size="sm" onClick={onCreate} disabled={disabled}>
          {t('knowledgeCompilation.ontologyCqCreate', {
            defaultValue: 'Add question',
          })}
        </Button>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0">
      <aside className="w-[320px] shrink-0 overflow-auto border-r border-border-button">
        {rows.map((row) => {
          const isActive = row.question.type === activeId;
          return (
            <button
              key={row.question.type}
              type="button"
              onClick={() => setActiveId(row.question.type)}
              className={`flex w-full flex-col gap-1 border-b border-border-button px-4 py-3 text-left transition-colors ${
                isActive ? 'bg-bg-card' : 'hover:bg-bg-card/60'
              }`}
            >
              <div className="flex items-center gap-2">
                <span className="font-mono text-[11px] text-text-secondary">
                  {row.question.type}
                </span>
                <span className="ml-auto">
                  <StateBadge state={row.state} />
                </span>
              </div>
              <span className="line-clamp-2 text-[13px] text-text-primary">
                {row.question.question || '—'}
              </span>
              <span className="text-[11px] text-text-secondary">
                {row.coverage
                  ? t('knowledgeCompilation.ontologyCqProgress', {
                      defaultValue: '{{satisfied}}/{{required}} elements',
                      satisfied: row.coverage.satisfied,
                      required: row.coverage.required,
                    })
                  : t('knowledgeCompilation.ontologyCqDeclaredElements', {
                      defaultValue: '{{count}} required elements',
                      count: requiredElementCount(row.question.elements),
                    })}
              </span>
            </button>
          );
        })}
      </aside>

      <section className="min-w-0 flex-1 overflow-auto px-6 py-5">
        {active && (
          <>
            <div className="flex items-start gap-3">
              <div className="min-w-0">
                <div className="font-mono text-xs text-text-secondary">
                  {active.question.type}
                </div>
                <h3 className="mt-1 text-base text-text-primary">
                  {active.question.question || '—'}
                </h3>
              </div>
              <div className="ml-auto flex shrink-0 items-center gap-2">
                <StateBadge state={active.state} />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onEdit(active.question)}
                  disabled={disabled}
                >
                  {t('knowledgeCompilation.ontologyCqEdit', {
                    defaultValue: 'Edit',
                  })}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onDelete(active.question.type)}
                  disabled={disabled}
                >
                  {t('knowledgeCompilation.ontologyCqDelete', {
                    defaultValue: 'Delete',
                  })}
                </Button>
              </div>
            </div>

            <h4 className="mt-6 text-xs font-medium text-text-secondary">
              {t('knowledgeCompilation.ontologyCqElements', {
                defaultValue: 'Required elements',
              })}
            </h4>
            <ul className="mt-2 divide-y divide-border-button rounded border border-border-button">
              {(active.coverage?.elements ?? []).map((element) => (
                <li
                  key={`${element.kind}:${element.name}`}
                  className="flex items-center gap-3 px-3 py-2"
                >
                  <span
                    className={
                      element.satisfied
                        ? 'text-state-success'
                        : element.required
                          ? 'text-state-error'
                          : 'text-state-warning'
                    }
                  >
                    {element.satisfied ? '✓' : element.required ? '✗' : '○'}
                  </span>
                  <span className="font-mono text-xs text-text-secondary">
                    {element.kind}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-[13px] text-text-primary">
                    {element.name}
                  </span>
                  <span className="text-[11px] text-text-secondary">
                    {element.reason
                      ? t(
                          `knowledgeCompilation.ontologyCqReason_${element.reason}`,
                          {
                            defaultValue:
                              element.reason === 'undeclared'
                                ? 'Not declared'
                                : 'Declared, no rows',
                          },
                        )
                      : t('knowledgeCompilation.ontologyCqObserved', {
                          defaultValue: '{{count}} rows',
                          count: element.observed,
                        })}
                  </span>
                </li>
              ))}
              {!active.coverage && (
                <li className="px-3 py-2 text-xs text-text-secondary">
                  {t('knowledgeCompilation.ontologyCqNotMeasuredHint', {
                    defaultValue:
                      'Run a check to see which of these elements the corpus actually fills.',
                  })}
                </li>
              )}
            </ul>

            <p className="mt-4 text-[11px] leading-5 text-text-secondary">
              {t('knowledgeCompilation.ontologyCqGrammarHint', {
                defaultValue:
                  'Elements are written as {{grammar}}. A leading ? marks an optional element and does not lower coverage.',
                grammar: CompetencyElementGrammar,
              })}
            </p>
          </>
        )}
      </section>
    </div>
  );
};

/**
 * The create/edit dialog.
 *
 * It keeps the id editable but suggests one, because the id is the diff key: two
 * questions sharing an id would make a version diff report a rewrite instead of
 * an insertion, and the publish gate would judge the wrong question.
 */
export const CompetencyQuestionDialog = ({
  open,
  onOpenChange,
  initial,
  existing,
  onSubmit,
  saving,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial?: IOntologyCompetencyQuestionDraft | null;
  existing: IOntologyCompetencyQuestionDraft[];
  onSubmit: (question: IOntologyCompetencyQuestionDraft) => void;
  saving: boolean;
}) => {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<IOntologyCompetencyQuestionDraft>({
    type: '',
    question: '',
    elements: '',
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    setDraft(
      initial ?? {
        type: nextCompetencyId(existing),
        question: '',
        elements: '',
      },
    );
  }, [open, initial, existing]);

  const duplicate = existing.some(
    (question) =>
      question.type === draft.type.trim() && question.type !== initial?.type,
  );
  const invalid = !draft.type.trim() || !draft.question.trim() || duplicate;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {initial
              ? t('knowledgeCompilation.ontologyCqEditTitle', {
                  defaultValue: 'Edit question',
                })
              : t('knowledgeCompilation.ontologyCqCreateTitle', {
                  defaultValue: 'Add competency question',
                })}
          </DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyCqId', { defaultValue: 'ID' })}
            <Input
              value={draft.type}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, type: event.target.value }))
              }
              className="font-mono"
            />
            {duplicate && (
              <span className="text-state-error">
                {t('knowledgeCompilation.ontologyCqIdDuplicate', {
                  defaultValue: 'This id is already used by another question.',
                })}
              </span>
            )}
          </label>
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyCqQuestion', {
              defaultValue: 'Question',
            })}
            <Input
              value={draft.question}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, question: event.target.value }))
              }
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyCqElements', {
              defaultValue: 'Required elements',
            })}
            <Textarea
              value={draft.elements}
              rows={3}
              placeholder={CompetencyElementGrammar}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, elements: event.target.value }))
              }
              className="font-mono text-xs"
            />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel', { defaultValue: 'Cancel' })}
          </Button>
          <Button
            disabled={invalid || saving}
            onClick={() => {
              onSubmit({
                type: draft.type.trim(),
                question: draft.question.trim(),
                elements: draft.elements.trim(),
              });
            }}
          >
            {t('common.save', { defaultValue: 'Save' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
