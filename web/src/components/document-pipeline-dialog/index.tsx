import { BuiltinPipelineItem } from '@/components/builtin-pipeline-form-field';
import { DataFlowSelect } from '@/components/data-pipeline-select';
import { ParseTypeItem } from '@/components/parse-type-form-field';
import PipelineOperatorTabs from '@/components/pipeline-operator-tabs';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { ParseType } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { IChangeParserRequestBody } from '@/interfaces/request/document';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IDocumentPipelineDialogProps,
  useDocumentPipelineForm,
} from './use-document-pipeline-form';

const FormId = 'DocumentPipelineDialogForm';

interface IProps
  extends IModalProps<IChangeParserRequestBody>, IDocumentPipelineDialogProps {
  loading: boolean;
}

export function DocumentPipelineDialog({
  hideModal,
  onOk,
  parserId,
  pipelineId,
  parserConfig,
  loading,
}: IProps) {
  const { t } = useTranslation();

  const {
    form,
    parseType,
    operatorNodes,
    operatorNodesLoading,
    activeTab,
    setActiveTab,
    handleOperatorValuesChange,
    showOperatorTabs,
    buildSubmitData,
  } = useDocumentPipelineForm({ parserId, pipelineId, parserConfig });

  const onSubmit = useCallback(
    async (data: Parameters<typeof buildSubmitData>[0]) => {
      const ret = await onOk?.(buildSubmitData(data));
      if (ret) {
        hideModal?.();
      }
    },
    [buildSubmitData, hideModal, onOk],
  );

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-w-[50vw] text-text-primary">
        <DialogHeader>
          <DialogTitle>{t('knowledgeDetails.chunkMethod')}</DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6 max-h-[70vh] overflow-auto -mx-6 px-10 py-5"
            id={FormId}
          >
            <ParseTypeItem />
            {parseType === ParseType.BuiltIn && <BuiltinPipelineItem />}
            {parseType === ParseType.Pipeline && (
              <DataFlowSelect
                isMult={false}
                showToDataPipeline={true}
                formFieldName="pipeline_id"
              />
            )}
            {showOperatorTabs && (
              <PipelineOperatorTabs
                nodes={operatorNodes}
                value={activeTab}
                onValueChange={setActiveTab}
                onOperatorValuesChange={handleOperatorValuesChange}
              />
            )}
          </form>
        </Form>
        <DialogFooter>
          <ButtonLoading
            type="submit"
            form={FormId}
            loading={
              loading || (operatorNodesLoading && operatorNodes.length === 0)
            }
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
