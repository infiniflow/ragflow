import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
import { useTagIsRenaming } from '@/hooks/use-knowledge-request';
import { IModalProps } from '@/interfaces/common';
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
          <ButtonLoading type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
