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
import { useTranslation } from 'react-i18next';

import { classColor } from './adapters';
import { ClassPill } from './tables';

/** The neighbours of the selected node, as the drawn edges know them. */
export interface AboxNeighbour {
  type: string;
  name: string;
}

const Empty = ({ text }: { text: string }) => (
  <p className="text-xs text-text-secondary">{text}</p>
);

const AttributeList = ({ row }: { row: IStructureGraphEntity }) => {
  const { t } = useTranslation();
  const entries = Object.entries(row.attributes ?? {});
  if (!entries.length) {
    return (
      <Empty
        text={t('knowledgeCompilation.ontologyInstancesNoEvidence', {
          defaultValue: 'No source text was kept for this row.',
        })}
      />
    );
  }
  return (
    <dl className="flex flex-col gap-1">
      {entries.map(([key, value]) => (
        <div key={key} className="flex gap-2 text-xs">
          <dt className="w-20 shrink-0 truncate text-text-secondary">{key}</dt>
          <dd className="min-w-0 flex-1 truncate">
            {Array.isArray(value) ? value.join('、') : String(value)}
          </dd>
        </div>
      ))}
    </dl>
  );
};

/**
 * What the canvas selected, in full.
 *
 * The panel is the reason a bubble graph is usable at this density: a node can
 * only carry a name, so everything that answers "should I trust this row" —
 * aliases, values, neighbours, the documents behind it — has to live beside the
 * canvas rather than on it.
 */
export function AboxDetail({
  row,
  neighbours,
  onOpenEvidence,
  onOpenInList,
}: {
  row?: IStructureGraphEntity | IStructureGraphRelation;
  neighbours: AboxNeighbour[];
  onOpenEvidence: (
    row: IStructureGraphEntity | IStructureGraphRelation,
  ) => void;
  onOpenInList?: (row: IStructureGraphEntity | IStructureGraphRelation) => void;
}) {
  const { t } = useTranslation();
  if (!row) {
    return (
      <aside className="w-80 flex-none overflow-auto border-l border-border-button p-4">
        <Empty
          text={t('knowledgeCompilation.ontologyInstancesPickOne', {
            defaultValue: 'Select a row to read its source.',
          })}
        />
      </aside>
    );
  }

  const isRelation = 'from' in row;
  const entity = isRelation ? undefined : (row as IStructureGraphEntity);
  const relation = isRelation ? (row as IStructureGraphRelation) : undefined;
  const name = entity ? entity.name : `${relation?.from} → ${relation?.to}`;
  const documents = row.document_ids ?? [];
  const color = classColor(entity?.type ?? relation?.source_type);

  return (
    <aside className="w-80 flex-none overflow-auto border-l border-border-button p-4">
      <div className="mb-3 flex items-start gap-2">
        <span
          className="flex size-9 flex-none items-center justify-center rounded-lg text-sm font-semibold text-white"
          style={{ backgroundColor: color }}
        >
          {(name ?? '?').trim().slice(0, 1)}
        </span>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{name || '—'}</div>
          <div className="mt-1 flex items-center gap-1">
            {entity ? (
              <ClassPill type={entity.type} />
            ) : (
              <>
                <ClassPill type={relation?.source_type} />
                <span className="text-xs text-text-secondary">→</span>
                <ClassPill type={relation?.target_type} />
              </>
            )}
          </div>
        </div>
      </div>

      <dl className="mb-3 grid grid-cols-[56px_minmax(0,1fr)] gap-y-1 text-xs">
        <dt className="text-text-secondary">
          {t('knowledgeCompilation.ontologyRelationsColumn', {
            defaultValue: 'Relations',
          })}
        </dt>
        <dd className="tabular-nums">{neighbours.length}</dd>
        <dt className="text-text-secondary">
          {t('knowledgeCompilation.aboxSourceDocs', {
            defaultValue: 'Sources',
          })}
        </dt>
        <dd className="tabular-nums">{documents.length}</dd>
        {entity?.mention_count !== undefined && (
          <>
            <dt className="text-text-secondary">
              {t('knowledgeCompilation.ontologyMentionsColumn', {
                defaultValue: 'Mentions',
              })}
            </dt>
            <dd className="tabular-nums">{entity.mention_count}</dd>
          </>
        )}
        <dt className="text-text-secondary">
          {t('knowledgeCompilation.ontologyAliasColumn', {
            defaultValue: 'Aliases',
          })}
        </dt>
        <dd className="truncate">
          {entity?.aliases?.length ? entity.aliases.join('、') : '—'}
        </dd>
      </dl>

      {entity && (
        <section className="mb-3">
          <h3 className="mb-1 text-[10px] uppercase tracking-wide text-text-secondary">
            {t('knowledgeCompilation.ontologyAttributeColumn', {
              defaultValue: 'Values',
            })}
          </h3>
          <AttributeList row={entity} />
        </section>
      )}

      {neighbours.length > 0 && (
        <section className="mb-3">
          <h3 className="mb-1 text-[10px] uppercase tracking-wide text-text-secondary">
            {t('knowledgeCompilation.ontologyRelationsColumn', {
              defaultValue: 'Relations',
            })}
          </h3>
          <div className="flex flex-col gap-1">
            {neighbours.map((item) => (
              <div
                key={`${item.type}:${item.name}`}
                className="flex items-center gap-2 rounded bg-bg-base px-2 py-1 text-xs"
              >
                <span className="truncate text-text-secondary">
                  {item.type}
                </span>
                <span className="ml-auto max-w-[60%] truncate">
                  {item.name}
                </span>
              </div>
            ))}
          </div>
        </section>
      )}

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={() => onOpenEvidence(row)}
          className="h-7 rounded border border-border-button px-3 text-xs"
        >
          {t('knowledgeCompilation.ontologyOpenEvidence', {
            defaultValue: 'View source',
          })}
        </button>
        {onOpenInList && (
          <button
            type="button"
            onClick={() => onOpenInList(row)}
            className="h-7 rounded border border-border-button px-3 text-xs"
          >
            {t('knowledgeCompilation.aboxOpenInList', {
              defaultValue: 'Open in list',
            })}
          </button>
        )}
      </div>
    </aside>
  );
}
