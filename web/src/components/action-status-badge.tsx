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

import { IModelAction } from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

/**
 * One badge for the six states a compiled action can be in.
 *
 * The rule that makes this worth its own component: **"bound" must never imply
 * "runnable"**. An action can be bound to a tool, have a complete contract, and
 * still be unable to run — because the tool is not read-only, because the agent
 * switch is off, because the deployment switch is off. So the green state is
 * decided by the server's verdict (all four gates passed), never by the presence
 * of a binding.
 *
 * The order of the checks is the escalation order: a rejected row is rejected
 * regardless of its contract, and an unreviewed row is a proposal regardless of
 * how complete it looks.
 */
export type ActionRowStatus =
  | 'rejected'
  | 'unreviewed'
  | 'authorization_missing'
  | 'unbound'
  | 'runnable'
  | 'blocked';

const STATUS_STYLE: Record<
  ActionRowStatus,
  { className: string; label: string }
> = {
  rejected: { className: 'text-text-disabled', label: 'Rejected' },
  unreviewed: { className: 'text-text-accent', label: 'Awaiting review' },
  authorization_missing: {
    className: 'text-state-warning',
    label: 'Authorization unclear',
  },
  unbound: { className: 'text-text-secondary', label: 'Not bound' },
  runnable: { className: 'text-state-success', label: 'Runnable' },
  blocked: { className: 'text-text-secondary', label: 'Cannot run' },
};

export function ActionStatusBadge({
  action,
  status: statusOverride,
  className,
}: {
  action: IModelAction;
  /**
   * The state to render instead of this row's own.
   *
   * One action is stated twice (a document row and a dataset fold) and the list
   * shows the pair as one entry; that entry needs the pair's state, which no
   * single row can compute about itself.
   */
  status?: ActionRowStatus;
  className?: string;
}) {
  const { t } = useTranslation();
  const status = statusOverride ?? resolveStatus(action);
  const state = STATUS_STYLE[status];

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 text-xs',
        state.className,
        className,
      )}
      // The verdict's own reasons explain the exact gate that failed, which is
      // more precise than the six-state label and costs nothing to carry.
      title={action.verdict_reasons?.join(' · ') || undefined}
    >
      <span className={cn('size-1.5 rounded-full bg-current')} />
      <span className={cn(status === 'rejected' && 'line-through')}>
        {t(`knowledgeCompilation.ontologyInspect.actionStatus_${status}`, {
          defaultValue: state.label,
        })}
      </span>
      {isManual(action) && (
        <span className="ml-1 rounded border border-border-button px-1 text-text-disabled">
          {t('knowledgeCompilation.ontologyInspect.actionStatus_manual', {
            defaultValue: 'no source quote',
          })}
        </span>
      )}
    </span>
  );
}

export function resolveStatus(action: IModelAction): ActionRowStatus {
  const reviewStatus = action.review_status ?? 'unreviewed';
  if (reviewStatus === 'rejected') {
    return 'rejected';
  }
  if (reviewStatus !== 'accepted') {
    return 'unreviewed';
  }
  if (action.contract?.missing?.includes('authorization')) {
    // Ahead of the binding check: an action nobody may approve is the more
    // urgent fact, and it is the one a reviewer is looking for.
    return 'authorization_missing';
  }
  if (!action.binding) {
    return 'unbound';
  }
  return action.verdict === 'allow' ? 'runnable' : 'blocked';
}

/**
 * A row an operator filled in by hand.
 *
 * Read from the field provenance, not from the review note: a value nobody's
 * source text supports is the fact that matters, and `manual` in the contract is
 * exactly that fact. The edit reason lives in the review note and is not the
 * same thing — a reviewed row can carry a note without any field being manual.
 */
function isManual(action: IModelAction): boolean {
  return Object.values(action.contract?.fields ?? {}).some(
    (field) => field.provenance === 'manual',
  );
}
