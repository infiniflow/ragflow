import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { LoadingButton } from '@/components/ui/loading-button';
import { useTagIsRenaming } from '@/hooks/knowledge-hooks';
import { IModalProps } from '@/interfaces/common';
import { TagRenameId } from '@/pages/add-knowledge/constant';
import { useTranslation } from 'react-i18next';
import { RenameForm } from './rename-form';

export function RenameDialog({
  hideModal,
  initialName,
}: IModalProps<any> & { initialName: string }) {
  const { t } = useTranslation();
  const loading = useTagIsRenaming();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('common.rename')}</DialogTitle>
        </DialogHeader>
        <RenameForm
          initialName={initialName}
          hideModal={hideModal}
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
