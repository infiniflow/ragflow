import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { LoadingButton } from '@/components/ui/loading-button';
import { IModalProps } from '@/interfaces/common';
import { TagRenameId } from '@/pages/add-knowledge/constant';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { RenameForm } from './rename-form';

export function RenameDialog({
  hideModal,
  initialName,
  onOk,
  loading,
  title,
}: IModalProps<any> & { initialName?: string; title?: ReactNode }) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{title || t('common.rename')}</DialogTitle>
        </DialogHeader>
        <RenameForm
          initialName={initialName}
          hideModal={hideModal}
          onOk={onOk}
        ></RenameForm>
        <DialogFooter>
          <LoadingButton type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
