import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';
import { IDocumentInfo } from '@/interfaces/database/document';

// Resolves the published pipeline version a document parses with, so the parse
// column can show the version title/alias instead of the pipeline name. The
// version list query is keyed by canvas id, so rows sharing a pipeline share
// one fetch. A document without a pinned version tracks the latest published
// version (the list is newest-first).
//
// `missing` flags a pipeline binding that can no longer parse: the version
// list failed to load (the pipeline was deleted or is no longer visible), or
// it loaded without the version the document parses with (a deleted pinned
// version, or an unpinned binding whose pipeline has no published versions
// left). While the list is in flight the verdict stays false so the row never
// flashes "unavailable" on mount or refetch. Built-in parser documents are
// never missing.
export const useDocumentPipelineVersion = (
  record: Pick<IDocumentInfo, 'pipeline_id' | 'pipeline_version_id'>,
) => {
  const {
    data: versions,
    loading,
    isError,
  } = useFetchCanvasVersionList(record.pipeline_id || undefined);

  const version = record.pipeline_version_id
    ? versions?.find((item) => item.id === record.pipeline_version_id)
    : versions?.[0];

  const missing =
    !!record.pipeline_id && (isError || (!loading && !!versions && !version));

  return { version, missing };
};
