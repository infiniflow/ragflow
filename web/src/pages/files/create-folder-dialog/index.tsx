import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { LoadingButton } from '@/components/ui/loading-button';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { useTranslation } from 'react-i18next';
import { CreateFolderForm } from './create-folder-form';

export function CreateFolderDialog({
  hideModal,
  onOk,
  loading,
}: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('fileManager.newFolder')}</DialogTitle>
        </DialogHeader>
        <CreateFolderForm hideModal={hideModal} onOk={onOk}></CreateFolderForm>
        <DialogFooter>
          <LoadingButton type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
