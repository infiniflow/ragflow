import { DocumentPipelineDialog } from '@/components/document-pipeline-dialog';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';

type ChangeParserDialogProps = {
  record: IDocumentInfo;
  onOk: (values: IChangeParserRequestBody) => Promise<void>;
  hideModal: () => void;
  loading: boolean;
};

// The document parser-change dialog edits the pipeline-shaped parser config.
export function ChangeParserDialog({
  record,
  onOk,
  hideModal,
  loading,
}: ChangeParserDialogProps) {
  return (
    <DocumentPipelineDialog
      parserId={record.chunk_method}
      pipelineId={record.pipeline_id}
      parserConfig={record.parser_config}
      onOk={onOk}
      hideModal={hideModal}
      loading={loading}
    ></DocumentPipelineDialog>
  );
}
