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

import BackButton from '@/components/back-button';
import { OntologyAboxPanel } from '@/components/ontology-abox';
import { OntologyReviewSheet } from '@/components/compilation-template-group/ontology/review-sheet';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import { useFetchCompilationModelImpact } from '@/hooks/use-compilation-model-request';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import type { ICompilationTemplate } from '@/interfaces/database/compilation-template';
import { CompilationTemplateKind } from '@/constants/compilation';
import { Routes } from '@/routes';
import {
  ontologyTemplateIdOf,
  ReviewSourceActions,
  ReviewSourceParam,
} from '@/utils/compilation-template-util';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, useSearchParams } from 'react-router';

/**
 * The ABox of one model, as a page.
 *
 * The rows live in a dataset, so the page opens on one and lets the reader
 * switch. It used to ask a model-scoped endpoint that fanned out over every
 * bound dataset and then cut the merged list in memory — a page of 50 cost 50
 * rows from each of up to 20 datasets, the total was those per-dataset row
 * counts added together (document rows and the dataset-level folded row both
 * counting), and there was no way to ask for the second page. Reading one
 * dataset at a time is the same read the dataset and document pages make, so
 * there is one path and the numbers on it add up.
 *
 * Editing is deliberately absent. A compiled row is an assertion about a
 * document; changing it in place would leave a claim whose source no longer
 * says what the row says. Judging rows belongs to the review queue.
 */
export default function CompilationTemplateInstancesPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  /**
   * The actions page links here with `?review=1&class=action&from=actions`: a
   * row that says "awaiting review" must lead to the place where it can be
   * reviewed, or the badge is a dead end.
   */
  const [reviewOpen, setReviewOpen] = useState(
    searchParams.get('review') === '1',
  );
  const reviewClass = searchParams.get('class') ?? undefined;
  const reviewSource = searchParams.get(ReviewSourceParam);
  /**
   * Closing the queue returns the reader where they came from.
   *
   * The queue is the *only* place a verdict can be recorded, which is why the
   * actions page links into it; but this page is not where that reader works.
   * Leaving them here would strand them on a workspace they never chose, with
   * the queue already closed.
   *
   * `replace` keeps the review URL out of history: Back must not reopen a sheet
   * the reader just dismissed.
   */
  const handleReviewOpenChange = useCallback(
    (open: boolean) => {
      setReviewOpen(open);
      if (open || reviewSource !== ReviewSourceActions || !id) return;
      navigate(`${Routes.CompilationTemplatesEditNext}/${id}/actions`, {
        replace: true,
      });
    },
    [id, navigate, reviewSource],
  );
  const { data: group } = useFetchCompilationTemplateGroup(id);
  // `:id` addresses the group (the editor route's unit); every model-scoped read
  // below is keyed by the template row inside it.
  const templateId = ontologyTemplateIdOf(group);
  const { impact } = useFetchCompilationModelImpact(templateId);
  const datasets = useMemo(() => impact?.datasets ?? [], [impact]);
  const [datasetId, setDatasetId] = useState('');

  // A model without a dataset picked reads its largest corpus first: "what did
  // this model produce" is the question the page opens with, and an empty
  // picker would answer "nothing".
  useEffect(() => {
    if (!datasetId && datasets.length > 0) {
      setDatasetId(datasets[0].id);
    }
  }, [datasetId, datasets]);

  const datasetOptions = useMemo(
    () =>
      datasets.map((dataset) => ({
        label: `${dataset.name} (${dataset.doc_num})`,
        value: dataset.id,
      })),
    [datasets],
  );

  const backTo = `${Routes.CompilationTemplatesEditNext}/${id}`;

  return (
    <section className="flex h-full flex-col bg-bg-base">
      <header className="flex shrink-0 items-center gap-3 border-b border-border-button px-5 py-4">
        <BackButton to={backTo} />
        <h2 className="font-medium text-text-secondary">
          {t('knowledgeCompilation.ontologyInstancesPage', {
            name: group?.name ?? '',
            defaultValue: 'Instances & assertions — {{name}}',
          })}
        </h2>
        <div className="ml-auto flex items-center gap-2">
          <div className="w-72">
            <SelectWithSearch
              value={datasetId}
              options={datasetOptions}
              onChange={setDatasetId}
            />
          </div>
          {/* Judging a row and browsing rows are the same session's two halves,
              so the queue opens from here rather than from a separate page. */}
          <OntologyReviewSheet
            templateId={templateId}
            open={reviewOpen}
            onOpenChange={handleReviewOpenChange}
            initialClass={reviewClass}
            entities={declaredClasses(group?.templates, templateId)?.map(
              (item) => item.type,
            )}
            trigger={
              <Button variant="outline" size="sm">
                {t('knowledgeCompilation.ontologyOpenReview', {
                  defaultValue: 'Review',
                })}
              </Button>
            }
          />
        </div>
      </header>

      <div className="min-h-0 flex-1 p-4">
        {datasetId ? (
          <OntologyAboxPanel
            datasetId={datasetId}
            templateId={templateId}
            classes={declaredClasses(group?.templates, templateId)}
          />
        ) : (
          <span className="text-xs text-text-secondary">
            {t('knowledgeCompilation.ontologyInstancesNoDataset', {
              defaultValue:
                'No dataset compiles with this model yet, so there are no rows to read.',
            })}
          </span>
        )}
      </div>
    </section>
  );
}

/**
 * declaredClasses reads the model's class list out of its working config.
 *
 * The filter should offer the vocabulary the model promises, not the classes
 * that happened to land on the first page — otherwise a declared class with no
 * instances yet is unreachable, and its emptiness is exactly what the reader
 * came to check.
 */
function declaredClasses(
  templates: ICompilationTemplate[] | undefined,
  templateId?: string,
) {
  const template =
    templates?.find((item) => item.id === templateId) ??
    templates?.find((item) => item.kind === CompilationTemplateKind.Ontology);
  const entity = template?.config?.entity as
    | { fields?: { type?: string; label?: string }[] }
    | undefined;
  const fields = (entity?.fields ?? []).filter((field) => !!field?.type);
  return fields.length > 0
    ? fields.map((field) => ({
        type: field.type as string,
        label: field.label,
      }))
    : undefined;
}
