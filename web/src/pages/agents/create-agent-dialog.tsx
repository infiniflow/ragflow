import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { AgentCategory } from '@/constants/agent';
import { TagRenameId } from '@/constants/knowledge';
import { BrainCircuit, Route } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { CreateAgentForm, CreateAgentFormProps } from './create-agent-form';

type CreateAgentDialogProps = CreateAgentFormProps & {
  canvasCategory?: AgentCategory;
};

export function CreateAgentDialog({
  hideModal,
  onOk,
  loading,
  canvasCategory,
}: CreateAgentDialogProps) {
  const { t } = useTranslation();

  const dialogTitle =
    canvasCategory === AgentCategory.DataflowCanvas
      ? t('flow.createIngestionPipeline')
      : t('flow.createWorkflow');

  const DialogIcon =
    canvasCategory === AgentCategory.DataflowCanvas
      ? Route
      : canvasCategory === AgentCategory.AgentCanvas
        ? BrainCircuit
        : undefined;

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent data-testid="agent-create-modal" className="max-w-[800px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {DialogIcon && <DialogIcon className="size-5" />}
            {dialogTitle}
          </DialogTitle>
        </DialogHeader>
        <CreateAgentForm
          hideModal={hideModal}
          onOk={onOk}
        ></CreateAgentForm>
        <DialogFooter>
          <ButtonLoading
            data-testid="agent-save"
            type="submit"
            form={TagRenameId}
            loading={loading}
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
