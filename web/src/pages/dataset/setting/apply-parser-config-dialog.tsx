import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import message from '@/components/ui/message';
import { useApplyDatasetParserConfig } from '@/hooks/use-apply-dataset-parser-config';
import { useTranslation } from 'react-i18next';

type ApplyParserConfigDialogProps = {
  datasetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ApplyParserConfigDialog({
  datasetId,
  open,
  onOpenChange,
}: ApplyParserConfigDialogProps) {
  const { t } = useTranslation();
  const { mutateAsync, isPending, error, reset } =
    useApplyDatasetParserConfig(datasetId);

  const handleCancel = () => {
    if (isPending) {
      return;
    }
    reset();
    onOpenChange(false);
  };

  const handleApply = async () => {
    if (isPending) {
      return;
    }
    try {
      await mutateAsync();
      message.success(t('knowledgeConfiguration.parserSettingsApplied'));
      onOpenChange(false);
    } catch {
      // Keep the dialog open so Yes can retry the operation.
    }
  };

  return (
    <ConfirmDeleteDialog
      open={open}
      title={t('knowledgeConfiguration.applyParserSettingsTitle')}
      content={{
        title: t('knowledgeConfiguration.parserSettingsChangeWarning'),
        node: error ? (
          <p role="alert" className="text-state-error text-sm">
            {t('knowledgeConfiguration.parserSettingsApplyFailed')}{' '}
            {error.message}
          </p>
        ) : null,
      }}
      okButtonText={t('common.yes')}
      cancelButtonText={t('common.no')}
      onOk={handleApply}
      onCancel={handleCancel}
    ></ConfirmDeleteDialog>
  );
}
