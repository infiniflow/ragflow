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

import { ActionContractBlock } from '@/components/action-contract-block';
import BackButton from '@/components/back-button';
import { OntologyEvidenceView } from '@/components/compilation-template-group/ontology/evidence-sheet';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useFetchCompilationModelImpact } from '@/hooks/use-compilation-model-request';
import { useFetchCompilationModelInstances } from '@/hooks/use-compilation-model-request';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { useFetchModelActions } from '@/hooks/use-ontology-action-request';
import {
  CompilationModelInspectScope,
  ICompilationModelInstance,
  ICompilationModelInspectParams,
  IModelAction,
} from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

const PageSize = 50;

const rowKey = (row: ICompilationModelInstance) =>
  `${row.dataset_id ?? ''}\u0000${row.id ?? row.name ?? ''}`;

/**
 * Instances and assertions: what the model produced, as a list.
 *
 * This is the ABox half of the model, and it is a page rather than a sheet
 * because browsing is the point: the reviewer's workflow is "filter to the rows
 * that are missing something, read one, move on", which a modal obstructs.
 *
 * Editing is deliberately absent. A compiled row is an assertion about a
 * document; changing it in place would leave a claim whose source no longer says
 * what the row says. Contract fields on an action are the one exception, and they
 * are edited in the action workbench, where the provenance change is explicit.
 */
export default function CompilationTemplateInstancesPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { data: group } = useFetchCompilationTemplateGroup(id);
  const [kind, setKind] = useState<'entity' | 'relation'>('entity');
  const [entityType, setEntityType] = useState('');
  const [property, setProperty] = useState('');
  const [scope, setScope] = useState<CompilationModelInspectScope>(
    CompilationModelInspectScope.Dataset,
  );
  const [datasetIds, setDatasetIds] = useState<string>('');
  const [activeKey, setActiveKey] = useState('');
  const [evidenceOpen, setEvidenceOpen] = useState(false);

  const { impact } = useFetchCompilationModelImpact(id);
  const datasets = useMemo(() => impact?.datasets ?? [], [impact]);

  // A model without a dataset picked reads its whole corpus: "everything" is the
  // question a reader opens this page with, and an empty picker would answer
  // "nothing".
  const effectiveDatasets =
    scope === CompilationModelInspectScope.Dataset
      ? datasetIds || datasets[0]?.id || ''
      : undefined;

  const params = useMemo<ICompilationModelInspectParams>(
    () =>
      kind === 'entity'
        ? {
            kind: 'entity',
            entity: entityType || undefined,
            scope,
            dataset_ids: effectiveDatasets,
            limit: PageSize,
          }
        : {
            kind: 'relation',
            property: property || undefined,
            scope,
            dataset_ids: effectiveDatasets,
            limit: PageSize,
          },
    [kind, entityType, property, scope, effectiveDatasets],
  );

  const { inspect, loading } = useFetchCompilationModelInstances(id, params);
  const { actions } = useFetchModelActions(id);

  const rows = useMemo<ICompilationModelInstance[]>(
    () =>
      kind === 'entity'
        ? (inspect?.entities ?? [])
        : (inspect?.relations ?? []),
    [kind, inspect],
  );

  /** Actions carry the contract; an instance row only names it. */
  const actionByName = useMemo(() => {
    const map = new Map<string, IModelAction>();
    (actions ?? []).forEach((action) => map.set(action.name, action));
    return map;
  }, [actions]);

  const entityOptions = useMemo(
    () => [
      {
        label: t('knowledgeCompilation.ontologyInspect.allClasses', {
          defaultValue: 'All entities',
        }),
        value: '',
      },
      ...Array.from(new Set(rows.map((row) => row.type ?? '').filter(Boolean)))
        .sort()
        .map((name) => ({ label: name, value: name })),
    ],
    [rows, t],
  );
  const propertyOptions = useMemo(
    () => [
      {
        label: t('knowledgeCompilation.ontologyInspect.allProperties', {
          defaultValue: 'All properties',
        }),
        value: '',
      },
      ...Array.from(new Set(rows.map((row) => row.name ?? '').filter(Boolean)))
        .sort()
        .map((name) => ({ label: name, value: name })),
    ],
    [rows, t],
  );
  const datasetOptions = useMemo(
    () => [
      {
        label: t('knowledgeCompilation.ontologyInspect.allDatasets', {
          defaultValue: 'All datasets',
        }),
        value: '',
      },
      ...datasets.map((dataset) => ({
        label: dataset.name,
        value: dataset.id,
      })),
    ],
    [datasets, t],
  );

  const active =
    rows.find((row) => rowKey(row) === activeKey) ?? rows[0] ?? undefined;
  const activeAction = active?.name ? actionByName.get(active.name) : undefined;
  const activeHasManualField = hasManualField(activeAction);

  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton to={backTo} />
        <h2 className="font-medium text-text-secondary">
          {t('knowledgeCompilation.ontologyInstancesPage', {
            name: group?.name ?? '',
            defaultValue: 'Instances & assertions — {{name}}',
          })}
        </h2>
        <span className="ml-auto text-xs text-text-secondary">
          {inspect?.model_version
            ? t('knowledgeCompilation.ontologyInstancesVersion', {
                version: inspect.model_version,
                defaultValue: 'compiled with v{{version}}',
              })
            : null}
        </span>
      </header>

      <div className="flex-1 min-h-0 p-4 grid grid-cols-[200px_minmax(0,1fr)_340px] gap-3 text-xs">
        <aside className="min-h-0 overflow-auto flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesKind', {
                defaultValue: 'Rows',
              })}
            </span>
            <div className="flex gap-1">
              <Button
                variant={kind === 'entity' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setKind('entity')}
              >
                {t('knowledgeCompilation.ontologyInstancesEntities', {
                  defaultValue: 'Instances',
                })}
              </Button>
              <Button
                variant={kind === 'relation' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setKind('relation')}
              >
                {t('knowledgeCompilation.ontologyInstancesRelations', {
                  defaultValue: 'Assertions',
                })}
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-1">
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesScope', {
                defaultValue: 'Scope',
              })}
            </span>
            <div className="flex gap-1">
              <Button
                variant={
                  scope === CompilationModelInspectScope.Dataset
                    ? 'default'
                    : 'outline'
                }
                size="sm"
                onClick={() => setScope(CompilationModelInspectScope.Dataset)}
              >
                {t('knowledgeCompilation.ontologyInstancesScopeDataset', {
                  defaultValue: 'Dataset',
                })}
              </Button>
              <Button
                variant={
                  scope === CompilationModelInspectScope.Corpus
                    ? 'default'
                    : 'outline'
                }
                size="sm"
                onClick={() => setScope(CompilationModelInspectScope.Corpus)}
              >
                {t('knowledgeCompilation.ontologyInstancesScopeCorpus', {
                  defaultValue: 'Corpus',
                })}
              </Button>
            </div>
          </div>

          {scope === CompilationModelInspectScope.Dataset && (
            <div className="flex flex-col gap-1">
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyInstancesDataset', {
                  defaultValue: 'Dataset',
                })}
              </span>
              <SelectWithSearch
                value={datasetIds || datasets[0]?.id || ''}
                options={datasetOptions.filter((option) => option.value !== '')}
                onChange={setDatasetIds}
              />
            </div>
          )}

          <div className="flex flex-col gap-1">
            <span className="text-text-secondary">
              {kind === 'entity'
                ? t('knowledgeCompilation.ontologyInstancesEntity', {
                    defaultValue: 'Entity',
                  })
                : t('knowledgeCompilation.ontologyInstancesProperty', {
                    defaultValue: 'Property',
                  })}
            </span>
            <SelectWithSearch
              value={kind === 'entity' ? entityType : property}
              options={kind === 'entity' ? entityOptions : propertyOptions}
              onChange={kind === 'entity' ? setEntityType : setProperty}
              allowClear
            />
          </div>
        </aside>

        <div className="min-h-0 overflow-auto flex flex-col gap-2">
          {loading && !inspect && (
            <span className="text-text-secondary">{t('common.loading')}</span>
          )}
          {!loading && rows.length === 0 && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesEmpty', {
                defaultValue:
                  'Nothing compiled yet. Publish the model, then run knowledge compilation over a dataset.',
              })}
            </span>
          )}
          {rows.map((row) => {
            const key = rowKey(row);
            const action = row.name ? actionByName.get(row.name) : undefined;
            const manual = hasManualField(action);
            return (
              <button
                key={key}
                type="button"
                onClick={() => setActiveKey(key)}
                className={cn(
                  'rounded border p-2 flex flex-col gap-1 text-start',
                  active && rowKey(active) === key
                    ? 'border-text-accent'
                    : 'border-border-button',
                )}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">
                    {row.name ??
                      (row.from && row.to ? `${row.from} → ${row.to}` : '')}
                  </span>
                  <Badge variant="secondary">
                    {row.type ?? row.name ?? ''}
                  </Badge>
                  {row.review_status === 'accepted' && (
                    <Badge variant="success">
                      {t(
                        'knowledgeCompilation.ontologyInspect.reviewAccepted',
                        {
                          defaultValue: 'Accepted',
                        },
                      )}
                    </Badge>
                  )}
                  {row.review_status === 'rejected' && (
                    <Badge variant="destructive">
                      {t(
                        'knowledgeCompilation.ontologyInspect.reviewRejected',
                        {
                          defaultValue: 'Rejected',
                        },
                      )}
                    </Badge>
                  )}
                  {manual && (
                    <span className="rounded border border-border-button px-1 text-text-disabled">
                      {t('knowledgeCompilation.ontologyInstancesManual', {
                        defaultValue: 'manual',
                      })}
                    </span>
                  )}
                  <span className="text-text-secondary">
                    {row.dataset_name ?? row.dataset_id}
                  </span>
                </div>
                {row.description && (
                  <p className="text-text-secondary">{row.description}</p>
                )}
              </button>
            );
          })}
        </div>

        <aside className="min-h-0 overflow-auto rounded border border-border-button p-2 flex flex-col gap-2">
          {active ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">
                  {active.name ??
                    (active.from && active.to
                      ? `${active.from} → ${active.to}`
                      : '')}
                </span>
                <span className="text-text-secondary">{active.type ?? ''}</span>
              </div>

              <dl className="flex flex-col gap-1">
                <Row label="scope" value={active.scope ?? ''} />
                <Row
                  label="dataset"
                  value={active.dataset_name ?? active.dataset_id ?? ''}
                />
                <Row label="version" value={String(active.version ?? '')} />
                <Row
                  label="mentions"
                  value={String(active.mention_count ?? '')}
                />
                {active.aliases?.length ? (
                  <Row label="aliases" value={active.aliases.join(', ')} />
                ) : null}
              </dl>

              {activeAction?.contract && (
                <ActionContractBlock
                  fields={activeAction.contract.fields}
                  missing={activeAction.contract.missing}
                  required={activeAction.contract.required}
                />
              )}

              {(active.source_chunk_ids?.length ?? 0) > 0 ||
              (active.evidence?.length ?? 0) > 0 ? (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEvidenceOpen(true)}
                  >
                    {t('knowledgeCompilation.ontologyInstancesEvidence', {
                      defaultValue: 'Open source text',
                    })}
                  </Button>
                  <OntologyEvidenceView
                    open={evidenceOpen}
                    datasetId={active.dataset_id ?? ''}
                    documentIds={active.document_ids ?? []}
                    chunkIds={active.source_chunk_ids ?? []}
                    evidence={active.evidence}
                  />
                </>
              ) : (
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyInstancesNoEvidence', {
                    defaultValue:
                      'No source text was recorded for this row, so it cannot be judged from here.',
                  })}
                </span>
              )}

              {activeHasManualField && (
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyInstancesManualHint', {
                    defaultValue:
                      'At least one contract field was edited by hand; its quote was dropped when it was entered.',
                  })}
                </span>
              )}
            </>
          ) : (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesPickOne', {
                defaultValue: 'Select a row to see where it came from.',
              })}
            </span>
          )}
        </aside>
      </div>
    </section>
  );
}

/**
 * Whether any contract field was entered by hand.
 *
 * Only an action carries a contract, so this is also what makes the "manual"
 * badge meaningful: an instance row cannot have been edited, because editing it
 * would separate the assertion from the sentence it was read from.
 */
function hasManualField(action?: IModelAction) {
  const fields = action?.contract?.fields;
  if (!fields) return false;
  return Object.values(fields).some((field) => field.provenance === 'manual');
}

function Row({ label, value }: { label: string; value: string }) {
  if (!value) return null;
  return (
    <div className="flex items-baseline gap-2">
      <dt className="text-text-disabled">{label}</dt>
      <dd className="break-all text-text-secondary">{value}</dd>
    </div>
  );
}
