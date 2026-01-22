import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { useTranslation } from 'react-i18next';
import { UploadAgentForm } from './upload-agent-form';

export function UploadAgentDialog({
  hideModal,
  onOk,
  loading,
}: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent data-testid="agent-import-modal">
        <DialogHeader>
          <DialogTitle>{t('fileManager.uploadFile')}</DialogTitle>
        </DialogHeader>
        <UploadAgentForm hideModal={hideModal} onOk={onOk}></UploadAgentForm>
        <DialogFooter>
          <ButtonLoading
            type="submit"
            form={TagRenameId}
            loading={loading}
            data-testid="app-save"
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
