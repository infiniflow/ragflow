import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { LoadingButton } from '@/components/ui/loading-button';
import { useDeleteLangfuseConfig } from '@/hooks/use-user-setting-request';
import { IModalProps } from '@/interfaces/common';
import { ExternalLink, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  FormId,
  LangfuseConfigurationForm,
} from './langfuse-configuration-form';

export function LangfuseConfigurationDialog({
  hideModal,
  loading,
  onOk,
}: IModalProps<any>) {
  const { t } = useTranslation();
  const { deleteLangfuseConfig } = useDeleteLangfuseConfig();

  const handleDelete = useCallback(async () => {
    const ret = await deleteLangfuseConfig();
    if (ret === 0) {
      hideModal?.();
    }
  }, [deleteLangfuseConfig, hideModal]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogTrigger asChild>
        <Button variant="outline"></Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('setting.configuration')} Langfuse</DialogTitle>
        </DialogHeader>
        <LangfuseConfigurationForm onOk={onOk}></LangfuseConfigurationForm>
        <DialogFooter className="!justify-between">
          <a
            href="https://langfuse.com/docs"
            className="flex items-center gap-2 underline text-blue-600 hover:text-blue-800 visited:text-purple-600"
            target="_blank"
            rel="noreferrer"
          >
            {t('setting.viewLangfuseSDocumentation')}
            <ExternalLink className="size-4" />
          </a>
          <div className="flex items-center gap-4">
            <ConfirmDeleteDialog onOk={handleDelete}>
              <Button variant={'outline'}>
                <Trash2 className="text-red-500" /> {t('common.delete')}
              </Button>
            </ConfirmDeleteDialog>

            <LoadingButton type="submit" form={FormId} loading={loading}>
              {t('common.save')}
            </LoadingButton>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
