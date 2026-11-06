import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { DocumentPipelineDialog } from '@/components/document-pipeline-dialog';
import { IDocumentInfo } from '@/interfaces/database/document';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useTranslation } from 'react-i18next';

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
      pipelineVersionId={record.pipeline_version_id}
      parserConfig={record.parser_config}
      onOk={onOk}
      hideModal={hideModal}
      loading={loading}
    ></DocumentPipelineDialog>
  );
}

// Shown after a parser-config save when the document already has chunks:
// confirm dropping them and re-parsing with the saved settings.
export function ReparseConfirmDialog({
  visible,
  onOk,
  onCancel,
}: {
  visible: boolean;
  onOk: () => void | Promise<void>;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <ConfirmDeleteDialog
      open={visible}
      title={t('knowledgeDetails.parseFile')}
      content={{ title: t('knowledgeDetails.parserConfigReparseTip') }}
      okButtonText={t('common.yes')}
      cancelButtonText={t('common.no')}
      onOk={onOk}
      onCancel={onCancel}
    ></ConfirmDeleteDialog>
  );
}
