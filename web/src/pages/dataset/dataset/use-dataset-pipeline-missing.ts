import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';

// Flags a dataset whose configured pipeline can no longer parse: the version
// list failed to load (the pipeline was deleted or is no longer visible), or
// it loaded empty (no published versions left). Unlike the document-level
// hook there is no pinned version here — any published version keeps the
// dataset parseable. While the list is in flight (or the query is disabled
// because the dataset has not loaded yet) the verdict stays false so the
// upload dialog never warns spuriously.
export const useDatasetPipelineMissing = (pipelineId?: string) => {
  const {
    data: versions,
    loading,
    isError,
  } = useFetchCanvasVersionList(pipelineId);

  const missing =
    !!pipelineId &&
    (isError || (!loading && !!versions && versions.length === 0));

  return missing;
};
