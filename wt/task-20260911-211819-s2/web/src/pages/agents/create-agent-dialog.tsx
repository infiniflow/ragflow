import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { AgentCategory } from '@/constants/agent';
import { IFlowTemplate } from '@/interfaces/database/agent';
import { BrainCircuit, Route } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateAgentForm, CreateAgentFormProps } from './create-agent-form';
import { countUnboundRetrieval } from './template-retrieval-binding';

type CreateAgentDialogProps = CreateAgentFormProps & {
  canvasCategory?: AgentCategory;
  template?: IFlowTemplate;
};

export function CreateAgentDialog({
  hideModal,
  onOk,
  loading,
  canvasCategory,
  template,
}: CreateAgentDialogProps) {
  const { t } = useTranslation();

  const retrievalBindings = useMemo(
    () => (template ? countUnboundRetrieval(template.dsl) : undefined),
    [template],
  );

  const dialogTitle =
    canvasCategory === AgentCategory.DataflowCanvas
      ? t('flow.createIngestionPipeline')
      : canvasCategory === AgentCategory.AgentCanvas
        ? t('flow.createWorkflow')
        : t('common.create');

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
          loading={loading}
          showTypeCards={!canvasCategory}
          retrievalBindings={retrievalBindings}
        ></CreateAgentForm>
      </DialogContent>
    </Dialog>
  );
}
