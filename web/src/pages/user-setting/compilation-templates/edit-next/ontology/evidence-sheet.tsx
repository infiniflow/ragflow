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

import { SearchHighlight } from '@/components/search-highlight';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IOntologyEvidence } from '@/interfaces/database/compilation-model';
import kbService from '@/services/knowledge-service';
import { useQueries } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

interface OntologyEvidenceSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The dataset the row came from — a chunk is only addressable inside one. */
  datasetId?: string;
  datasetName?: string;
  heading?: string;
  documentIds?: string[];
  chunkIds?: string[];
  /** The quotes the assertion was read from, when the template asked for them. */
  evidence?: IOntologyEvidence[];
}

export interface EvidenceChunk {
  id: string;
  text: string;
}

/**
 * Characters of source kept on each side of a quote. Enough to read the quote in
 * the sentence it belongs to — which is the whole reason to show the source —
 * without turning the panel into a document reader.
 */
const contextWindow = 180;

/**
 * One request per document, because the chunk endpoint is document-scoped and a
 * dataset-scope row's chunks come from every document that mentioned the
 * instance. Asking once per document is what keeps the sheet honest about "this
 * instance was read in three documents" instead of showing one of them.
 */
export const loadEvidenceChunks = async (
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
 *
 * Two shapes, because the corpus has two generations of rows. A row compiled by
 * a template that asks for evidence carries the quote and its location, and the
 * quote is what the reader came to check — it is shown first, in the sentence
 * around it. A row compiled before that carries only chunk ids, and then the
 * whole chunk is the only source there is. Showing the quote when it exists
 * matters: a twenty-line chunk makes the reader find the sentence, and finding
 * it is exactly the judgement they came to make.
 */
export function OntologyEvidenceSheet({
  open,
  onOpenChange,
  datasetId,
  datasetName,
  heading,
  documentIds,
  chunkIds,
  evidence,
}: OntologyEvidenceSheetProps) {
  const { t } = useTranslation();

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

        <div className="mt-4 flex-1 min-h-0 overflow-auto flex flex-col gap-3 text-xs">
          <OntologyEvidenceView
            open={open}
            datasetId={datasetId}
            documentIds={documentIds}
            chunkIds={chunkIds}
            evidence={evidence}
          />
        </div>
      </SheetContent>
    </Sheet>
  );
}

interface OntologyEvidenceViewProps {
  /** Gates the chunk reads: a closed panel reads nothing. */
  open?: boolean;
  datasetId?: string;
  documentIds?: string[];
  chunkIds?: string[];
  evidence?: IOntologyEvidence[];
}

/**
 * The source text of one row, rendered where the caller lays it out.
 *
 * It is split out of the sheet above because the review queue needs the same
 * thing beside its rows: a reviewer reads the quote and decides. Keeping one
 * implementation is what stops the two panels from disagreeing about which text
 * an assertion came from.
 */
