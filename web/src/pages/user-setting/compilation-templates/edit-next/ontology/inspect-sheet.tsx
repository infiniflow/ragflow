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

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  useFetchCompilationModelImpact,
  useFetchCompilationModelInstances,
} from '@/hooks/use-compilation-model-request';
import {
  CompilationModelInspectScope,
  ICompilationModelInspectParams,
  ICompilationModelInstance,
} from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { Eye } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { OntologyEvidenceSheet } from './evidence-sheet';

interface OntologyInspectSheetProps {
  /** The saved template's id. A model that was never saved has no output. */
  templateId?: string;
  classes: string[];
  properties: string[];
}

interface EvidenceTarget {
  heading: string;
  datasetId?: string;
  datasetName?: string;
  documentIds: string[];
  chunkIds: string[];
}

const InspectPageSize = 50;

/**
 * Shows what the model extracted, over the datasets that compile with it.
 *
 * The question it answers — "what does my model actually produce?" — cannot be
 * asked of one dataset: a model is one template compiled by many documents
 * across many datasets, while every compiled row is stored per dataset. So the
 * scope is explicit (one dataset, or every dataset of the model) and every row
 * carries the dataset it came from.
 *
 * It is a read — no model runs — so it is free to open while editing. The
 * rehearsal that runs a draft over sample documents is the other half, and lives
 * in the model library.
 */
