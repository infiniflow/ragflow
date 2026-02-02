import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
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
      <DialogContent data-testid="agent-create-modal">
        <DialogHeader>
          <DialogTitle>{t('flow.createGraph')}</DialogTitle>
        </DialogHeader>
        <CreateAgentForm
          hideModal={hideModal}
          onOk={onOk}
          shouldChooseAgent={shouldChooseAgent}
        ></CreateAgentForm>
        <DialogFooter>
          <ButtonLoading
            type="submit"
            form={TagRenameId}
            loading={loading}
            data-testid="agent-save"
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
