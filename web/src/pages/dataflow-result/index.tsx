import { PipelinePreviewResult } from '@/ee/pages/dataflow-result/pipeline-preview-result';
import { useFetchNextChunkList } from '@/hooks/use-chunk-request';
import { useGetDocumentUrl } from '@/components/document-preview/hooks';
import { Routes } from '@/routes';
import {
  useFetchPipelineFileLogDetail,
  useGetPipelineResultSearchParams,
} from './hooks';
import { PipelineResultView } from './view';

function DatasetResult() {
  const { isReadOnly, knowledgeId } = useGetPipelineResultSearchParams();
  const {
    data: { documentInfo },
  } = useFetchNextChunkList();
  const { data } = useFetchPipelineFileLogDetail({});
  const fileUrl = useGetDocumentUrl();
  return (
    <PipelineResultView
      documentInfo={documentInfo}
      dataset={data}
      fileUrl={fileUrl}
      isReadOnly={isReadOnly}
      backTo={
        knowledgeId
          ? `${Routes.DatasetBase}${Routes.DataSetOverview}/${knowledgeId}`
          : '#'
      }
    />
  );
}

export default function DataflowResult() {
  return <PipelinePreviewResult fallback={<DatasetResult />} />;
}
