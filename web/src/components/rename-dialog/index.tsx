import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ButtonLoading } from '../ui/button';
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
          <ButtonLoading type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
