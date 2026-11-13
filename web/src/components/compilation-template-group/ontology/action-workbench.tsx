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

'use client';

import { ActionContractBlock } from '@/components/action-contract-block';
import {
  ActionRowStatus,
  ActionStatusBadge,
  resolveStatus,
} from '@/components/action-status-badge';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import message from '@/components/ui/message';
import { backfillActionContracts } from '@/services/compilation-model-service';
import {
  useDryRunAction,
  useExecuteAction,
  useFetchActionExecutions,
  useFetchActionBindings,
  useFetchModelActions,
} from '@/hooks/use-ontology-action-request';
import {
  ActionVerdict,
  IActionCallResult,
  IActionExecution,
  IModelAction,
} from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  ReviewSourceActions,
  ReviewSourceParam,
} from '@/utils/compilation-template-util';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ActionContractEditor } from './action-contract-editor';

/**
 * The ontology class an action is declared under.
 *
 * The server owns this name (`contract.ActionEntityType`) and the review queue
 * filters on it, so a link that says "these actions still need a verdict" has to
 * carry the same string.
 */
const ActionEntityClass = 'action';

/**
 * One action as the list shows it: every row that states it.
 *
 * An action is stated twice — once per document that mentions it, once as the
 * dataset-wide fold — and the two rows carry independent review states. Listing
 * rows instead of actions showed twelve actions as twenty-four entries, so the
 * same name appeared with "accepted" on one line and "awaiting review" on the
 * next. Grouping is the only way the list can answer "what still needs a reader".
 */
type ActionGroup = { key: string; name: string; rows: IModelAction[] };

/**
 * What a reader needs to look at first when a group's rows disagree.
 *
 * A rejected row outranks an undecided one because someone already found it
 * wrong; a decided-but-unauthorized row outranks "unbound", and so on down to
 * the only state that needs nothing from anyone.
 */
const StatusEscalation: ActionRowStatus[] = [
  'rejected',
  'unreviewed',
  'authorization_missing',
  'unbound',
  'blocked',
  'runnable',
];

const groupStatus = (group: ActionGroup): ActionRowStatus => {
  const states = group.rows.map(resolveStatus);
  return (
    StatusEscalation.find((state) => states.includes(state)) ?? 'unreviewed'
  );
};

/** How many of the group's rows carry a verdict, right or wrong. */
const groupJudged = (group: ActionGroup) =>
  group.rows.filter(
    (row) =>
      resolveStatus(row) === 'rejected' || row.review_status === 'accepted',
  ).length;

/** The row that carries the group's escalated state, so the tooltip matches. */
const groupVoice = (group: ActionGroup) => {
  const status = groupStatus(group);
  return (
    group.rows.find((row) => resolveStatus(row) === status) ?? group.rows[0]
  );
};

/** A verdict's colour. `confirm` is not a soft `allow`, so it gets its own tone. */
function verdictClass(verdict: ActionVerdict | string) {
  switch (verdict) {
    case 'allow':
      return 'text-state-success border-state-success';
    case 'confirm':
      return 'text-state-warning border-state-warning';
    case 'refuse':
      return 'text-state-error border-state-error';
    default:
      return 'text-text-secondary border-border-button';
  }
}

/**
 * Actions: what an action is allowed to do, and what happened when it was tried.
 *
 * The panel stops at the contract on purpose. Which tool performs an action is
 * runtime wiring — it belongs to the assistant that answers questions, next to
 * the MCP servers and dataset bindings it already owns — and it changes on a
 * different clock than the model: a contract comes from a document and moves
 * when the document is recompiled, while a tool name moves when an operator
 * reconfigures the runtime. Declaring the contract here and wiring it there is
 * what keeps a recompile from silently breaking a live integration.
 */
/**
 * Actions, without the sheet around them.
 *
 * The page (`/compilation-templates/:id/actions`) and the editor's sheet render
 * this same body: the two differ in where the panel sits, not in what it does,
 * and a second implementation would let them disagree about a verdict.
 */
