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

import { cn } from '@/lib/utils';
import { ReactNode, useCallback } from 'react';
import { MoreButton } from './more-button';

interface ConversationListItemData {
  id: string;
  name: string;
  is_new?: boolean;
}

interface ConversationListRowProps<Item extends ConversationListItemData> {
  item: Item;
  selected: boolean;
  onSelect: (id: string, isNew: boolean) => void;
  renderActions?: (item: Item, trigger: ReactNode) => ReactNode;
  getDisplayName?: (item: Item) => string;
  testIdPrefix: string;
}

function ConversationListRow<Item extends ConversationListItemData>({
  item,
  selected,
  onSelect,
  renderActions,
  getDisplayName,
  testIdPrefix,
}: ConversationListRowProps<Item>) {
  const handleClick = useCallback(() => {
    onSelect(item.id, item.is_new ?? false);
  }, [item.id, item.is_new, onSelect]);

  const trigger = (
    <MoreButton
      className={cn(selected ? 'block' : 'hidden')}
      data-testid={`${testIdPrefix}-actions`}
      data-session-id={item.id}
    />
  );

  return (
    <li
      className="group flex min-w-0 items-center gap-1 rounded-lg pr-3 data-[selected=true]:bg-bg-card has-[>button:focus-visible]:bg-bg-card"
      data-selected={selected}
    >
      <button
        type="button"
        className="min-w-0 flex-1 truncate px-3 py-2 text-left focus-visible:outline-none"
        onClick={handleClick}
        aria-current={selected ? 'page' : undefined}
        data-testid={`${testIdPrefix}-item`}
        data-session-id={item.id}
      >
        {getDisplayName?.(item) ?? item.name}
      </button>
      {renderActions?.(item, trigger)}
    </li>
  );
}

interface ConversationListProps<Item extends ConversationListItemData> {
  items: Item[];
  label: string;
  selectedId?: string;
  onSelect: (id: string, isNew: boolean) => void;
  renderActions?: (item: Item, trigger: ReactNode) => ReactNode;
  getDisplayName?: (item: Item) => string;
  testIdPrefix?: string;
}

export function ConversationList<Item extends ConversationListItemData>({
  items,
  label,
  selectedId,
  onSelect,
  renderActions,
  getDisplayName,
  testIdPrefix = 'conversation-list',
}: ConversationListProps<Item>) {
  return (
    <nav aria-label={label}>
      <ul className="space-y-2">
        {items.map((item) => (
          <ConversationListRow
            key={item.id}
            item={item}
            selected={selectedId === item.id}
            onSelect={onSelect}
            renderActions={renderActions}
            getDisplayName={getDisplayName}
            testIdPrefix={testIdPrefix}
          />
        ))}
      </ul>
    </nav>
  );
}
