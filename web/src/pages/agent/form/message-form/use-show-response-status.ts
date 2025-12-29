import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn } from 'react-hook-form';
import {
  AgentDialogueMode,
  BeginId,
  WebhookExecutionMode,
} from '../../constant';
import useGraphStore from '../../store';

export function useShowWebhookResponseStatus(form: UseFormReturn<any>) {
  const getNode = useGraphStore((state) => state.getNode);

  const formData = getNode(BeginId)?.data.form;

  const isWebhookMode = formData?.mode === AgentDialogueMode.Webhook;

  const showWebhookResponseStatus = useMemo(() => {
    return (
      isWebhookMode &&
      formData?.execution_mode === WebhookExecutionMode.Streaming
    );
  }, [formData?.execution_mode, isWebhookMode]);

  useEffect(() => {
    if (showWebhookResponseStatus && isEmpty(form.getValues('status'))) {
      form.setValue('status', 200, { shouldValidate: true, shouldDirty: true });
    }
  }, [form, showWebhookResponseStatus]);

  return { showWebhookResponseStatus, isWebhookMode };
}
