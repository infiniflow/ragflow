import { Operator } from '../../constant';
import { ScoredSwitchEscalateHandleId } from '../../scored-switch';
import { buildDslComponentsByGraph } from '../../utils';

const nodeId = 'ScoredSwitch:TestId';

function scoredSwitchNode(escalateEnabled: boolean) {
  return {
    id: nodeId,
    type: 'scoredSwitchNode',
    position: { x: 0, y: 0 },
    data: {
      label: Operator.ScoredSwitch,
      name: 'Scored Switch',
      form: {
        llm_id: 'model@provider',
        query: 'sys.query',
        escalate_enabled: escalateEnabled,
        margin_threshold: 0.15,
        min_top_score: 0,
        request_format: 'content_parts',
        items: [
          { name: 'billing', description: 'Payments', uuid: 'opt-a' },
          { name: 'technical', description: 'Bugs', uuid: 'opt-b' },
        ],
      },
    },
  } as any;
}

describe('buildDslComponentsByGraph ScoredSwitch params', () => {
  it('writes option targets and the escalate edge when escalation is on', () => {
    const components = buildDslComponentsByGraph(
      [scoredSwitchNode(true)],
      [
        { source: nodeId, target: 'Message:a', sourceHandle: 'opt-a' },
        {
          source: nodeId,
          target: 'LLM:b',
          sourceHandle: ScoredSwitchEscalateHandleId,
        },
      ] as any,
      {},
    ) as any;
    const params = components[nodeId].obj.params;
    expect(params.items).toBeUndefined();
    expect(params.escalate_enabled).toBe(true);
    expect(params.options).toEqual([
      {
        name: 'billing',
        description: 'Payments',
        uuid: 'opt-a',
        to: ['Message:a'],
      },
      {
        name: 'technical',
        description: 'Bugs',
        uuid: 'opt-b',
        to: [],
      },
    ]);
    expect(params.escalate_to).toEqual(['LLM:b']);
    expect(components[nodeId].obj.component_name).toBe('ScoredSwitch');
  });

  it('drops the escalate edge when escalation is off', () => {
    const components = buildDslComponentsByGraph(
      [scoredSwitchNode(false)],
      [
        {
          source: nodeId,
          target: 'LLM:b',
          sourceHandle: ScoredSwitchEscalateHandleId,
        },
      ] as any,
      {},
    ) as any;
    const params = components[nodeId].obj.params;
    expect(params.escalate_enabled).toBe(false);
    expect(params.escalate_to).toEqual([]);
  });
});
