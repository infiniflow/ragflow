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

import OperatorIcon from '@/components/operator-icon';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { ListChecks, MoveRight } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { CanvasIssue, CanvasIssueType } from '../utils/canvas-checklist';

type IssueGroup = {
  key: string;
  nodeName: string;
  operatorLabel: string;
  items: CanvasIssue[];
};

function groupIssuesByNode(issues: CanvasIssue[]): IssueGroup[] {
  const groups = new Map<string, IssueGroup>();
  for (const issue of issues) {
    // Composite key: multiple embedded tools of one agent share the Tool
    // canvas node (nodeId) but must each get their own group; the agent-form
    // fallback (no toolId) distinguishes them by display name.
    const key = `${issue.nodeId}::${issue.toolId ?? issue.nodeName}`;
    const group = groups.get(key);
    if (group) {
      group.items.push(issue);
    } else {
      groups.set(key, {
        key,
        nodeName: issue.nodeName,
        operatorLabel: issue.operatorLabel,
        items: [issue],
      });
    }
  }
  return [...groups.values()];
}

function IssueCountBadge({ count }: { count: number }) {
  if (count <= 0) {
    return null;
  }
  return (
    <span className="shrink-0 rounded-full bg-state-warning px-1.5 py-0.5 text-[10px] leading-none">
      {count}
    </span>
  );
}

function ChecklistIssueItem({
  issue,
  onLocate,
}: {
  issue: CanvasIssue;
  onLocate: (issue: CanvasIssue) => void;
}) {
  const { t } = useTranslation();
  const handleClick = useCallback(() => {
    onLocate(issue);
  }, [issue, onLocate]);

  return (
    <li>
      <button
        type="button"
        data-testid="canvas-checklist-issue"
        onClick={handleClick}
        className="group flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-bg-base-hover"
      >
        <span className="size-1.5 shrink-0 rounded-full bg-state-warning" />
        <span className="flex-1 text-sm text-state-warning">
          {t(issue.messageKey, issue.messageParams)}
        </span>
        <MoveRight className="size-4 shrink-0 text-text-disabled opacity-0 group-hover:opacity-100" />
      </button>
    </li>
  );
}

/**
 * Top-right canvas checklist: a live count of canvas issues (dangling
 * variables, missing required fields, orphan steps) grouped by node. Variable
 * issues locate the node and open its form; orphan issues only highlight it.
 */
export function CanvasChecklist({ issues }: { issues: CanvasIssue[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const requestNodeFocus = useGraphStore((state) => state.requestNodeFocus);

  const groups = useMemo(() => groupIssuesByNode(issues), [issues]);

  const handleLocate = useCallback(
    (issue: CanvasIssue) => {
      requestNodeFocus(issue.nodeId, {
        toolId: issue.toolId,
        openForm: issue.type !== CanvasIssueType.Orphan,
      });
      setOpen(false);
    },
    [requestNodeFocus],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant={'secondary'} data-testid="canvas-checklist">
          <ListChecks />
          {t('flow.checklist')}
          <IssueCountBadge count={issues.length} />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 p-0">
        <div className="border-b border-border-default px-4 py-3">
          <div className="flex items-center gap-2 text-sm font-medium text-text-primary">
            <ListChecks className="size-4" />
            {t('flow.checklist')}
            <IssueCountBadge count={issues.length} />
          </div>
          <p className="mt-1 text-xs text-text-secondary">
            {t('flow.checklistTitle')}
          </p>
        </div>
        {issues.length === 0 ? (
          <div className="px-4 py-6 text-center text-sm text-text-secondary">
            {t('flow.checklistEmpty')}
          </div>
        ) : (
          <div className="max-h-80 space-y-2 overflow-y-auto p-2">
            {groups.map((group) => (
              <section key={group.key} className="rounded-lg bg-bg-card p-1.5">
                <header className="flex items-center gap-2 px-1.5 py-1 text-sm font-medium text-text-primary">
                  <OperatorIcon name={group.operatorLabel as Operator} />
                  <span className="truncate">{group.nodeName}</span>
                </header>
                <ul>
                  {group.items.map((issue, index) => (
                    <ChecklistIssueItem
                      key={`${issue.messageKey}-${issue.messageParams?.variable ?? index}`}
                      issue={issue}
                      onLocate={handleLocate}
                    />
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
