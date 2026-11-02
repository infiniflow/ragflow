import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IFlowTemplate } from '@/interfaces/database/agent';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { FlowTypeConfig } from './constant';
import { CreateAgentForm, CreateAgentFormProps } from './create-agent-form';
import {
  collectUnboundRetrievalBindings,
  countUnboundModel,
} from './template-retrieval-binding';

type CreateAgentDialogProps = CreateAgentFormProps & {
  template?: IFlowTemplate;
};

export function CreateAgentDialog({
  hideModal,
  onOk,
  loading,
  selectedType,
  template,
}: CreateAgentDialogProps) {
  const { t } = useTranslation();

  const retrievalBindings = useMemo(
    () =>
      template ? collectUnboundRetrievalBindings(template.dsl) : undefined,
    [template],
  );
  const modelBindings = useMemo(
    () => (template ? countUnboundModel(template.dsl) : undefined),
    [template],
  );

  const dialogTitle = t(`flow.${FlowTypeConfig[selectedType].createTitleKey}`);
  const DialogIcon = FlowTypeConfig[selectedType].icon;

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent data-testid="agent-create-modal" className="max-w-[800px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <DialogIcon className="size-5" />
            {dialogTitle}
          </DialogTitle>
        </DialogHeader>
        <CreateAgentForm
          hideModal={hideModal}
          onOk={onOk}
          loading={loading}
          selectedType={selectedType}
          retrievalBindings={retrievalBindings}
          modelBindings={modelBindings}
        ></CreateAgentForm>
      </DialogContent>
    </Dialog>
  );
}
