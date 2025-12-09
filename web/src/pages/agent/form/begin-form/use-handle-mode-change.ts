import { useCallback } from 'react';
import { UseFormReturn } from 'react-hook-form';
import { AgentDialogueMode } from '../../constant';

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

export function useHandleModeChange(form: UseFormReturn<any>) {
  const handleModeChange = useCallback(
    (mode: AgentDialogueMode) => {
      if (mode === AgentDialogueMode.Webhook) {
        form.setValue('schema', schema, { shouldDirty: true });
      }
    },
    [form],
  );
  return { handleModeChange };
}
