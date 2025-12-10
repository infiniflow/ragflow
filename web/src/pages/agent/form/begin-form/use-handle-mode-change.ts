import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';
import {
  AgentDialogueMode,
  RateLimitPerList,
  WebhookExecutionMode,
  WebhookMaxBodySize,
  WebhookSecurityAuthType,
} from '../../constant';

// const WebhookSchema = {
//   query: {
//     type: 'object',
//     required: [],
//     properties: {
//       //   debug: { type: 'boolean' },
//       //   event: { type: 'string' },
//     },
//   },

//   headers: {
//     type: 'object',
//     required: [],
//     properties: {
//       //   'X-Trace-ID': { type: 'string' },
//     },
//   },

//   body: {
//     type: 'object',
//     required: [],
//     properties: {
//       id: { type: 'string' },
//       payload: { type: 'object' },
//     },
//   },
// };

const schema = {
  properties: {
    query: {
      type: 'object',
      description: '',
    },
    headers: {
      type: 'object',
      description: '',
    },
    body: {
      type: 'object',
      description: '',
    },
  },
};

const initialFormValuesMap = {
  schema: schema,
  'security.auth_type': WebhookSecurityAuthType.Basic,
  'security.rate_limit.per': RateLimitPerList[0],
  'security.max_body_size': WebhookMaxBodySize[0],
  execution_mode: WebhookExecutionMode.Immediately,
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
