import { ChunkMethodDialog } from '@/components/chunk-method-dialog';
import { DocumentPipelineDialog } from '@/components/document-pipeline-dialog';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { BackendVariant } from '@/utils/backend-variant';
import { getExtension } from '@/utils/document-util';

type ChangeParserDialogProps = {
  record: IDocumentInfo;
  visible: boolean;
  onOk: (values: IChangeParserRequestBody) => Promise<void>;
  hideModal: () => void;
  loading: boolean;
};

// Dispatch point for the document parser-change dialog: the Go backend edits
// the pipeline-shaped parser config, while the Python backend uses the legacy
// chunk-method dialog.
export function ChangeParserDialog({
  record,
  visible,
  onOk,
  hideModal,
  loading,
}: ChangeParserDialogProps) {
  return (
    <BackendVariant
      go={
        <DocumentPipelineDialog
          parserId={record.chunk_method}
          pipelineId={record.pipeline_id}
          parserConfig={record.parser_config}
          onOk={onOk}
          hideModal={hideModal}
          loading={loading}
        ></DocumentPipelineDialog>
      }
      python={
        <ChunkMethodDialog
          documentId={record.id}
          parserId={record.chunk_method}
          pipelineId={record.pipeline_id}
          parserConfig={record.parser_config}
          documentExtension={getExtension(record.name)}
          onOk={onOk}
          visible={visible}
          hideModal={hideModal}
          loading={loading}
        ></ChunkMethodDialog>
      }
    />
  );
}
