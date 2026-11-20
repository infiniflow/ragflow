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
  CompetencyVocabulary,
  nextCompetencyId,
  parseCompetencyElements,
  requiredElementCount,
  suggestCompetencyElements,
  toggleCompetencyElement,
  traceCompetencyElements,
} from './competency';

type Props = {
  questions: IOntologyCompetencyQuestionDraft[];
  insight: IOntologyInsight | null;
  loading: boolean;
  onEdit: (question: IOntologyCompetencyQuestionDraft) => void;
  onDelete: (type: string) => void;
  onCreate: () => void;
  /**
   * Re-derives one question's elements and saves it.
   *
   * Optional because the panel is a read surface: without it the ledger still
   * says "no required element", it just cannot fix it in place. A question
   * written before elements were derived is exactly the case this exists for.
   */
  onDerive?: (question: IOntologyCompetencyQuestionDraft) => void;
  /**
   * Opens the canvas on one element, spelled the way the clause list spells it
   * (`class:contract`). Optional for the same reason as `onDerive`: the panel is
   * usable as a read surface.
   */
  onLocate?: (focus: string) => void;
  /** Runs a measurement now; the results are the evidence under each element. */
  onRunCheck?: () => void;
  checking?: boolean;
  /** Which snapshot the evidence describes, e.g. `v7 · 2026-11-20 09:12`. */
  measuredLabel?: string;
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

const stateOf = (coverage?: IOntologyCompetencyCoverage): QuestionState => {
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
  onDerive,
  onLocate,
  onRunCheck,
  checking,
  measuredLabel,
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
          return rank !== 0
            ? rank
            : a.question.type.localeCompare(b.question.type);
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
  // Measured elements come from the corpus; declared ones are the derivation
  // stored on the question. Both are shown, because "not measured" and "nothing
  // declared" are different answers and only one of them is actionable.
  const measuredElements = active?.coverage?.elements;
  const declaredElements = active
    ? parseCompetencyElements(active.question.elements)
    : [];
  // What the locate button points at: the first element the corpus failed to
  // fill, because that is the one being looked at, otherwise the first one.
  const locateElement =
    measuredElements?.find(
      (element) => element.required && !element.satisfied,
    ) ??
    measuredElements?.[0] ??
    declaredElements[0];

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
              {measuredElements
                ? measuredElements.map((element) => (
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
                  ))
                : declaredElements.map((element) => (
                    // Not measured yet: the elements are still known, because
                    // they were derived from the question. Listing them as
                    // "not measured" is what the design asks for, and it is the
                    // difference between "nothing to check here" and "check has
                    // not run".
                    <li
                      key={`${element.kind}:${element.name}`}
                      className="flex items-center gap-3 px-3 py-2"
                    >
                      <span className="text-text-secondary">○</span>
                      <span className="font-mono text-xs text-text-secondary">
                        {element.kind}
                      </span>
                      <span className="min-w-0 flex-1 truncate text-[13px] text-text-primary">
                        {element.name}
                      </span>
                      <span className="text-[11px] text-text-secondary">
                        {t('knowledgeCompilation.ontologyCqNotMeasured', {
                          defaultValue: 'Not measured',
                        })}
                      </span>
                    </li>
                  ))}
              {declaredElements.length === 0 && (
                <li className="flex items-center gap-3 px-3 py-2 text-xs text-text-secondary">
                  <span className="min-w-0 flex-1">
                    {t('knowledgeCompilation.ontologyCqNoElements', {
                      defaultValue:
                        'This question declares no required element, so nothing measures whether the model can answer it.',
                    })}
                  </span>
                  {onDerive && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={disabled}
                      onClick={() => onDerive(active.question)}
                    >
                      {t('knowledgeCompilation.ontologyCqDerive', {
                        defaultValue: 'Derive elements',
                      })}
                    </Button>
                  )}
                </li>
              )}
              {!active.coverage && declaredElements.length > 0 && (
                <li className="px-3 py-2 text-xs text-text-secondary">
                  {t('knowledgeCompilation.ontologyCqNotMeasuredHint', {
                    defaultValue:
                      'Run a check to see which of these elements the corpus actually fills.',
                  })}
                </li>
              )}
            </ul>

            <div className="mt-3 flex flex-wrap items-center gap-2">
              {onLocate && locateElement && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={disabled}
                  onClick={() =>
                    onLocate(`${locateElement.kind}:${locateElement.name}`)
                  }
                >
                  {t('knowledgeCompilation.ontologyCqLocate', {
                    defaultValue: 'Locate {{name}} in canvas',
                    name: locateElement.name,
                  })}
                </Button>
              )}
              {onRunCheck && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={checking || disabled}
                  onClick={onRunCheck}
                >
                  {t('knowledgeCompilation.ontologyCqRunEvidence', {
                    defaultValue: 'Run and show evidence',
                  })}
                </Button>
              )}
            </div>

            {/* The evidence, named: which snapshot the ✓/✗ describe. Without it
                a measured list and a derived one look the same. */}
            {active.coverage && measuredLabel && (
              <p className="mt-2 text-[11px] text-text-secondary">
                {t('knowledgeCompilation.ontologyCqEvidence', {
                  defaultValue:
                    'Measured on {{label}} · {{satisfied}}/{{required}} elements satisfied',
                  label: measuredLabel,
                  satisfied: active.coverage.satisfied,
                  required: active.coverage.required,
                })}
              </p>
            )}

            <p className="mt-4 text-[11px] leading-5 text-text-secondary">
              {t('knowledgeCompilation.ontologyCqGrammarHint', {
                defaultValue:
                  'Elements are derived from the question text as {{grammar}}. A leading ? marks an optional element and does not lower coverage.',
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
  vocabulary,
  onSubmit,
  saving,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial?: IOntologyCompetencyQuestionDraft | null;
  existing: IOntologyCompetencyQuestionDraft[];
  vocabulary: CompetencyVocabulary;
  onSubmit: (question: IOntologyCompetencyQuestionDraft) => void;
  saving: boolean;
}) => {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<IOntologyCompetencyQuestionDraft>({
    type: '',
    question: '',
    elements: '',
  });
  // What the last trace found. Cleared on every model/question edit so the note
  // never describes a question that is no longer on screen.
  const [traceNote, setTraceNote] = useState<string>('');
  // Sticky once the author types a clause. The derivation runs on every question
  // edit until then, so the normal path is "write the question, the elements
  // appear"; a correction is never overwritten afterwards.
  const [elementsEdited, setElementsEdited] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    const next = initial ?? {
      type: nextCompetencyId(existing),
      question: '',
      elements: '',
    };
    // A list saved before the derivation existed is history, not an author's
    // current intent: open with whatever is on disk, but do not lock the field —
    // the next question edit re-derives it. A list typed in this session is the
    // only thing that must survive, and that is what `elementsEdited` tracks.
    const elements = next.elements.trim()
      ? next.elements
      : traceCompetencyElements(next.question, vocabulary).elements;
    setDraft({ ...next, elements });
    setElementsEdited(false);
    setTraceNote('');
  }, [open, initial, existing, vocabulary]);

  // One sentence describing one derivation, shared by typing and by the button
  // so the note never describes a question that is no longer on screen.
  const noteFor = useCallback(
    (result: ReturnType<typeof traceCompetencyElements>): string => {
      if (!result.classes.length && !result.properties.length) {
        return t('knowledgeCompilation.ontologyCqTraceEmpty', {
          defaultValue:
            'Nothing in this question matches a declared name, so nothing was derived. Pick the elements below instead — the list is the model itself.',
        });
      }
      const counts = t('knowledgeCompilation.ontologyCqTraceFound', {
        defaultValue:
          'Matched {{classes}} class(es), {{properties}} attribute(s), {{relations}} relation(s).',
        classes: result.classes.length,
        properties: result.properties.length,
        relations: result.relations.length,
      });
      return result.unmatched.length
        ? `${counts} ${t('knowledgeCompilation.ontologyCqTraceUnmatched', {
            defaultValue: 'Not matched: {{words}}',
            words: result.unmatched.join(', '),
          })}`
        : counts;
    },
    [t],
  );

  // The element list is derived from the question as it is typed: the question
  // already names what it is about, and the model already knows every name that
  // can exist, so typing clauses by hand re-types what both already said
  // (semantica's trace_to_elements does the same server-side).
  const onQuestionChange = (question: string) => {
    if (elementsEdited) {
      setDraft((prev) => ({ ...prev, question }));
      return;
    }
    const result = traceCompetencyElements(question, vocabulary);
    setDraft((prev) => ({ ...prev, question, elements: result.elements }));
    setTraceNote(question.trim() ? noteFor(result) : '');
  };

  // The explicit button re-runs the derivation after a manual edit of a clause,
  // which is the one case typing deliberately skips.
  const trace = () => {
    setElementsEdited(false);
    const result = traceCompetencyElements(draft.question, vocabulary);
    setDraft((prev) => ({ ...prev, elements: result.elements }));
    setTraceNote(noteFor(result));
  };

  const duplicate = existing.some(
    (question) =>
      question.type === draft.type.trim() && question.type !== initial?.type,
  );
  const noElements = requiredElementCount(draft.elements) === 0;
  // A list saved by hand can disagree with the question it belongs to. Saying so
  // is how an author learns the field is derived now, instead of concluding the
  // editor still expects them to maintain it.
  const derivedElements = traceCompetencyElements(
    draft.question,
    vocabulary,
  ).elements;
  const staleElements =
    Boolean(draft.elements.trim()) &&
    draft.elements.trim() !== derivedElements.trim();
  // The pick list, ordered with what the question names first. Without it a
  // question that shares no word with the declarations (English model, Chinese
  // question) would leave the author with an empty box and no way forward.
  const suggestions = useMemo(
    () => suggestCompetencyElements(draft.question, vocabulary),
    [draft.question, vocabulary],
  );
  const selectedClauses = new Set(
    parseCompetencyElements(draft.elements).map(
      (element) => `${element.kind}:${element.name}`,
    ),
  );
  const pickElement = (clause: string) => {
    setElementsEdited(true);
    setDraft((prev) => ({
      ...prev,
      elements: toggleCompetencyElement(prev.elements, clause),
    }));
  };
  const invalid =
    !draft.type.trim() || !draft.question.trim() || duplicate || noElements;

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
              onChange={(event) => onQuestionChange(event.target.value)}
            />
          </label>
          <div className="flex flex-col gap-1 text-xs text-text-secondary">
            <div className="flex items-center gap-2">
              <span>
                {t('knowledgeCompilation.ontologyCqElements', {
                  defaultValue: 'Required elements',
                })}
              </span>
              {/* The question already names what it is about, and the model
                  already knows every name that can exist: the list is derived
                  from the text, so the button is only needed to re-derive after
                  a manual correction. The textarea stays editable, because a
                  suggestion the author cannot correct is worse than none. */}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="ml-auto h-6 px-2 text-[11px] font-normal"
                disabled={
                  !draft.question.trim() || vocabulary.classes.length === 0
                }
                onClick={trace}
              >
                {t('knowledgeCompilation.ontologyCqTrace', {
                  defaultValue: 'Re-trace from question',
                })}
              </Button>
            </div>
            <Textarea
              value={draft.elements}
              rows={3}
              placeholder={CompetencyElementGrammar}
              onChange={(event) => {
                setElementsEdited(true);
                setDraft((prev) => ({ ...prev, elements: event.target.value }));
              }}
              className="font-mono text-xs"
            />
            {suggestions.length > 0 && (
              <div className="flex flex-col gap-1 rounded border border-border-button p-2">
                <span className="text-[11px] text-text-secondary">
                  {t('knowledgeCompilation.ontologyCqSuggestHint', {
                    defaultValue:
                      'Elements this model declares. Click to add or remove — nothing has to be typed as a clause.',
                  })}
                </span>
                <div className="flex max-h-28 flex-wrap gap-1 overflow-y-auto">
                  {suggestions.map((suggestion) => (
                    <button
                      key={suggestion.clause}
                      type="button"
                      title={
                        suggestion.aliases.join(' / ') || suggestion.clause
                      }
                      onClick={() => pickElement(suggestion.clause)}
                      className={
                        selectedClauses.has(suggestion.clause)
                          ? 'rounded border border-state-success px-1.5 py-0.5 text-[11px] text-state-success'
                          : 'rounded border border-border-button px-1.5 py-0.5 text-[11px] text-text-secondary'
                      }
                    >
                      {suggestion.label}
                    </button>
                  ))}
                </div>
              </div>
            )}
            {traceNote && (
              <span className="text-[11px] leading-4">{traceNote}</span>
            )}
            {staleElements && !elementsEdited && (
              <span className="text-state-warning">
                {t('knowledgeCompilation.ontologyCqStaleElements', {
                  defaultValue:
                    'This list was typed before elements were derived and no longer matches the question. Edit the question or press Re-trace to replace it.',
                })}
              </span>
            )}
            {!noElements && (
              <span className="text-[11px] leading-4">
                {t('knowledgeCompilation.ontologyCqDeriveHint', {
                  defaultValue:
                    'Derived from the question: the classes, attributes and relations it names are the elements an answer needs. Edit a clause only to correct the guess.',
                })}
              </span>
            )}
            {noElements && draft.question.trim() !== '' && (
              <span className="text-state-error">
                {t('knowledgeCompilation.ontologyCqElementsRequired', {
                  defaultValue:
                    'No required element: without one this question cannot be checked, and its coverage would be an empty set rather than a pass.',
                })}
              </span>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel', { defaultValue: 'Cancel' })}
          </Button>
          <Button
            disabled={invalid || saving}
            onClick={() => {
              // The button is disabled without an element, so this only guards
              // the path where the derivation effect has not flushed yet. An
              // empty list must never reach the template: the backend reads it
              // as "nothing required" and would report the question as answered.
              const elements =
                draft.elements.trim() ||
                traceCompetencyElements(draft.question, vocabulary).elements;
              onSubmit({
                type: draft.type.trim(),
                question: draft.question.trim(),
                elements,
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
