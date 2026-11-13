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

import type {
  IStructureGraphEntity,
  IStructureGraphRelation,
} from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

import { classColor } from './adapters';
import { AboxPageSize } from './hooks';

/** The class pill. Colour is the same hash the canvas uses, so a row and its
 * bubble read as the same class. */
export function ClassPill({ type }: { type?: string }) {
  const color = classColor(type);
  return (
    <span
      className="inline-flex h-5 items-center gap-1 rounded-full border px-2 text-[10px]"
      style={{
        borderColor: `${color}66`,
        backgroundColor: `${color}1f`,
      }}
    >
      <span
        className="inline-block size-1.5 rounded-full"
        style={{ backgroundColor: color }}
      />
      {type || '—'}
    </span>
  );
}

function Pager({
  total,
  offset,
  onOffset,
}: {
  total: number;
  offset: number;
  onOffset: (next: number) => void;
}) {
  const { t } = useTranslation();
  const start = total === 0 ? 0 : offset + 1;
  const end = Math.min(offset + AboxPageSize, total);
  return (
    <div className="flex flex-none items-center gap-2 border-t border-border-button px-3 py-2">
      <span className="text-xs tabular-nums text-text-secondary">
        {t('knowledgeCompilation.aboxPageRange', {
          defaultValue: '{{start}}–{{end}} of {{total}}',
          start,
          end,
          total,
        })}
      </span>
      <div className="ml-auto flex gap-1">
        <button
          type="button"
          disabled={offset <= 0}
          onClick={() => onOffset(Math.max(offset - AboxPageSize, 0))}
          className="h-6 rounded border border-border-button px-2 text-xs disabled:opacity-40"
        >
          {t('knowledgeCompilation.aboxPrevious', { defaultValue: 'Previous' })}
        </button>
        <button
          type="button"
          disabled={offset + AboxPageSize >= total}
          onClick={() => onOffset(offset + AboxPageSize)}
          className="h-6 rounded border border-border-button px-2 text-xs disabled:opacity-40"
        >
          {t('knowledgeCompilation.aboxNext', { defaultValue: 'Next' })}
        </button>
      </div>
    </div>
  );
}

const EmptyRow = ({ text }: { text: string }) => (
  <div className="flex flex-1 items-center justify-center p-6 text-xs text-text-secondary">
    {text}
  </div>
);

/** Rows of instances — the view that can reach the last row. */
export function EntityTable({
  rows,
  total,
  offset,
  onOffset,
  activeKey,
  onSelect,
  onOpenEvidence,
}: {
  rows: IStructureGraphEntity[];
  total: number;
  offset: number;
  onOffset: (next: number) => void;
  activeKey: string;
  onSelect: (row: IStructureGraphEntity, key: string) => void;
  onOpenEvidence: (row: IStructureGraphEntity) => void;
}) {
  const { t } = useTranslation();
  if (!rows.length) {
    return (
      <EmptyRow
        text={t('knowledgeCompilation.ontologyInstancesEmpty', {
          defaultValue: 'Nothing compiled for this filter yet.',
        })}
      />
    );
  }
  return (
    <>
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-collapse text-xs">
          <thead className="sticky top-0 z-10 bg-bg-base">
            <tr className="text-[10px] uppercase tracking-wide text-text-secondary">
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyInstancesEntities', {
                  defaultValue: 'Entities',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyTypeColumn', {
                  defaultValue: 'Class',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyAliasColumn', {
                  defaultValue: 'Aliases',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyDescriptionColumn', {
                  defaultValue: 'Description',
                })}
              </th>
              <th className="px-3 py-2 text-right font-medium">
                {t('knowledgeCompilation.ontologyAttributeColumn', {
                  defaultValue: 'Values',
                })}
              </th>
              <th className="px-3 py-2 text-right font-medium">
                {t('knowledgeCompilation.aboxSourceDocs', {
                  defaultValue: 'Sources',
                })}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => {
              const key = `${row.scope ?? ''}:${row.doc_id ?? ''}:${row.name ?? ''}:${index}`;
              return (
                <tr
                  key={key}
                  onClick={() => onSelect(row, key)}
                  className={cn(
                    'cursor-pointer border-b border-border-button/60 hover:bg-bg-base',
                    key === activeKey && 'bg-bg-base',
                  )}
                >
                  <td className="max-w-[220px] truncate px-3 py-2 font-medium">
                    {row.name || '—'}
                  </td>
                  <td className="px-3 py-2">
                    <ClassPill type={row.type} />
                  </td>
                  <td className="max-w-[180px] truncate px-3 py-2 text-text-secondary">
                    {row.aliases?.length ? row.aliases.join('、') : '—'}
                  </td>
                  <td className="max-w-[300px] truncate px-3 py-2 text-text-secondary">
                    {row.description || row.discription || '—'}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-text-secondary">
                    {Object.keys(row.attributes ?? {}).length}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <button
                      type="button"
                      className="text-xs text-text-accent hover:underline"
                      onClick={(event) => {
                        event.stopPropagation();
                        onOpenEvidence(row);
                      }}
                    >
                      {row.document_ids?.length ?? 0}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <Pager total={total} offset={offset} onOffset={onOffset} />
    </>
  );
}

