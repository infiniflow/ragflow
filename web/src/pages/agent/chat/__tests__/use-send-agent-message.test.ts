jest.mock('eventsource-parser/stream', () => ({}));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));
jest.mock('@/utils/api', () => ({ __esModule: true, default: {} }));
jest.mock('@/utils/authorization-util', () => ({
  __esModule: true,
  getAuthorization: () => 'token',
}));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

import { IEventList, MessageEventType } from '@/hooks/use-send-message';
import { findMessageSegmentsFromList } from '../use-send-agent-message';

function event(
  type: string,
  messageId: string,
  data: Record<string, unknown> = {},
) {
  return {
    event: type,
    message_id: messageId,
    session_id: 's',
    created_at: 0,
    task_id: 't',
    data,
  };
}

describe('findMessageSegmentsFromList', () => {
  it('splits multiple message areas by message_end', () => {
    const events = [
      event(MessageEventType.Message, 'm1', { content: 'Начинаю обработку' }),
      event(MessageEventType.MessageEnd, 'm1', { status: 200 }),
      event(MessageEventType.Message, 'm1', { content: 'Готово, вот результат' }),
      event(MessageEventType.MessageEnd, 'm1', { status: 200 }),
      event(MessageEventType.WorkflowFinished, 'm1', { outputs: {} }),
    ] as IEventList;

    const segments = findMessageSegmentsFromList(events);

    expect(segments).toHaveLength(2);
    expect(segments[0].content).toBe('Начинаю обработку');
    expect(segments[0].id).toBe('m1');
    expect(segments[1].content).toBe('Готово, вот результат');
    expect(segments[1].id).toBe('m1#1');
  });

  it('keeps a trailing area without message_end as the last segment', () => {
    const events = [
      event(MessageEventType.Message, 'm1', { content: 'Первый блок' }),
      event(MessageEventType.MessageEnd, 'm1', { status: 200 }),
      event(MessageEventType.Message, 'm1', { content: 'Хвостовой текст' }),
      event(MessageEventType.WorkflowFinished, 'm1', { outputs: {} }),
    ] as IEventList;

    const segments = findMessageSegmentsFromList(events);

    expect(segments).toHaveLength(2);
    expect(segments[1].content).toBe('Хвостовой текст');
  });

  it('produces a single segment when there is no message_end', () => {
    const events = [
      event(MessageEventType.Message, 'm1', { content: 'Один ответ' }),
      event(MessageEventType.WorkflowFinished, 'm1', { outputs: {} }),
    ] as IEventList;

    const segments = findMessageSegmentsFromList(events);

    expect(segments).toHaveLength(1);
    expect(segments[0].content).toBe('Один ответ');
    expect(segments[0].id).toBe('m1');
  });

  it('attaches downloads to their own area segment', () => {
    const events = [
      event(MessageEventType.Message, 'm1', { content: '' }),
      event(MessageEventType.MessageEnd, 'm1', {
        status: 200,
        downloads: [{ doc_id: 'd1', filename: 'protocol.txt', mime_type: 'text/plain' }],
      }),
      event(MessageEventType.Message, 'm1', { content: '' }),
      event(MessageEventType.MessageEnd, 'm1', {
        status: 200,
        downloads: [{ doc_id: 'd2', filename: 'transcript.txt', mime_type: 'text/plain' }],
      }),
      event(MessageEventType.WorkflowFinished, 'm1', { outputs: {} }),
    ] as IEventList;

    const segments = findMessageSegmentsFromList(events);

    expect(segments).toHaveLength(2);
    expect(segments[0].downloads).toHaveLength(1);
    expect(segments[0].downloads[0].doc_id).toBe('d1');
    expect(segments[1].downloads[0].doc_id).toBe('d2');
  });

  it('wraps think content in <think> tags per segment', () => {
    const events = [
      event(MessageEventType.Message, 'm1', { content: '', start_to_think: true }),
      event(MessageEventType.Message, 'm1', { content: 'размышляю' }),
      event(MessageEventType.Message, 'm1', { content: '', end_to_think: true }),
      event(MessageEventType.Message, 'm1', { content: 'Ответ' }),
      event(MessageEventType.MessageEnd, 'm1', { status: 200 }),
      event(MessageEventType.WorkflowFinished, 'm1', { outputs: {} }),
    ] as IEventList;

    const segments = findMessageSegmentsFromList(events);

    expect(segments).toHaveLength(1);
    expect(segments[0].content).toBe('<think>размышляю</think>Ответ');
  });
});
