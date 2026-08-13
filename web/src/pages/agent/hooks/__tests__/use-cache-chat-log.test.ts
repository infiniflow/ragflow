jest.mock('eventsource-parser/stream', () => ({}));

import { getRunningNodeIds } from '../use-cache-chat-log';

function nodeEvent(type: string, componentId: string): Record<string, unknown> {
  return {
    event: type,
    message_id: 'm1',
    session_id: 's',
    created_at: 0,
    task_id: 't',
    data: { component_id: componentId },
  };
}

describe('getRunningNodeIds', () => {
  it('returns nodes whose last event is node_started', () => {
    const events = [
      nodeEvent('node_started', 'a'),
      nodeEvent('node_started', 'b'),
      nodeEvent('node_finished', 'a'),
    ] as any[];

    expect(getRunningNodeIds(events)).toEqual(['b']);
  });

  it('keeps the spinner on across loop iterations for a rerun node', () => {
    // Первый проход цикла: узел a запустился и завершился.
    // Второй проход: узел a снова запустился — его последнее событие
    // node_started, крутилка должна гореть.
    const events = [
      nodeEvent('node_started', 'a'),
      nodeEvent('node_finished', 'a'),
      nodeEvent('node_started', 'b'),
      nodeEvent('node_finished', 'b'),
      nodeEvent('node_started', 'a'),
    ] as any[];

    expect(getRunningNodeIds(events)).toEqual(['a']);
  });

  it('returns empty when all nodes are finished', () => {
    const events = [
      nodeEvent('node_started', 'a'),
      nodeEvent('node_finished', 'a'),
      nodeEvent('node_started', 'b'),
      nodeEvent('node_finished', 'b'),
    ] as any[];

    expect(getRunningNodeIds(events)).toEqual([]);
  });

  it('handles undefined/empty input', () => {
    expect(getRunningNodeIds(undefined)).toEqual([]);
    expect(getRunningNodeIds([])).toEqual([]);
  });
});