export function ActionWorkbench({
  templateId,
  groupId,
  enabled = true,
}: {
  templateId?: string;
  /**
   * The group the model belongs to, used to build a link into the workspace
   * pages. Those pages are addressed by group id while every read here is keyed
   * by template id, so the caller that has both passes the group along.
   */
  groupId?: string;
  /** False while the sheet is closed: an unopened panel must not fetch. */
  enabled?: boolean;
}) {
  const { t } = useTranslation();
  /** The action the list has selected, keyed by name rather than row id. */
  const [activeKey, setActiveKey] = useState<string>();
  /** Which of that action's rows the detail pane is showing. */
  const [activeRowId, setActiveRowId] = useState<string>();
  // Two views of the same resource: what the actions are, and what has been
  // attempted. They share a view because a question about one always ends up
  // being a question about the other ("can it run" / "did it run").
  const [tab, setTab] = useState<'actions' | 'audit'>('actions');
  const [backfilling, setBackfilling] = useState(false);
  const { actions, loading } = useFetchModelActions(templateId, enabled);
  const { execEnabled } = useFetchActionBindings(templateId, enabled);
  const [page, setPage] = useState(1);
  const { executions, total } = useFetchActionExecutions(
    templateId,
    page,
    enabled,
  );

  const groups = useMemo<ActionGroup[]>(() => {
    const out: ActionGroup[] = [];
    const byKey = new Map<string, ActionGroup>();
    for (const item of actions ?? []) {
      // A row without a name is its own entry: grouping unnamed rows under one
      // key would merge two unrelated actions into one.
      const key = item.name || item.id;
      const found = byKey.get(key);
      if (found) {
        found.rows.push(item);
        continue;
      }
      const group: ActionGroup = { key, name: key, rows: [item] };
      byKey.set(key, group);
      out.push(group);
    }
    return out;
  }, [actions]);

  const activeGroup = useMemo(
    () => groups.find((group) => group.key === activeKey) ?? groups[0],
    [groups, activeKey],
  );

  /**
   * The row the detail pane shows.
   *
   * Undecided first, because "what still needs a reader" is the question the
   * workbench exists to answer; when every row is decided, the first one stands
   * for the action.
   */
  const active = useMemo(() => {
    if (!activeGroup) {
      return undefined;
    }
    return (
      activeGroup.rows.find((row) => row.id === activeRowId) ??
      activeGroup.rows.find((row) => resolveStatus(row) === 'unreviewed') ??
      activeGroup.rows[0]
    );
  }, [activeGroup, activeRowId]);

  /** Where one row states the action: per document, or once for the dataset. */
  const scopeLabel = useCallback(
    (scope?: string) =>
      scope === 'dataset'
        ? t('knowledgeCompilation.ontologyInspect.actionScopeDataset', {
            defaultValue: 'dataset-wide',
          })
        : t('knowledgeCompilation.ontologyInspect.actionScopeDoc', {
            defaultValue: 'per document',
          }),
    [t],
  );

  /**
   * Where "awaiting review" leads: the instances workspace with the review queue
   * already open on this class, so the badge is an entry point rather than a
   * label that reports a state the reader then has to go hunting for.
   *
   * `from` names this workspace, so closing the queue returns the reader to the
   * actions page instead of leaving them on the instances page they only passed
   * through.
   */
  const reviewHref = groupId
    ? `${Routes.CompilationTemplatesEditNext}/${groupId}/instances?review=1&class=${ActionEntityClass}&${ReviewSourceParam}=${ReviewSourceActions}`
    : undefined;

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {t('knowledgeCompilation.ontologyInspect.actionsTitle', {
            defaultValue: 'Actions',
          })}
        </span>
        {execEnabled === false && (
          <Badge variant="secondary" className="text-state-warning">
            {t('knowledgeCompilation.ontologyInspect.actionsExecOff', {
              defaultValue: 'execution is off for this deployment',
            })}
          </Badge>
        )}
      </div>

      <div className="mt-2 flex shrink-0 items-center gap-2 text-xs">
        <Button
          variant={tab === 'actions' ? 'default' : 'outline'}
          size="sm"
          onClick={() => setTab('actions')}
        >
          {t('knowledgeCompilation.ontologyInspect.actionsTabActions', {
            defaultValue: 'Actions',
          })}
        </Button>
        <Button
          variant={tab === 'audit' ? 'default' : 'outline'}
          size="sm"
          onClick={() => setTab('audit')}
        >
          {t('knowledgeCompilation.ontologyInspect.actionsTabAudit', {
            count: total ?? 0,
            defaultValue: 'Attempts ({{count}})',
          })}
        </Button>
        {/* Rows compiled before the contract columns existed read as "nothing
            extracted". The backfill stamps them so they can be reviewed; it is
            idempotent, so pressing it twice is harmless. */}
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          loading={backfilling}
          onClick={async () => {
            if (!templateId) {
              return;
            }
            setBackfilling(true);
            try {
              const { data } = await backfillActionContracts(templateId);
              message.success(
                t('knowledgeCompilation.ontologyInspect.actionBackfillDone', {
                  count: data?.data?.updated ?? 0,
                  defaultValue: 'Stamped {{count}} row(s).',
                }),
              );
            } catch (error) {
              message.error((error as Error)?.message);
            } finally {
              setBackfilling(false);
            }
          }}
        >
          {t('knowledgeCompilation.ontologyInspect.actionBackfill', {
            defaultValue: 'Backfill old rows',
          })}
        </Button>
      </div>

      {tab === 'audit' && (
        <ActionAuditTable
          executions={executions ?? []}
          total={total ?? 0}
          page={page}
          onPage={setPage}
        />
      )}

      <div
        className={cn(
          'mt-2 flex-1 min-h-0 grid grid-cols-[minmax(240px,32%)_1fr] gap-3 text-xs',
          tab === 'audit' && 'hidden',
        )}
      >
        <div className="min-h-0 overflow-auto rounded border border-border-button p-2 flex flex-col gap-2">
          {loading && !actions && (
            <span className="text-text-secondary">{t('common.loading')}</span>
          )}
          {!loading && (actions?.length ?? 0) === 0 && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.actionsEmpty', {
                defaultValue:
                  'This model compiled no actions. An action is an instance of the `action` entity.',
              })}
            </span>
          )}
          {groups.map((group) => {
            // "Awaiting review" is a state the reader can act on, so that badge
            // links to the queue; the rest stay plain labels. Both live beside
            // the row's own button rather than inside it, because a link nested
            // in a button is two controls fighting over one click.
            const status = groupStatus(group);
            // One undecided row is enough to make the queue worth opening, even
            // when the group's headline state is something else (a rejection).
            const awaitingReview = group.rows.some(
              (row) => resolveStatus(row) === 'unreviewed',
            );
            const voice = groupVoice(group);
            return (
              <div
                key={group.key}
                className={cn(
                  'rounded border p-2 flex items-start gap-2',
                  activeGroup?.key === group.key
                    ? 'border-text-accent'
                    : 'border-border-button',
                )}
              >
                <button
                  type="button"
                  onClick={() => {
                    setActiveKey(group.key);
                    setActiveRowId(undefined);
                  }}
                  className="min-w-0 flex-1 text-start flex flex-col gap-1"
                >
                  <span className="font-medium">{group.name}</span>
                  <span className="text-text-secondary">
                    {/* One action, several rows: say how many and how many of
                        them carry a verdict, so "accepted here, awaiting review
                        there" stops looking like a contradiction. */}
                    {group.rows.length > 1
                      ? t(
                          'knowledgeCompilation.ontologyInspect.actionGroupRows',
                          {
                            rows: group.rows.length,
                            judged: groupJudged(group),
                            defaultValue: '{{rows}} rows · {{judged}} judged',
                          },
                        )
                      : `${voice.contract?.declared ?? 0}/${voice.contract?.required?.length ?? 6}`}
                  </span>
                </button>
                {/* The six-state badge, not the raw verdict: what a reader needs
                    first is whether this row is even reviewable, and "allow"
                    says nothing about that. */}
                {awaitingReview && reviewHref ? (
                  <Link
                    to={reviewHref}
                    className="shrink-0 underline-offset-2 hover:underline"
                    title={t(
                      'knowledgeCompilation.ontologyInspect.actionReviewLink',
                      {
                        defaultValue:
                          'Open the review queue filtered to this class of assertion',
                      },
                    )}
                  >
                    <ActionStatusBadge action={voice} status={status} />
                  </Link>
                ) : (
                  <ActionStatusBadge
                    action={voice}
                    status={status}
                    className="shrink-0"
                  />
                )}
              </div>
            );
          })}
        </div>

        <div className="min-h-0 overflow-auto flex flex-col gap-3">
          {active ? (
            <div className="flex flex-col gap-3">
              {/* The same action is stated once per document and once for the
                  dataset, with separate contracts and separate verdicts. The
                  reader picks which statement they are reading instead of the
                  detail pane choosing one behind their back. */}
              {activeGroup && activeGroup.rows.length > 1 && (
                <div className="flex flex-wrap items-center gap-2 text-text-secondary">
                  {activeGroup.rows.map((row) => (
                    <Button
                      key={row.id}
                      size="sm"
                      variant={row.id === active.id ? 'default' : 'outline'}
                      onClick={() => setActiveRowId(row.id)}
                    >
                      {scopeLabel(row.scope)}
                    </Button>
                  ))}
                </div>
              )}
              <ActionDetail
                templateId={templateId!}
                action={active}
                execEnabled={execEnabled !== false}
                executions={(executions ?? []).filter(
                  (row) => row.action_name === active.name,
                )}
              />
            </div>
          ) : (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.actionsPickOne', {
                defaultValue: 'Select an action.',
              })}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

