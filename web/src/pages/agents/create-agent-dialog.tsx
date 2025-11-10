import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/pages/add-knowledge/constant';
import { useTranslation } from 'react-i18next';
import { CreateAgentForm, CreateAgentFormProps } from './create-agent-form';

type CreateAgentDialogProps = CreateAgentFormProps;

export function CreateAgentDialog({
  hideModal,
  onOk,
  loading,
  shouldChooseAgent,
}: CreateAgentDialogProps) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('flow.createGraph')}</DialogTitle>
        </DialogHeader>
        <CreateAgentForm
          hideModal={hideModal}
          onOk={onOk}
          shouldChooseAgent={shouldChooseAgent}
        ></CreateAgentForm>
        <DialogFooter>
          <ButtonLoading type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
