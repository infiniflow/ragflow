import { JSONSchema } from '@/components/jsonjoy-builder';
import { useSetModalState } from '@/hooks/common-hooks';
import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';

export function useShowSchemaDialog(form: UseFormReturn<any>) {
  const {
    visible: schemaDialogVisible,
    showModal: showSchemaDialog,
    hideModal: hideSchemaDialog,
  } = useSetModalState();

  const handleSchemaDialogOk = useCallback(
    (values: JSONSchema) => {
      // Sync data to canvas
      form.setValue('schema', values);
      hideSchemaDialog();
    },
    [form, hideSchemaDialog],
  );

  return {
    schemaDialogVisible,
    showSchemaDialog,
    hideSchemaDialog,
    handleSchemaDialogOk,
  };
}
