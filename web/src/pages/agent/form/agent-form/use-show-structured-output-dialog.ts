import { JSONSchema } from '@/components/jsonjoy-builder';
import { useSetModalState } from '@/hooks/common-hooks';
import { useCallback } from 'react';
import useGraphStore from '../../store';

export function useShowStructuredOutputDialog(nodeId?: string) {
  const {
    visible: structuredOutputDialogVisible,
    showModal: showStructuredOutputDialog,
    hideModal: hideStructuredOutputDialog,
  } = useSetModalState();
  const { updateNodeForm, getNode } = useGraphStore((state) => state);

  const initialStructuredOutput = getNode(nodeId)?.data.form.outputs.structured;

  const handleStructuredOutputDialogOk = useCallback(
    (values: JSONSchema) => {
      // Sync data to canvas
      if (nodeId) {
        updateNodeForm(nodeId, values, ['outputs', 'structured']);
      }
      hideStructuredOutputDialog();
    },
    [hideStructuredOutputDialog, nodeId, updateNodeForm],
  );

  return {
    initialStructuredOutput,
    structuredOutputDialogVisible,
    showStructuredOutputDialog,
    hideStructuredOutputDialog,
    handleStructuredOutputDialogOk,
  };
}