export function OntologyEvidenceView({
  open = true,
  datasetId,
  documentIds,
  chunkIds,
  evidence,
}: OntologyEvidenceViewProps) {
  const { t } = useTranslation();
  const documents = documentIds ?? [];
  const quotes = useMemo(() => evidence ?? [], [evidence]);

  // A quote names its own chunk, and that chunk is not necessarily in the row's
  // source_chunk_ids: re-chunking after compilation can leave the two out of
  // step. Asking for both is what keeps the quote resolvable.
  const ids = useMemo(() => {
    const set = new Set<string>(chunkIds ?? []);
    quotes.forEach((entry) => {
      if (entry.chunk_id) {
        set.add(entry.chunk_id);
      }
    });
    return Array.from(set);
  }, [chunkIds, quotes]);

  const queries = useQueries({
    queries: documents.map((documentId) => ({
      queryKey: ['ontology-evidence', datasetId, documentId, ids],
      enabled: open && !!datasetId && ids.length > 0,
      queryFn: () => loadEvidenceChunks(datasetId as string, documentId, ids),
    })),
  });

  const chunks = queries.flatMap((query) => query.data ?? []);
  const loading = queries.some((query) => query.isLoading);
  const textByChunkId = useMemo(() => {
    const map = new Map<string, string>();
    chunks.forEach((chunk) => {
      if (!map.has(chunk.id)) {
        map.set(chunk.id, chunk.text);
      }
    });
    return map;
  }, [chunks]);

  const nothingToShow = !loading && chunks.length === 0 && quotes.length === 0;

  return (
    <>
      {loading && chunks.length === 0 && quotes.length === 0 && (
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyInspect.evidenceLoading', {
            defaultValue: 'Loading…',
          })}
        </span>
      )}

      {nothingToShow && (
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyInspect.evidenceNone', {
            defaultValue:
              'No source text was recorded for this row. It was extracted before the row kept its chunks, or the text has since been deleted.',
          })}
        </span>
      )}

      {quotes.map((entry, index) => {
        const text = entry.chunk_id
          ? textByChunkId.get(entry.chunk_id)
          : undefined;
        return (
          <article
            key={`${entry.chunk_id ?? 'quote'}-${index}`}
            className="rounded border border-border-button p-2 flex flex-col gap-1"
          >
            <span className="text-text-secondary break-all">
              {entry.chunk_id
                ? t('knowledgeCompilation.ontologyInspect.evidenceQuoteFrom', {
                    defaultValue: 'Quoted from {{chunk}}',
                    chunk: entry.chunk_id,
                  })
                : t('knowledgeCompilation.ontologyInspect.evidenceQuote', {
                    defaultValue: 'Quote',
                  })}
            </span>
            <blockquote className="border-l-2 border-border-button pl-2 whitespace-pre-wrap">
              <SearchHighlight text={entry.quote} query={entry.quote} />
            </blockquote>
            {text ? (
              <p className="mt-1 whitespace-pre-wrap text-text-secondary">
                <QuoteInContext text={text} quote={entry.quote} />
              </p>
            ) : (
              // The chunk could not be loaded — a deleted document, or an
              // id the current chunken keep no longer returns. The quote is
              // still what the model was shown, so it still stands alone.
              <span className="mt-1 text-text-secondary">
                {t('knowledgeCompilation.ontologyInspect.evidenceContextGone', {
                  defaultValue:
                    'The surrounding text is no longer available; the quote above is what the model was shown.',
                })}
              </span>
            )}
          </article>
        );
      })}

      {quotes.length === 0 &&
        chunks.map((chunk) => (
          <article
            key={chunk.id}
            className="rounded border border-border-button p-2 flex flex-col gap-1"
          >
            <span className="text-text-secondary break-all">{chunk.id}</span>
            <p className="whitespace-pre-wrap">{chunk.text}</p>
          </article>
        ))}
    </>
  );
}

/**
 * The chunk text around a quote, with the quote marked.
 *
 * The quote is located by text rather than by the stored span: the compiler
 * records byte offsets (what its gate can align on) while a JS string index is a
 * UTF-16 code unit, so slicing by the offset would land in the wrong place for
 * any CJK or accented text. When the quote cannot be found at all — the chunk
 * was re-parsed or edited since compilation — the quote alone is shown instead
 * of a span that would look authoritative and be wrong.
 */
export function QuoteInContext({
  text,
  quote,
}: {
  text: string;
  quote: string;
}) {
  const needle = quote.trim();
  const at = needle ? text.indexOf(needle) : -1;
  if (at < 0) {
    return <SearchHighlight text={quote} query={quote} />;
  }
  const from = Math.max(0, at - contextWindow);
  const to = Math.min(text.length, at + needle.length + contextWindow);
  return (
    <>
      {from > 0 ? '…' : null}
      <SearchHighlight text={text.slice(from, to)} query={needle} />
      {to < text.length ? '…' : null}
    </>
  );
}

export default OntologyEvidenceSheet;