/** Rows of assertions. */
export function RelationTable({
  rows,
  total,
  offset,
  onOffset,
  activeKey,
  onSelect,
  onOpenEvidence,
}: {
  rows: IStructureGraphRelation[];
  total: number;
  offset: number;
  onOffset: (next: number) => void;
  activeKey: string;
  onSelect: (row: IStructureGraphRelation, key: string) => void;
  onOpenEvidence: (row: IStructureGraphRelation) => void;
}) {
  const { t } = useTranslation();
  if (!rows.length) {
    return (
      <EmptyRow
        text={t('knowledgeCompilation.ontologyInstancesEmpty', {
          defaultValue: 'Nothing compiled for this filter yet.',
        })}
      />
    );
  }
  return (
    <>
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-collapse text-xs">
          <thead className="sticky top-0 z-10 bg-bg-base">
            <tr className="text-[10px] uppercase tracking-wide text-text-secondary">
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyFromColumn', {
                  defaultValue: 'From',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyPropertyColumn', {
                  defaultValue: 'Relation',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyToColumn', {
                  defaultValue: 'To',
                })}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('knowledgeCompilation.ontologyEndpointsColumn', {
                  defaultValue: 'Endpoints',
                })}
              </th>
              <th className="px-3 py-2 text-right font-medium">
                {t('knowledgeCompilation.aboxSourceDocs', {
                  defaultValue: 'Sources',
                })}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => {
              const key = `${row.doc_id ?? ''}:${row.from}:${row.type ?? ''}:${row.to}:${index}`;
              return (
                <tr
                  key={key}
                  onClick={() => onSelect(row, key)}
                  className={cn(
                    'cursor-pointer border-b border-border-button/60 hover:bg-bg-base',
                    key === activeKey && 'bg-bg-base',
                  )}
                >
                  <td className="max-w-[200px] truncate px-3 py-2 font-medium">
                    {row.from || '—'}
                  </td>
                  <td className="px-3 py-2 font-mono text-[11px] text-text-secondary">
                    {row.type || '—'}
                  </td>
                  <td className="max-w-[200px] truncate px-3 py-2 font-medium">
                    {row.to || '—'}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center gap-1">
                      <ClassPill type={row.source_type} />
                      <span className="text-text-secondary">→</span>
                      <ClassPill type={row.target_type} />
                    </div>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <button
                      type="button"
                      className="text-xs text-text-accent hover:underline"
                      onClick={(event) => {
                        event.stopPropagation();
                        onOpenEvidence(row);
                      }}
                    >
                      {row.document_ids?.length ?? 0}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <Pager total={total} offset={offset} onOffset={onOffset} />
    </>
  );
}