/** One action: its contract, a rehearsal, and its own recent attempts. */
function ActionDetail({
  templateId,
  action,
  execEnabled,
  executions,
}: {
  templateId: string;
  action: IModelAction;
  execEnabled: boolean;
  executions: {
    id: string;
    status: string;
    verdict: string;
    create_time?: number;
  }[];
}) {
  const { t } = useTranslation();
  const [dryRunResult, setDryRunResult] = useState<IActionCallResult>();
  const [execResult, setExecResult] = useState<IActionCallResult>();
  const [reason, setReason] = useState('');
  const { loading: dryRunLoading, dryRun } = useDryRunAction();
  const { loading: execLoading, execute } = useExecuteAction();

  const handleDryRun = useCallback(async () => {
    try {
      setDryRunResult(
        await dryRun({ id: templateId, actionName: action.name }),
      );
    } catch (error) {
      message.error((error as Error)?.message);
    }
  }, [dryRun, templateId, action.name]);

  const handleExecute = useCallback(async () => {
    if (!reason.trim()) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionReasonRequired', {
          defaultValue: 'State why this action is being run.',
        }),
      );
      return;
    }
    try {
      setExecResult(
        await execute({
          id: templateId,
          actionName: action.name,
          reason: reason.trim(),
        }),
      );
    } catch (error) {
      // A refusal answers with its verdict, not an error; a genuine failure is
      // worth showing verbatim too.
      message.error((error as Error)?.message);
    }
  }, [execute, reason, templateId, action.name, t]);

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-sm">{action.name}</span>
        <Badge variant="secondary" className={verdictClass(action.verdict)}>
          {action.verdict}
        </Badge>
        {action.dataset_id && (
          <span className="text-text-secondary">{action.dataset_id}</span>
        )}
      </div>

      {action.description && (
        <p className="text-text-secondary">{action.description}</p>
      )}

      {(action.verdict_reasons?.length ?? 0) > 0 && (
        <p className="text-text-secondary break-all">
          {action.verdict_reasons?.join(' · ')}
        </p>
      )}

      <div className="flex items-center gap-2">
        <span className="font-medium">
          {t('knowledgeCompilation.ontologyInspect.actionContractTitle', {
            defaultValue: 'Execution contract',
          })}
        </span>
        <ActionContractEditor
          templateId={templateId}
          action={action}
          trigger={
            <Button variant="ghost" size="sm">
              {t('common.edit')}
            </Button>
          }
        />
      </div>
      <ActionContractBlock
        fields={action.contract?.fields}
        missing={action.contract?.missing}
        declared={action.contract?.declared}
        required={action.contract?.required}
      />

      <div className="flex flex-col gap-2 rounded border border-border-button p-2">
        <span className="font-medium">
          {t('knowledgeCompilation.ontologyInspect.actionTrialTitle', {
            defaultValue: 'Rehearse and run',
          })}
        </span>
        <p className="text-text-disabled">
          {t('knowledgeCompilation.ontologyInspect.actionTrialIntro', {
            defaultValue:
              'A rehearsal calls nothing: it reports the verdict and the exact arguments the mapping produces. Running is audited either way, refusals included.',
          })}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            loading={dryRunLoading}
            onClick={handleDryRun}
          >
            {t('knowledgeCompilation.ontologyInspect.actionDryRun', {
              defaultValue: 'Rehearse',
            })}
          </Button>
          <Input
            value={reason}
            placeholder={t(
              'knowledgeCompilation.ontologyInspect.actionReason',
              { defaultValue: 'Reason for running (required)' },
            )}
            onChange={(e) => setReason(e.target.value)}
            className="h-8 max-w-[320px] text-xs"
          />
          <Button
            size="sm"
            loading={execLoading}
            disabled={!execEnabled}
            onClick={handleExecute}
          >
            {t('knowledgeCompilation.ontologyInspect.actionExecute', {
              defaultValue: 'Run',
            })}
          </Button>
        </div>
        {dryRunResult && (
          <CallResult
            title={t(
              'knowledgeCompilation.ontologyInspect.actionDryRunResult',
              {
                defaultValue: 'Rehearsal',
              },
            )}
            result={dryRunResult}
          />
        )}
        {execResult && (
          <CallResult
            title={t('knowledgeCompilation.ontologyInspect.actionExecResult', {
              defaultValue: 'Result',
            })}
            result={execResult}
          />
        )}
      </div>

      <div className="flex flex-col gap-1 rounded border border-border-button p-2">
        <span className="font-medium">
          {t('knowledgeCompilation.ontologyInspect.actionAudit', {
            defaultValue: 'Recent attempts',
          })}
        </span>
        {executions.length === 0 ? (
          <span className="text-text-disabled">
            {t('knowledgeCompilation.ontologyInspect.actionAuditEmpty', {
              defaultValue: 'Nobody has tried to run this action.',
            })}
          </span>
        ) : (
          executions.map((row) => (
            <div key={row.id} className="flex flex-wrap items-center gap-2">
              <span className="text-text-secondary">{row.status}</span>
              <span className="text-text-disabled">{row.verdict}</span>
            </div>
          ))
        )}
      </div>
    </>
  );
}

