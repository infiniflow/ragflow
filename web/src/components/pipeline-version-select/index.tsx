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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

export type PipelineVersion = {
  id: string;
  title: string;
  alias?: string | null;
};

// filterVersionsByAlias applies the free-text alias filter (design A2/F1).
// An empty or whitespace-only filter keeps every version; matching is
// case-insensitive; a null alias never matches.
export function filterVersionsByAlias<V extends PipelineVersion>(
  versions: V[],
  aliasFilter: string,
): V[] {
  const needle = aliasFilter.trim().toLowerCase();
  if (!needle) return versions;
  return versions.filter((v) => (v.alias ?? '').toLowerCase().includes(needle));
}

// PipelineVersionSelect lets the user pin a specific dataflow canvas version
// (design section 5/C2) or leave it empty to track the latest version.
// Versions can be filtered by their free-text alias (design A2/F1). Shared by
// the document pipeline dialog and the dataset settings page.
export function PipelineVersionSelect({
  versions,
  value,
  onChange,
}: {
  versions: PipelineVersion[];
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const [aliasFilter, setAliasFilter] = useState('');

  const filtered = filterVersionsByAlias(versions, aliasFilter);

  return (
    <div className="space-y-2">
      <Input
        className="h-8"
        placeholder={t('flow.version.filterByAlias')}
        value={aliasFilter}
        onChange={(e) => setAliasFilter(e.target.value)}
      />
      <Select value={value} onValueChange={(v) => onChange(v)}>
        <SelectTrigger>
          <SelectValue placeholder={t('flow.version.latest')} />
        </SelectTrigger>
        <SelectContent>
          {filtered.map((v) => (
            <SelectItem key={v.id} value={v.id}>
              {v.alias ? v.alias + ' (' + v.title + ')' : v.title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
