import { useToast } from '@/components/hooks/use-toast';
import message from '@/components/ui/message';
import { AgentCategory } from '@/constants/agent';
import { FileMimeType } from '@/constants/common';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCreateAgent } from '@/hooks/use-agent-request';
import * as bridge from '@/pages/agent/utils/dsl-bridge';
import { inferIsAgentFromImport } from '@/pages/agent/utils/dsl-bridge';
import isEmpty from 'lodash/isEmpty';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { FormSchemaType } from './upload-agent-dialog/upload-agent-form';

export const useHandleImportJsonFile = (expectedCategory: string) => {
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const { t } = useTranslation();
  const { toast } = useToast();
  const { loading, createAgent } = useCreateAgent();
  const { navigateToAgent } = useNavigatePage();

  const onFileUploadOk = useCallback(
    async ({ fileList, name }: FormSchemaType) => {
      if (fileList.length > 0) {
        const file = fileList[0];
        if (file.type !== FileMimeType.Json) {
          toast({ title: t('flow.jsonUploadTypeErrorMessage') });
          return;
        }

        const graphOrDslStr = await file.text();
        const errorMessage = t('flow.jsonUploadContentErrorMessage');
        try {
          const rawParsed = JSON.parse(graphOrDslStr);
          if (graphOrDslStr && !isEmpty(rawParsed)) {
            const isAgent = inferIsAgentFromImport(rawParsed);
            // The active tab decides what this import must produce; a file of
            // the other canvas kind is rejected instead of silently creating
            // a canvas in the wrong category.
            const inferredCategory = isAgent
              ? AgentCategory.AgentCanvas
              : AgentCategory.DataflowCanvas;
            if (inferredCategory !== expectedCategory) {
              message.error(t('flow.jsonUploadCategoryMismatch'));
              return;
            }
            const dsl = bridge.importDsl(rawParsed, isAgent);
            const ret = await createAgent({
              title: name,
              dsl,
              canvas_category: expectedCategory,
            });
            if (ret?.code === 0) {
              hideFileUploadModal();
              // Import is only offered on the Workflow/Pipeline tabs, so the
              // tab category is always a canvas category here.
              navigateToAgent(ret.data.id, expectedCategory as AgentCategory)();
            }
            return ret;
          } else {
            message.error(errorMessage);
          }
        } catch {
          message.error(errorMessage);
        }
      }
    },
    [
      expectedCategory,
      createAgent,
      hideFileUploadModal,
      navigateToAgent,
      t,
      toast,
    ],
  );

  return {
    fileUploadVisible,
    handleImportJson: showFileUploadModal,
    hideFileUploadModal,
    onFileUploadOk,
    loading,
  };
};
