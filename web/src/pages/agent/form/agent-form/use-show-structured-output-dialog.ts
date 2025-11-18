import { JSONSchema } from '@/components/jsonjoy-builder';
import { AgentStructuredOutputField } from '@/constants/agent';
import { useSetModalState } from '@/hooks/common-hooks';
import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';

export function useShowStructuredOutputDialog(form: UseFormReturn<any>) {
  const {
    visible: structuredOutputDialogVisible,
    showModal: showStructuredOutputDialog,
    hideModal: hideStructuredOutputDialog,
  } = useSetModalState();

  const initialStructuredOutput = form.getValues(AgentStructuredOutputField);

  const handleStructuredOutputDialogOk = useCallback(
    (values: JSONSchema) => {
      // Sync data to canvas
      form.setValue(AgentStructuredOutputField, values);
      hideStructuredOutputDialog();
    },
    [form, hideStructuredOutputDialog],
  );

  return {
    initialStructuredOutput,
    structuredOutputDialogVisible,
    showStructuredOutputDialog,
    hideStructuredOutputDialog,
    handleStructuredOutputDialogOk,
  };
}
