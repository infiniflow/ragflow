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

import { Input } from '@/components/ui/input';
import type { IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { classColor } from './adapters';

/**
 * Search over the canvas that is already on screen.
 *
 * It filters nothing out and asks the server for nothing: the graph has a node
 * budget, so the nodes to search are exactly the ones drawn. Typing narrows a
 * list of results; picking one selects that node and lets the graph dim the rest
 * — which is also how a reader discovers that a match has neighbours.
 *
 * A search box that silently looked only at the drawn subset would be a lie, so
 * the empty state says it looked at this canvas, not at the class.
 */
export function AboxSearchBox({
  value,
  onChange,
  results,
  onPick,
}: {
  value: string;
  onChange: (next: string) => void;
  results: IArtifactGraphEntity[];
  onPick: (entity: IArtifactGraphEntity) => void;
}) {
  const { t } = useTranslation();
  const open = value.trim().length > 0;

  return (
    <div className="absolute left-3 top-3 z-10 w-60">
      <div className="relative">
        <Search className="pointer-events-none absolute left-2 top-1.5 size-3.5 text-text-secondary" />
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && results[0]) {
              onPick(results[0]);
            }
            if (event.key === 'Escape') {
              onChange('');
            }
          }}
          placeholder={t('knowledgeCompilation.aboxSearchPlaceholder', {
            defaultValue: 'Search instances on this canvas',
          })}
          className="h-7 bg-bg-base/95 pl-7 text-xs"
        />
      </div>

      {open && (
        <div className="mt-1 overflow-hidden rounded border border-border-button bg-bg-base shadow-md">
          {results.length === 0 ? (
            <p className="px-2 py-2 text-[10px] text-text-secondary">
              {t('knowledgeCompilation.aboxSearchNoMatch', {
                defaultValue: 'Nothing on this canvas matches.',
              })}
            </p>
          ) : (
            <ul className="max-h-56 overflow-auto">
              {results.map((entity) => (
                <li key={entity.slug}>
                  <button
                    type="button"
                    onClick={() => onPick(entity)}
                    className="flex w-full items-center gap-2 px-2 py-1 text-start text-xs hover:bg-bg-component"
                  >
                    <span
                      className={cn('size-1.5 flex-none rounded-full')}
                      style={{ backgroundColor: classColor(entity.type) }}
                    />
                    <span className="truncate">{entity.name}</span>
                    <span className="ml-auto truncate text-[10px] text-text-secondary">
                      {entity.type}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
