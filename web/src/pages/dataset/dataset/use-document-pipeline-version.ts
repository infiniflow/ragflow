import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';
import { IDocumentInfo } from '@/interfaces/database/document';

// Resolves the published pipeline version a document parses with, so the parse
// column can show the version title/alias instead of the pipeline name. The
// version list query is keyed by canvas id, so rows sharing a pipeline share
// one fetch. A document without a pinned version tracks the latest published
// version (the list is newest-first); a pinned version that no longer exists
// resolves to undefined and the caller falls back to the pipeline name.
export const useDocumentPipelineVersion = (
  record: Pick<IDocumentInfo, 'pipeline_id' | 'pipeline_version_id'>,
) => {
  const { data: versions } = useFetchCanvasVersionList(
    record.pipeline_id || undefined,
  );

  return record.pipeline_version_id
    ? versions?.find((item) => item.id === record.pipeline_version_id)
    : versions?.[0];
};
