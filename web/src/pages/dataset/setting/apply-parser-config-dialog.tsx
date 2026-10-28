import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { ButtonLoading, buttonVariants } from '@/components/ui/button';
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

  const handleOpenChange = (nextOpen: boolean) => {
    if (isPending) return;
    reset();
    onOpenChange(nextOpen);
  };

  const handleApply = async () => {
    try {
      await mutateAsync();
      message.success(t('knowledgeConfiguration.parserSettingsApplied'));
      onOpenChange(false);
    } catch {
      // Keep the saved-settings dialog open so Yes can retry the operation.
    }
  };

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent className="bg-bg-base border-border-default">
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t('knowledgeConfiguration.applyParserSettingsTitle')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('knowledgeConfiguration.parserSettingsChangeWarning')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <p role="alert" className="text-state-error text-sm">
            {t('knowledgeConfiguration.parserSettingsApplyFailed')}{' '}
            {error.message}
          </p>
        )}
        <AlertDialogFooter>
          <ButtonLoading
            type="button"
            variant="outline"
            loading={isPending}
            disabled={isPending}
            onClick={handleApply}
          >
            {t('common.yes')}
          </ButtonLoading>
          <AlertDialogCancel
            className={buttonVariants({ variant: 'default' })}
            disabled={isPending}
          >
            {t('common.no')}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