export function OntologyInspectSheet({
  templateId,
  classes,
  properties,
}: OntologyInspectSheetProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [scope, setScope] = useState<CompilationModelInspectScope>(
    CompilationModelInspectScope.Dataset,
  );
  const [datasetIds, setDatasetIds] = useState<string[]>([]);
  const [tab, setTab] = useState<'instances' | 'assertions'>('instances');
  const [classType, setClassType] = useState('');
  const [property, setProperty] = useState('');
  const [sourceType, setSourceType] = useState('');
  const [targetType, setTargetType] = useState('');
  const [evidence, setEvidence] = useState<EvidenceTarget | null>(null);

  const { impact, loading: datasetsLoading } = useFetchCompilationModelImpact(
    open ? templateId : undefined,
  );

  // The selection follows the model's datasets, and shrinks when one stops
  // compiling with it: opening the panel must land on something compiled rather
  // than on an empty picker.
  useEffect(() => {
    const available = impact?.datasets?.map((dataset) => dataset.id) ?? [];
    setDatasetIds((current) => {
      const kept = current.filter((id) => available.includes(id));
      if (kept.length > 0) return kept;
      return available.length > 0 ? [available[0]] : [];
    });
  }, [impact]);

  useEffect(() => {
    setClassType((current) => current || classes[0] || '');
  }, [classes]);

  useEffect(() => {
    setProperty((current) => current || properties[0] || '');
  }, [properties]);

  const isCorpus = scope === CompilationModelInspectScope.Corpus;
  const datasets = isCorpus ? undefined : datasetIds.join(',');

  const params = useMemo<ICompilationModelInspectParams>(
    () =>
      tab === 'instances'
        ? {
            class: classType || undefined,
            scope,
            dataset_ids: datasets,
            limit: InspectPageSize,
          }
        : {
            property: property || undefined,
            source_type: sourceType || undefined,
            target_type: targetType || undefined,
            scope,
            dataset_ids: datasets,
            limit: InspectPageSize,
          },
    [tab, classType, property, sourceType, targetType, scope, datasets],
  );

  const { inspect, loading } = useFetchCompilationModelInstances(
    open ? templateId : undefined,
    params,
  );

  const rows: ICompilationModelInstance[] =
    tab === 'instances'
      ? (inspect?.entities ?? [])
      : (inspect?.relations ?? []);
  const modelVersion = inspect?.model_version ?? 0;
  const classOptions = useMemo(
    () => classes.map((name) => ({ label: name, value: name })),
    [classes],
  );
  const propertyOptions = useMemo(
    () => properties.map((name) => ({ label: name, value: name })),
    [properties],
  );

  const openEvidence = (row: ICompilationModelInstance) => {
    const chunkIds = row.source_chunk_ids ?? [];
    if (chunkIds.length === 0) return;
    setEvidence({
      heading:
        tab === 'instances'
          ? `${row.name ?? ''} · ${row.dataset_name ?? ''}`
          : `${row.from ?? ''} → ${row.to ?? ''} · ${row.dataset_name ?? ''}`,
      datasetId: row.dataset_id,
      datasetName: row.dataset_name,
      documentIds: row.document_ids ?? [],
      chunkIds,
    });
  };

  const toggleDataset = (id: string) => {
    setDatasetIds((current) =>
      current.includes(id)
        ? current.filter((item) => item !== id)
        : [...current, id],
    );
  };

  return (
    <>
      <Sheet open={open} onOpenChange={setOpen} modal={false}>
        <Tooltip>
          <TooltipTrigger asChild>
            <SheetTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="whitespace-nowrap"
                disabled={!templateId}
              >
                <Eye className="size-4" />
                {t('knowledgeCompilation.ontologyInspect.open', {
                  defaultValue: 'Compiled output',
                })}
              </Button>
            </SheetTrigger>
          </TooltipTrigger>
          <TooltipContent>
            {templateId
              ? t('knowledgeCompilation.ontologyInspect.openHint', {
                  defaultValue:
                    'What this model extracted, with the source text behind it',
                })
              : t('knowledgeCompilation.ontologyInspect.saveFirst', {
                  defaultValue:
                    'Save the model first: there is no output until a dataset has compiled with it',
                })}
          </TooltipContent>
        </Tooltip>

        <SheetContent
          className="w-1/2 max-w-[760px] flex flex-col"
          onInteractOutside={(e) => e.preventDefault()}
        >
          <SheetHeader>
            <SheetTitle>
              {t('knowledgeCompilation.ontologyInspect.title', {
                defaultValue: 'Compiled output',
              })}
            </SheetTitle>
          </SheetHeader>

          <div className="mt-2 flex flex-col gap-3 text-xs">
            <p className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.intro', {
                defaultValue:
                  'What this model extracted from the datasets that compile with it. Every row comes with the dataset it came from and the source text behind it.',
              })}
            </p>

            <div className="flex flex-wrap items-center gap-2">
              <SelectWithSearch
                value={scope}
                options={[
                  {
                    label: t(
                      'knowledgeCompilation.ontologyInspect.scopeDataset',
                      { defaultValue: 'One dataset' },
                    ),
                    value: CompilationModelInspectScope.Dataset,
                  },
                  {
                    label: t(
                      'knowledgeCompilation.ontologyInspect.scopeCorpus',
                      { defaultValue: 'All datasets' },
                    ),
                    value: CompilationModelInspectScope.Corpus,
                  },
                ]}
                onChange={(value) =>
                  setScope(value as CompilationModelInspectScope)
                }
              />

              {!isCorpus && (
                <div className="flex flex-wrap items-center gap-1">
                  {datasetsLoading && (
                    <span className="text-text-secondary">
                      {t(
                        'knowledgeCompilation.ontologyInspect.datasetsLoading',
                        { defaultValue: 'Loading datasets…' },
                      )}
                    </span>
                  )}
                  {(impact?.datasets ?? []).map((dataset) => (
                    <button
                      key={dataset.id}
                      type="button"
                      onClick={() => toggleDataset(dataset.id)}
                      className={cn(
                        'rounded-full border px-2 py-0.5',
                        datasetIds.includes(dataset.id)
                          ? 'border-text-accent text-text-accent'
                          : 'border-border-button text-text-secondary',
                      )}
                    >
                      {dataset.name} ({dataset.doc_num})
                    </button>
                  ))}
                  {!datasetsLoading &&
                    (impact?.datasets ?? []).length === 0 && (
                      <span className="text-text-secondary">
                        {t('knowledgeCompilation.ontologyInspect.noDatasets', {
                          defaultValue:
                            'No dataset compiles with this model yet, so there is nothing to inspect.',
                        })}
                      </span>
                    )}
                </div>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  className={cn(
                    'border-b-2 px-1 pb-0.5',
                    tab === 'instances'
                      ? 'border-text-accent text-text-accent'
                      : 'border-transparent text-text-secondary',
                  )}
                  onClick={() => setTab('instances')}
                >
                  {t('knowledgeCompilation.ontologyInspect.instances', {
                    defaultValue: 'Instances',
                  })}
                </button>
                <button
                  type="button"
                  className={cn(
                    'border-b-2 px-1 pb-0.5',
                    tab === 'assertions'
                      ? 'border-text-accent text-text-accent'
                      : 'border-transparent text-text-secondary',
                  )}
                  onClick={() => setTab('assertions')}
                >
                  {t('knowledgeCompilation.ontologyInspect.assertions', {
                    defaultValue: 'Assertions',
                  })}
                </button>
              </div>

              {tab === 'instances' ? (
                <SelectWithSearch
                  value={classType}
                  options={classOptions}
                  onChange={setClassType}
                />
              ) : (
                <div className="flex items-center gap-2">
                  <SelectWithSearch
                    value={property}
                    options={propertyOptions}
                    onChange={setProperty}
                  />
                  <SelectWithSearch
                    value={sourceType}
                    options={[{ label: '—', value: '' }, ...classOptions]}
                    onChange={setSourceType}
                  />
                  <SelectWithSearch
                    value={targetType}
                    options={[{ label: '—', value: '' }, ...classOptions]}
                    onChange={setTargetType}
                  />
                </div>
              )}

              {inspect && (
                <span className="ml-auto text-text-secondary">
                  {t('knowledgeCompilation.ontologyInspect.summary', {
                    count: inspect.total,
                    datasets: inspect.datasets.length,
                    version: modelVersion,
                    defaultValue:
                      '{{count}} in scope · {{datasets}} dataset(s) · model v{{version}}',
                  })}
                </span>
              )}
            </div>

            {inspect?.truncated && (
              <div className="rounded border border-border-button p-2 text-text-secondary">
                {t('knowledgeCompilation.ontologyInspect.truncated', {
                  defaultValue:
                    'Showing a capped window, not everything in scope. Narrow the datasets or the class to see the rest.',
                })}
                {(inspect.skipped_dataset_ids ?? []).length > 0 && (
                  <div className="mt-1 break-all">
                    {t('knowledgeCompilation.ontologyInspect.skipped', {
                      ids: (inspect.skipped_dataset_ids ?? []).join(', '),
                      defaultValue: 'Not read: {{ids}}',
                    })}
                  </div>
                )}
              </div>
            )}
          </div>

          <div className="mt-4 flex-1 min-h-0 overflow-auto flex flex-col gap-2 text-xs">
            {loading && (
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyInspect.loading', {
                  defaultValue: 'Reading compiled rows…',
                })}
              </span>
            )}

            {!loading && rows.length === 0 && (
              <div className="text-text-secondary flex flex-col gap-1">
                <span>
                  {t('knowledgeCompilation.ontologyInspect.empty', {
                    defaultValue: 'Nothing in this scope.',
                  })}
                </span>
                <span>
                  {t('knowledgeCompilation.ontologyInspect.emptyHint', {
                    defaultValue:
                      'Either these datasets have not been compiled with this model yet, or the model does not extract this one.',
                  })}
                </span>
              </div>
            )}

            {rows.map((row, index) => {
              const stale =
                !!row.version && modelVersion > 0 && row.version < modelVersion;
              const chunkCount = row.source_chunk_ids?.length ?? 0;
              return (
                <article
                  key={`${row.dataset_id ?? ''}-${row.name ?? row.from ?? ''}-${index}`}
                  className="rounded border border-border-button p-2 flex flex-col gap-1"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {tab === 'instances'
                        ? row.name
                        : `${row.from ?? ''} → ${row.to ?? ''}`}
                    </span>
                    <span className="text-text-secondary">{row.type}</span>
                    {row.version && (
                      <span
                        className={
                          stale ? 'text-state-warning' : 'text-text-secondary'
                        }
                        title={t(
                          'knowledgeCompilation.ontologyInspect.staleVersion',
                          {
                            version: row.version,
                            current: modelVersion,
                            defaultValue:
                              'Extracted by v{{version}}; the model is now v{{current}}, so this row predates it',
                          },
                        )}
                      >
                        v{row.version}
                      </span>
                    )}
                    {isCorpus && (
                      <span className="ml-auto text-text-secondary">
                        {row.dataset_name}
                      </span>
                    )}
                  </div>

                  {row.description && (
                    <p className="text-text-secondary">{row.description}</p>
                  )}

                  {chunkCount > 0 ? (
                    <button
                      type="button"
                      className="self-start text-text-accent hover:underline"
                      onClick={() => openEvidence(row)}
                    >
                      {t('knowledgeCompilation.ontologyInspect.evidence', {
                        count: chunkCount,
                        defaultValue: 'Source text ({{count}})',
                      })}
                    </button>
                  ) : (
                    <span className="text-text-secondary">
                      {t(
                        'knowledgeCompilation.ontologyInspect.evidenceMissing',
                        {
                          defaultValue: 'No source text recorded for this row',
                        },
                      )}
                    </span>
                  )}
                </article>
              );
            })}
          </div>
        </SheetContent>
      </Sheet>

      <OntologyEvidenceSheet
        open={!!evidence}
        onOpenChange={(next) => {
          if (!next) setEvidence(null);
        }}
        datasetId={evidence?.datasetId}
        datasetName={evidence?.datasetName}
        heading={evidence?.heading}
        documentIds={evidence?.documentIds}
        chunkIds={evidence?.chunkIds}
      />
    </>
  );
}

export default OntologyInspectSheet;
