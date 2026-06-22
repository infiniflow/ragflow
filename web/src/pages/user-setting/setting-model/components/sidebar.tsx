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

import { LlmIcon } from '@/components/svg-icon';
import { SearchInput } from '@/components/ui/input';
import {
  LlmKeys,
  useFetchAddedProviders,
  useFetchAvailableProviders,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { cn } from '@/lib/utils';
import llmService from '@/services/llm-service';
import { useQueries } from '@tanstack/react-query';
import { ChevronRight } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Sidebar selection type for the right pane.
 * - 'default': show the system default models view.
 * - any string: a provider (LLM factory) name; show its instance list.
 */
export type SidebarSelection = 'default' | string;

interface SidebarProps {
  selection: SidebarSelection;
  onSelect: (v: SidebarSelection) => void;
}

/**
 * Left column of the v2 settings page.
 *
 * Layout (top -> bottom):
 *  1. "Default models" entry — highlighted when `selection === 'default'`,
 *     with a chevron on the right.
 *  2. "Available models" section header.
 *  3. Search input — case-insensitive filter over provider names.
 *  4. Scrollable list of available providers — clicking a row highlights
 *     it and triggers `onSelect(providerName)`. Providers that already
 *     have at least one configured instance show a green dot on the
 *     right.
 */
export function Sidebar({ selection, onSelect }: SidebarProps) {
  const { t } = useTranslation();
  const { data: providers } = useFetchAvailableProviders();
  const { data: addedProviders } = useFetchAddedProviders();
  const [search, setSearch] = useState('');

  const addedInstanceQueries = useQueries({
    queries: addedProviders.map((p) => ({
      queryKey: LlmKeys.providerInstances(p.name),
      queryFn: async () => {
        const { data } = await llmService.listProviderInstances(
          { provider_name: p.name },
          true,
        );
        return (data?.data ?? []) as IProviderInstance[];
      },
      enabled: !!p.name,
      gcTime: 0,
    })),
  });

  const addedSet = useMemo(() => {
    const names = addedProviders
      .filter((_, idx) => (addedInstanceQueries[idx]?.data?.length ?? 0) > 0)
      .map((p) => p.name);
    return new Set(names);
  }, [addedProviders, addedInstanceQueries]);

  const filteredProviders = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = q
      ? providers.filter((p) => p.name.toLowerCase().includes(q))
      : providers;
    // Stable partition: added providers first, then unadded.
    const added = list.filter((p) => addedSet.has(p.name));
    const others = list.filter((p) => !addedSet.has(p.name));
    return [...added, ...others];
  }, [providers, search, addedSet]);

  return (
    <div className="flex flex-col gap-3 py-4 text-text-primary">
      <button
        type="button"
        className={cn(
          'flex items-center justify-between px-3 py-2 rounded-md text-sm transition-colors',
          selection === 'default'
            ? 'bg-bg-input text-text-primary'
            : 'text-text-secondary hover:bg-bg-input hover:text-text-primary',
        )}
        onClick={() => onSelect('default')}
        data-testid="sidebar-default-models"
      >
        <span className="font-medium">{t('setting.systemModelSettings')}</span>
        <ChevronRight className="size-4" />
      </button>

      <div className="text-xs font-medium text-text-secondary px-1">
        {t('setting.availableModels')}
      </div>

      <div className="px-1">
        <SearchInput
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('setting.search')}
          data-testid="sidebar-provider-search"
        />
      </div>

      <div className="flex flex-col gap-1 overflow-auto scrollbar-auto">
        {filteredProviders.map((provider) => {
          const isActive = selection === provider.name;
          const isAdded = addedSet.has(provider.name);
          return (
            <button
              key={provider.name}
              type="button"
              onClick={() => onSelect(provider.name)}
              data-testid={`sidebar-provider-${provider.name}`}
              className={cn(
                'flex items-center gap-3 px-3 py-2 rounded-md text-left transition-colors',
                isActive
                  ? 'bg-bg-input text-text-primary'
                  : 'text-text-secondary hover:bg-bg-input hover:text-text-primary',
              )}
            >
              <LlmIcon
                name={provider.name}
                width={24}
                height={24}
                imgClass="size-6 text-text-primary"
              />
              <span className="truncate text-sm flex-1">{provider.name}</span>
              {isAdded && (
                <span
                  aria-label="configured"
                  className="size-2 rounded-full bg-state-success shrink-0"
                  data-testid={`sidebar-provider-dot-${provider.name}`}
                />
              )}
            </button>
          );
        })}
        {filteredProviders.length === 0 && (
          <div className="text-xs text-text-secondary px-3 py-2">
            {t('setting.empty')}
          </div>
        )}
      </div>
    </div>
  );
}

export default Sidebar;
