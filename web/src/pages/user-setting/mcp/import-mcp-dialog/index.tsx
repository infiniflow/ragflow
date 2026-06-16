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
import { ImportMcpForm } from './import-mcp-form';

export function ImportMcpDialog({
  hideModal,
  onOk,
  loading,
}: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('mcp.import')}</DialogTitle>
        </DialogHeader>
        <ImportMcpForm hideModal={hideModal} onOk={onOk}></ImportMcpForm>
        <DialogFooter className="flex-row justify-end gap-2">
          <ButtonLoading type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
