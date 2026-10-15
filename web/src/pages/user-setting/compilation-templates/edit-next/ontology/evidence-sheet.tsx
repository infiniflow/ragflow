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

import { useQueries } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import kbService from '@/services/knowledge-service';

interface OntologyEvidenceSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The dataset the row came from — a chunk is only addressable inside one. */
  datasetId?: string;
  datasetName?: string;
  heading?: string;
  documentIds?: string[];
  chunkIds?: string[];
}

interface EvidenceChunk {
  id: string;
  text: string;
}

/**
 * One request per document, because the chunk endpoint is document-scoped and a
 * dataset-scope row's chunks come from every document that mentioned the
 * instance. Asking once per document is what keeps the sheet honest about "this
 * instance was read in three documents" instead of showing one of them.
 */
const loadEvidenceChunks = async (
  datasetId: string,
  documentId: string,
  chunkIds: string[],
): Promise<EvidenceChunk[]> => {
  const response = await kbService.chunkList({
    dataset_id: datasetId,
    document_id: documentId,
    chunk_ids: chunkIds,
    page_size: 100,
  });
  const chunks = (response?.data?.data?.chunks ?? []) as Record<
    string,
    unknown
  >[];
  return chunks.map((chunk) => ({
    id: String(chunk.chunk_id ?? chunk.id ?? ''),
    text: String(chunk.content_with_weight ?? ''),
  }));
};

/**
 * Shows the text an instance or an assertion was extracted from.
 *
 * This is the point of the panel it opens from: a class list says what the model
 * believes, and the source text is the only thing that lets a reader decide
 * whether the belief is right.
 */
export function OntologyEvidenceSheet({
  open,
  onOpenChange,
  datasetId,
  datasetName,
  heading,
  documentIds,
  chunkIds,
}: OntologyEvidenceSheetProps) {
  const { t } = useTranslation();
  const documents = documentIds ?? [];
  const ids = chunkIds ?? [];

  const queries = useQueries({
    queries: documents.map((documentId) => ({
      queryKey: ['ontology-evidence', datasetId, documentId, ids],
      enabled: open && !!datasetId && ids.length > 0,
      queryFn: () => loadEvidenceChunks(datasetId as string, documentId, ids),
    })),
  });

  const chunks = queries.flatMap((query) => query.data ?? []);
  const loading = queries.some((query) => query.isLoading);

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent
        className="w-1/2 max-w-[700px] flex flex-col"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle>
            {t('knowledgeCompilation.ontologyInspect.evidenceTitle', {
              defaultValue: 'Source text',
            })}
          </SheetTitle>
        </SheetHeader>

        <div className="mt-1 text-xs text-text-secondary break-all">
          {heading}
          {datasetName ? ` · ${datasetName}` : ''}
        </div>

        <div className="mt-4 flex-1 min-h-0 overflow-auto flex flex-col gap-2 text-xs">
          {loading && chunks.length === 0 && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.evidenceLoading', {
                defaultValue: 'Loading…',
              })}
            </span>
          )}

          {!loading && chunks.length === 0 && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.evidenceNone', {
                defaultValue:
                  'No source text was recorded for this row. It was extracted before the row kept its chunks, or the text has since been deleted.',
              })}
            </span>
          )}

          {chunks.map((chunk) => (
            <article
              key={chunk.id}
              className="rounded border border-border-button p-2 flex flex-col gap-1"
            >
              <span className="text-text-secondary break-all">{chunk.id}</span>
              <p className="whitespace-pre-wrap">{chunk.text}</p>
            </article>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  );
}

export default OntologyEvidenceSheet;
