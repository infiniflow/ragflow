import { Operator } from '@/constants/agent';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { findAgentNodeWithoutModel } from './agent-node-model';

function baseNode(
  id: string,
  label: Operator,
  form: Record<string, any> = {},
): RAGFlowNodeType {
  return {
    id,
    type: 'ragNode',
    position: { x: 0, y: 0 },
    data: { label, name: id, form },
  } as RAGFlowNodeType;
}

describe('findAgentNodeWithoutModel', () => {
  it('flags an Agent node whose model is empty', () => {
    const nodes = [
      baseNode('agent-1', Operator.Agent),
      baseNode('agent-2', Operator.Agent, { llm_id: '' }),
    ];
    expect(findAgentNodeWithoutModel(nodes)).toBe(nodes[0]);
  });

  it('accepts an Agent node with a model', () => {
    const nodes = [baseNode('agent-1', Operator.Agent, { llm_id: 'gpt-4o' })];
    expect(findAgentNodeWithoutModel(nodes)).toBeUndefined();
  });

  it('ignores nodes of other operators without a model', () => {
    const nodes = [
      baseNode('message-1', Operator.Message),
      baseNode('retrieval-1', Operator.Retrieval),
    ];
    expect(findAgentNodeWithoutModel(nodes)).toBeUndefined();
  });
});
