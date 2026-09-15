import { getWebhookTraceStatus } from '../status';

describe('getWebhookTraceStatus', () => {
  test('keeps loading and unfinished traces in the running state', () => {
    expect(getWebhookTraceStatus()).toEqual({ status: 'running' });
    expect(
      getWebhookTraceStatus({
        events: [],
        finished: false,
      }),
    ).toEqual({ status: 'running' });
  });

  test('reports success only for an explicit successful finished event', () => {
    expect(
      getWebhookTraceStatus({
        events: [{ event: 'finished', data: { success: true } }],
        finished: true,
      }),
    ).toEqual({ status: 'success' });

    expect(
      getWebhookTraceStatus({
        events: [],
        finished: true,
      }),
    ).toEqual({ status: 'fail', message: undefined });
  });

  test.each([
    ['error', { event: 'error', message: 'LLM failed' }, 'LLM failed'],
    [
      'cancelled',
      { event: 'cancelled', data: { message: 'Run cancelled' } },
      'Run cancelled',
    ],
    [
      'waiting for user',
      { event: 'waiting_for_user', data: { tips: 'Choose an option' } },
      'Choose an option',
    ],
    [
      'node failure',
      { event: 'node_finished', data: { error: 'Node failed' } },
      'Node failed',
    ],
  ])('reports %s terminal traces as failed', (_name, event, message) => {
    expect(
      getWebhookTraceStatus({
        events: [event, { event: 'finished', data: { success: false } }],
        finished: true,
      }),
    ).toEqual({ status: 'fail', message });
  });

  test('honors an unsuccessful finished event without a separate error', () => {
    expect(
      getWebhookTraceStatus({
        events: [{ event: 'finished', data: { success: false } }],
        finished: true,
      }),
    ).toEqual({ status: 'fail', message: undefined });
  });
});
