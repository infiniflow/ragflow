import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn } from 'react-hook-form';
import {
  AgentDialogueMode,
  BeginId,
  WebhookExecutionMode,
} from '../../constant';
import useGraphStore from '../../store';
import { BeginFormSchemaType } from '../begin-form/schema';

export function useShowWebhookResponseStatus(form: UseFormReturn<any>) {
  const getNode = useGraphStore((state) => state.getNode);

  const showWebhookResponseStatus = useMemo(() => {
    const formData: BeginFormSchemaType = getNode(BeginId)?.data.form;
    return (
      formData.mode === AgentDialogueMode.Webhook &&
      formData.execution_mode === WebhookExecutionMode.Streaming
    );
  }, []);

  useEffect(() => {
    if (showWebhookResponseStatus && isEmpty(form.getValues('status'))) {
      form.setValue('status', 200, { shouldValidate: true, shouldDirty: true });
    }
  }, []);

  return showWebhookResponseStatus;
}
