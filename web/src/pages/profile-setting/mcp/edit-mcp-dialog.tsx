import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IModalProps } from '@/interfaces/common';
import { useTranslation } from 'react-i18next';
import { EditMcpForm, FormId } from './edit-mcp-form';

export function EditMcpDialog({ hideModal, loading, onOk }: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit profile</DialogTitle>
        </DialogHeader>
        <EditMcpForm onOk={onOk} hideModal={hideModal}></EditMcpForm>
        <DialogFooter>
          <ButtonLoading type="submit" form={FormId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
