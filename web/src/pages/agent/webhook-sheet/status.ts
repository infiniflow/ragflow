import type { IWebhookTrace } from '@/interfaces/database/agent';

type WebhookTraceStatus = {
  status: 'running' | 'success' | 'fail';
  message?: string;
};

const FailedTerminalEvents = new Set([
  'error',
  'cancelled',
  'waiting_for_user',
]);

const getFailureMessage = (event: any) =>
  event?.data?.error ||
  event?.data?.message ||
  event?.data?.tips ||
  event?.message;

export const getWebhookTraceStatus = (
  trace?: Pick<IWebhookTrace, 'events' | 'finished'>,
): WebhookTraceStatus => {
  if (trace?.finished !== true) {
    return { status: 'running' };
  }

  const events = trace.events ?? [];
  const failureEvent = events.find(
    (event) => FailedTerminalEvents.has(event.event) || event.data?.error,
  );
  const finishedEvent = [...events]
    .reverse()
    .find((event) => event.event === 'finished');

  if (failureEvent || finishedEvent?.data?.success !== true) {
    return {
      status: 'fail',
      message: getFailureMessage(failureEvent),
    };
  }

  return { status: 'success' };
};
