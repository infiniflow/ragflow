import { JSONSchema } from '@/components/jsonjoy-builder';
import { AgentStructuredOutputField } from '@/constants/agent';
import { useSetModalState } from '@/hooks/common-hooks';
import { useCallback } from 'react';
import { initialAgentValues } from '../../constant';
import useGraphStore from '../../store';

export function useShowStructuredOutputDialog(nodeId?: string) {
  const {
    visible: structuredOutputDialogVisible,
    showModal: showStructuredOutputDialog,
    hideModal: hideStructuredOutputDialog,
  } = useSetModalState();
  const { updateNodeForm } = useGraphStore((state) => state);

  const handleStructuredOutputDialogOk = useCallback(
    (values: JSONSchema) => {
      // Sync data to canvas
      if (nodeId) {
        updateNodeForm(nodeId, values, ['outputs', AgentStructuredOutputField]);
      }
      hideStructuredOutputDialog();
    },
    [hideStructuredOutputDialog, nodeId, updateNodeForm],
  );

  return {
    structuredOutputDialogVisible,
    showStructuredOutputDialog,
    hideStructuredOutputDialog,
    handleStructuredOutputDialogOk,
  };
}

export function useHandleShowStructuredOutput(nodeId?: string) {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const handleShowStructuredOutput = useCallback(
    (val: boolean) => {
      if (nodeId) {
        if (val) {
          updateNodeForm(nodeId, {}, ['outputs', AgentStructuredOutputField]);
        } else {
          updateNodeForm(nodeId, initialAgentValues.outputs, ['outputs']);
        }
      }
    },
    [nodeId, updateNodeForm],
  );

  return {
    handleShowStructuredOutput,
  };
}
