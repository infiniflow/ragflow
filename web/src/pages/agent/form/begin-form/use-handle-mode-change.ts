import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';
import {
  AgentDialogueMode,
  WebhookContentType,
  WebhookExecutionMode,
  WebhookMaxBodySize,
  WebhookMethod,
  WebhookRateLimitPer,
  WebhookSecurityAuthType,
} from '../../constant';

const initialFormValuesMap = {
  methods: [WebhookMethod.Get],
  schema: {},
  'security.auth_type': WebhookSecurityAuthType.Basic,
  'security.rate_limit.per': WebhookRateLimitPer.Second,
  'security.rate_limit.limit': 10,
  'security.max_body_size': WebhookMaxBodySize[0],
  'response.status': 200,
  execution_mode: WebhookExecutionMode.Immediately,
  content_types: WebhookContentType.ApplicationJson,
};

export function useHandleModeChange(form: UseFormReturn<any>) {
  const handleModeChange = useCallback(
    (mode: AgentDialogueMode) => {
      if (mode === AgentDialogueMode.Webhook) {
        Object.entries(initialFormValuesMap).forEach(([key, value]) => {
          form.setValue(key, value, { shouldDirty: true });
        });
      }
    },
    [form],
  );
  return { handleModeChange };
}
