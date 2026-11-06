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

import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Plus } from 'lucide-react';
import { AgentCreateAction } from './hooks/use-agent-create-actions';

export function AgentCreateMenu({
  actions,
  directOnClick,
}: {
  actions: AgentCreateAction[];
  // Compilation has no template/import entries, so its + button skips the
  // dropdown and creates directly.
  directOnClick?: () => void;
}) {
  if (directOnClick) {
    return (
      <Button data-testid="create-agent" onClick={directOnClick}>
        <Plus className="size-[1em]" />
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger data-testid="create-agent" asChild>
        <Button>
          <Plus className="size-[1em]" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent data-testid="agent-create-menu">
        {actions.map((action) => (
          <DropdownMenuItem
            key={action.key}
            data-testid={action.testId}
            justifyBetween={false}
            onClick={action.onClick}
          >
            <action.icon />
            {action.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// The non-search empty card exposes the same per-tab create actions as the +
// dropdown, rendered as plain buttons.
export function AgentsEmptyStateActions({
  actions,
}: {
  actions: AgentCreateAction[];
}) {
  return (
    <ul className="flex flex-col gap-y-5 text-text-secondary text-sm pt-5">
      {actions.map((action) => (
        <li key={action.key}>
          <Button
            variant="static"
            size="auto"
            data-testid={
              action.key === 'blank' ? 'agents-empty-create' : undefined
            }
            onClick={action.onClick}
          >
            <action.icon className="size-[1em]" />
            {action.label}
          </Button>
        </li>
      ))}
    </ul>
  );
}
