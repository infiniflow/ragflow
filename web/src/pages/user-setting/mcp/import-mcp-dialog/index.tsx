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
import { ImportMcpForm } from './import-mcp-form';

export function ImportMcpDialog({
  hideModal,
  onOk,
  loading,
}: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('mcp.import')}</DialogTitle>
        </DialogHeader>
        <ImportMcpForm hideModal={hideModal} onOk={onOk}></ImportMcpForm>
        <DialogFooter>
          <LoadingButton type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
