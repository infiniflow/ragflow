import { useToast } from '@/components/hooks/use-toast';
import message from '@/components/ui/message';
import { AgentCategory, DataflowOperator } from '@/constants/agent';
import { FileMimeType } from '@/constants/common';
import { useSetModalState } from '@/hooks/common-hooks';
import { EmptyDsl, useSetAgent } from '@/hooks/use-agent-request';
import { Node } from '@xyflow/react';
import isEmpty from 'lodash/isEmpty';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { DataflowEmptyDsl } from './hooks/use-create-agent';
import { FormSchemaType } from './upload-agent-dialog/upload-agent-form';

function hasNode(nodes: Node[], operator: DataflowOperator) {
  return nodes.some((x) => x.data.label === operator);
}

export const useHandleImportJsonFile = () => {
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const { t } = useTranslation();
  const { toast } = useToast();
  const { loading, setAgent } = useSetAgent();

  const onFileUploadOk = useCallback(
    async ({ fileList, name }: FormSchemaType) => {
      if (fileList.length > 0) {
        const file = fileList[0];
        if (file.type !== FileMimeType.Json) {
          toast({ title: t('flow.jsonUploadTypeErrorMessage') });
          return;
        }

        const graphStr = await file.text();
        const errorMessage = t('flow.jsonUploadContentErrorMessage');
        try {
          const graph = JSON.parse(graphStr);
          if (graphStr && !isEmpty(graph) && Array.isArray(graph?.nodes)) {
            const nodes: Node[] = graph.nodes;

            let isAgent = true;

            if (
              hasNode(nodes, DataflowOperator.Begin) &&
              hasNode(nodes, DataflowOperator.Parser)
            ) {
              isAgent = false;
            }

            const dsl = isAgent
              ? { ...EmptyDsl, graph }
              : { ...DataflowEmptyDsl, graph };

            setAgent({
              title: name,
              dsl,
              canvas_category: isAgent
                ? AgentCategory.AgentCanvas
                : AgentCategory.DataflowCanvas,
            });
            hideFileUploadModal();
          } else {
            message.error(errorMessage);
          }
        } catch (error) {
          message.error(errorMessage);
        }
      }
    },
    [hideFileUploadModal, setAgent, t, toast],
  );

  return {
    fileUploadVisible,
    handleImportJson: showFileUploadModal,
    hideFileUploadModal,
    onFileUploadOk,
    loading,
  };
};
