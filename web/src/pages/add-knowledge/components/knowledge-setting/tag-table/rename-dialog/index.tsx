import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IModalProps } from '@/interfaces/common';
import { TagRenameId } from '@/pages/add-knowledge/constant';
import { useTranslation } from 'react-i18next';
import { RenameForm } from './rename-form';

export function RenameDialog({
  hideModal,
  initialName,
}: IModalProps<any> & { initialName: string }) {
  const { t } = useTranslation();

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
          <Button type="submit" form={TagRenameId}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