function CallResult({
  title,
  result,
}: {
  title: string;
  result: IActionCallResult;
}) {
  return (
    <div className="rounded border border-border-button p-2 flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-text-secondary">{title}</span>
        <Badge variant="secondary" className={verdictClass(result.verdict)}>
          {result.verdict}
        </Badge>
        <span className="text-text-disabled">{result.status}</span>
        {typeof result.duration_ms === 'number' && (
          <span className="text-text-disabled">{`${result.duration_ms} ms`}</span>
        )}
      </div>
      {result.arguments && (
        <pre className="text-text-secondary whitespace-pre-wrap break-all">
          {JSON.stringify(result.arguments, null, 2)}
        </pre>
      )}
      {result.error && <p className="text-state-error">{result.error}</p>}
      {result.output && (
        <pre className="text-text-primary whitespace-pre-wrap break-all">
          {result.output}
        </pre>
      )}
    </div>
  );
}

/**
 * Every attempt at every action of this model, refused ones included.
 *
 * The argument column shows *names only*: the server already stores keys without
 * values, and this table must not become the place someone widens that. What was
 * sent is an audit question; what it contained is a disclosure.
 */
export function ActionAuditTable({
  executions,
  total,
  page,
  onPage,
  /**
   * Whether the table offers its own filters. The standalone page does; the
   * workbench tab already narrows to one action, and two filters over the same
   * list would disagree on which one wins.
   */
  filterable = false,
}: {
  executions: IActionExecution[];
  total: number;
  page: number;
  onPage: (next: number) => void;
  filterable?: boolean;
}) {
  const { t } = useTranslation();
  const [actionFilter, setActionFilter] = useState('');
  const [verdictFilter, setVerdictFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const pageSize = 20;
  const pages = Math.max(1, Math.ceil(total / pageSize));

  const actionOptions = useMemo(
    () => [
      {
        label: t('knowledgeCompilation.ontologyInspect.actionAuditAllActions', {
          defaultValue: 'All actions',
        }),
        value: '',
      },
      ...Array.from(new Set(executions.map((row) => row.action_name)))
        .sort()
        .map((name) => ({ label: name, value: name })),
    ],
    [executions, t],
  );
  const verdictOptions = useMemo(
    () => [
      {
        label: t(
          'knowledgeCompilation.ontologyInspect.actionAuditAllVerdicts',
          {
            defaultValue: 'All verdicts',
          },
        ),
        value: '',
      },
      ...['allow', 'confirm', 'manual', 'refuse'].map((value) => ({
        label: value,
        value,
      })),
    ],
    [t],
  );
  const statusOptions = useMemo(
    () => [
      {
        label: t(
          'knowledgeCompilation.ontologyInspect.actionAuditAllStatuses',
          {
            defaultValue: 'All statuses',
          },
        ),
        value: '',
      },
      ...['succeeded', 'refused', 'failed', 'timeout'].map((value) => ({
        label: value,
        value,
      })),
    ],
    [t],
  );

  const rows = useMemo(
    () =>
      executions.filter(
        (row) =>
          (!actionFilter || row.action_name === actionFilter) &&
          (!verdictFilter || row.verdict === verdictFilter) &&
          (!statusFilter || row.status === statusFilter),
      ),
    [executions, actionFilter, verdictFilter, statusFilter],
  );

  return (
    <div className="mt-2 flex min-h-0 flex-1 flex-col gap-2 overflow-auto text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyInspect.actionAuditIntro', {
            defaultValue:
              'Every attempt, including the ones the gates refused. A refusal is a fact about the model, not about the runtime.',
          })}
        </span>
        <span className="ml-auto flex items-center gap-2 text-text-secondary">
          <Button
            variant="ghost"
            size="sm"
            disabled={page <= 1}
            onClick={() => onPage(page - 1)}
          >
            {t('common.prev')}
          </Button>
          <span>{`${page} / ${pages}`}</span>
          <Button
            variant="ghost"
            size="sm"
            disabled={page >= pages}
            onClick={() => onPage(page + 1)}
          >
            {t('common.next')}
          </Button>
        </span>
      </div>

      {filterable && (
        <div className="flex flex-wrap items-center gap-2">
          <SelectWithSearch
            value={actionFilter}
            options={actionOptions}
            onChange={setActionFilter}
            allowClear
          />
          <SelectWithSearch
            value={verdictFilter}
            options={verdictOptions}
            onChange={setVerdictFilter}
            allowClear
          />
          <SelectWithSearch
            value={statusFilter}
            options={statusOptions}
            onChange={setStatusFilter}
            allowClear
          />
        </div>
      )}

      {executions.length === 0 ? (
        <span className="text-text-disabled">
          {t('knowledgeCompilation.ontologyInspect.actionAuditEmptyAll', {
            defaultValue: 'No attempt has been made on this model yet.',
          })}
        </span>
      ) : rows.length === 0 ? (
        <span className="text-text-disabled">
          {t('knowledgeCompilation.ontologyInspect.actionAuditEmptyFiltered', {
            defaultValue: 'No attempt on this page matches the filters.',
          })}
        </span>
      ) : (
        rows.map((row) => (
          <details
            key={row.id}
            className="rounded border border-border-button p-2"
          >
            <summary className="flex flex-wrap items-center gap-2 cursor-pointer">
              <span className="text-text-primary">{row.action_name}</span>
              <span className="text-text-secondary">{row.mode}</span>
              <span className={verdictClass(row.verdict)}>{row.verdict}</span>
              <span className="text-text-secondary">{row.status}</span>
              <span className="text-text-disabled">{row.mcp_tool}</span>
              {row.create_time ? (
                <span className="ml-auto text-text-disabled">
                  {new Date(row.create_time).toLocaleString()}
                </span>
              ) : null}
            </summary>

            <div className="mt-1 flex flex-col gap-1">
              {/* The reason chain: which gate decided it. A refusal without this
                  is a verdict the reader cannot act on. */}
              {(row.verdict_reasons ?? []).length > 0 && (
                <span className="break-all text-state-warning">
                  {t(
                    'knowledgeCompilation.ontologyInspect.actionAuditReasons',
                    {
                      reasons: (row.verdict_reasons ?? []).join(' · '),
                      defaultValue: 'Decided by: {{reasons}}',
                    },
                  )}
                </span>
              )}
              <span className="text-text-disabled">
                {t('knowledgeCompilation.ontologyInspect.actionAuditArgs', {
                  args: Object.keys(row.request_arg ?? {}).join(', ') || '—',
                  defaultValue: 'Arguments (names only): {{args}}',
                })}
              </span>
              {row.confirmed_by && (
                <span className="text-text-secondary">
                  {t(
                    'knowledgeCompilation.ontologyInspect.actionAuditConfirmed',
                    {
                      by: row.confirmed_by,
                      defaultValue: 'Confirmed by {{by}}',
                    },
                  )}
                </span>
              )}
              {row.error && <p className="text-state-error">{row.error}</p>}
              {row.response_excerpt && (
                <pre className="whitespace-pre-wrap break-all text-text-secondary">
                  {row.response_excerpt}
                </pre>
              )}
            </div>
          </details>
        ))
      )}
    </div>
  );
}
