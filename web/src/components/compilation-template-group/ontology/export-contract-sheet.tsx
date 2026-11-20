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

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useFetchOntologyModelContract } from '@/hooks/use-ontology-model-request';
import { useFetchOntologyModelImpact } from '@/hooks/use-ontology-model-request';
import { ontologyInstancesExportUrl } from '@/services/ontology-model-service';

/**
 * The three renderings of one model version.
 *
 * They stay in one dialog because they are one artefact: a reviewer signs the
 * prose, a consumer loads the schema, and a pipeline ingests the rows. Letting
 * them be fetched from different places is how two teams end up holding
 * different versions of the same contract.
 */
export const ExportContractSheet = ({
  templateId,
  defaultVersion,
  trigger,
}: {
  templateId?: string;
  defaultVersion?: number;
  trigger?: React.ReactNode;
}) => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [version, setVersion] = useState<number | undefined>(defaultVersion);

  useEffect(() => {
    if (open) {
      setVersion(defaultVersion);
    }
  }, [open, defaultVersion]);

  const { contract, loading } = useFetchOntologyModelContract(
    templateId,
    version,
    open,
  );
  const { impact } = useFetchOntologyModelImpact(templateId);
  const datasets = useMemo(() => impact?.datasets ?? [], [impact]);
  const [datasetId, setDatasetId] = useState('');

  useEffect(() => {
    if (!datasetId && datasets.length > 0) {
      setDatasetId(datasets[0].id);
    }
  }, [datasetId, datasets]);

  const download = (content: string, filename: string, type: string) => {
    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const baseName = `${contract?.template_id || templateId || 'model'}-v${
    contract?.version ?? 0
  }`;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger ?? (
          <Button variant="ghost" size="sm" className="whitespace-nowrap">
            {t('knowledgeCompilation.ontologyExport', { defaultValue: 'Export' })}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="flex max-h-[80vh] max-w-3xl flex-col">
        <DialogHeader>
          <DialogTitle>
            {t('knowledgeCompilation.ontologyExportTitle', {
              defaultValue: 'Export contract — {{name}}',
              name: contract?.name ?? '',
            })}
          </DialogTitle>
        </DialogHeader>

        <div className="flex items-center gap-2 text-xs text-text-secondary">
          <span>
            {t('knowledgeCompilation.ontologyExportVersion', {
              defaultValue: 'Version',
            })}
          </span>
          <div className="w-40">
            <SelectWithSearch
              value={version ? String(version) : ''}
              options={[
                {
                  label: t('knowledgeCompilation.ontologyExportCurrent', {
                    defaultValue: 'Published (current)',
                  }),
                  value: '',
                },
                ...(defaultVersion
                  ? [
                      {
                        label: `v${defaultVersion}`,
                        value: String(defaultVersion),
                      },
                    ]
                  : []),
              ]}
              onChange={(value) =>
                setVersion(value ? Number(value) : undefined)
              }
            />
          </div>
          {contract?.published_at && (
            <span className="ml-auto">
              {contract.publish_status} · {contract.published_at}
            </span>
          )}
        </div>

        <Tabs defaultValue="prose" className="flex min-h-0 flex-1 flex-col">
          <TabsList className="shrink-0">
            <TabsTrigger value="prose">
              {t('knowledgeCompilation.ontologyExportProse', {
                defaultValue: 'Human-readable',
              })}
            </TabsTrigger>
            <TabsTrigger value="machine">
              {t('knowledgeCompilation.ontologyExportMachine', {
                defaultValue: 'Machine',
              })}
            </TabsTrigger>
            <TabsTrigger value="instances">
              {t('knowledgeCompilation.ontologyExportInstances', {
                defaultValue: 'Instances',
              })}
            </TabsTrigger>
          </TabsList>

          <TabsContent
            value="prose"
            className="min-h-0 flex-1 overflow-auto rounded border border-border-button bg-bg-base p-3"
          >
            {loading ? (
              <span className="text-xs text-text-secondary">
                {t('common.loading', { defaultValue: 'Loading...' })}
              </span>
            ) : (
              <pre className="whitespace-pre-wrap font-sans text-[13px] leading-6 text-text-primary">
                {contract?.markdown}
              </pre>
            )}
          </TabsContent>

          <TabsContent
            value="machine"
            className="flex min-h-0 flex-1 flex-col gap-2"
          >
            <div className="flex shrink-0 gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={!contract?.machine}
                onClick={() =>
                  download(contract!.machine, `${baseName}.yaml`, 'text/yaml')
                }
              >
                {t('knowledgeCompilation.ontologyExportYaml', {
                  defaultValue: 'Download .yaml',
                })}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!contract?.schema}
                onClick={() =>
                  download(
                    JSON.stringify(contract!.schema, null, 2),
                    `${baseName}.schema.json`,
                    'application/json',
                  )
                }
              >
                {t('knowledgeCompilation.ontologyExportSchema', {
                  defaultValue: 'Download schema.json',
                })}
              </Button>
            </div>
            <pre className="min-h-0 flex-1 overflow-auto rounded border border-border-button bg-bg-base p-3 font-mono text-[12px] leading-5 text-text-primary">
              {contract?.machine}
            </pre>
          </TabsContent>

          <TabsContent
            value="instances"
            className="flex min-h-0 flex-1 flex-col gap-2"
          >
            <p className="text-[11px] leading-5 text-text-secondary">
              {t('knowledgeCompilation.ontologyExportInstancesHint', {
                defaultValue:
                  'The compiled rows of one dataset, as JSON Lines. The file is written by the server as it streams, so it is bounded by the corpus rather than by memory.',
              })}
            </p>
            <div className="flex items-center gap-2">
              <div className="w-64">
                <SelectWithSearch
                  value={datasetId}
                  options={datasets.map((dataset) => ({
                    label: `${dataset.name} (${dataset.doc_num})`,
                    value: dataset.id,
                  }))}
                  onChange={setDatasetId}
                />
              </div>
              {datasetId && templateId && (
                <Button variant="outline" size="sm" asChild>
                  <a
                    href={ontologyInstancesExportUrl(datasetId, templateId)}
                    download
                  >
                    {t('knowledgeCompilation.ontologyExportJsonl', {
                      defaultValue: 'Download instances.jsonl',
                    })}
                  </a>
                </Button>
              )}
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
};
