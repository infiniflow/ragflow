import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { Operator } from '../constant';
import { findInvalidNode } from './find-invalid-node';
import { shouldAutosaveCanvas } from './use-save-graph';

function buildNode(
  id: string,
  label: Operator,
  form: Record<string, any> = {},
): RAGFlowNodeType {
  return {
    id,
    data: { label, name: `${label} node`, form },
  } as unknown as RAGFlowNodeType;
}

describe('findInvalidNode', () => {
  it('flags an edited Agent node without a model', () => {
    const node = buildNode('agent-1', Operator.Agent, { llm_id: '' });

    expect(findInvalidNode([node], ['agent-1'])).toEqual({
      node,
      reason: 'missingModel',
    });
  });

  it('flags an edited Agent node whose model is missing from the form', () => {
    const node = buildNode('agent-1', Operator.Agent, {});

    expect(findInvalidNode([node], ['agent-1'])?.reason).toBe('missingModel');
  });

  it('allows an edited Agent node with a model', () => {
    const node = buildNode('agent-1', Operator.Agent, {
      llm_id: 'deepseek-chat@DeepSeek',
    });

    expect(findInvalidNode([node], ['agent-1'])).toBeUndefined();
  });

  it('ignores an Agent node the user did not edit', () => {
    const node = buildNode('agent-1', Operator.Agent, { llm_id: '' });

    expect(findInvalidNode([node], [])).toBeUndefined();
  });

  it('still reports an edited Parser node with an invalid form', () => {
    const node = buildNode('parser-1', Operator.Parser, {});

    expect(findInvalidNode([node], ['parser-1'])?.reason).toBe('invalidForm');
  });
});

describe('shouldAutosaveCanvas', () => {
  const ready = {
    chatDrawerVisible: false,
    agentId: 'agent-1',
    agentLoaded: true,
    nodeCount: 2,
    edgeCount: 1,
  };

  it('autosaves a loaded canvas with nodes', () => {
    expect(shouldAutosaveCanvas(ready)).toBe(true);
  });

  it('skips while the chat drawer is open', () => {
    expect(shouldAutosaveCanvas({ ...ready, chatDrawerVisible: true })).toBe(
      false,
    );
  });

  it('skips before the agent id is on the route', () => {
    expect(shouldAutosaveCanvas({ ...ready, agentId: undefined })).toBe(false);
  });

  it('skips before the agent detail has loaded', () => {
    expect(shouldAutosaveCanvas({ ...ready, agentLoaded: false })).toBe(false);
  });

  it('skips an empty nodes/edges store so autosave cannot wipe a pipeline', () => {
    expect(shouldAutosaveCanvas({ ...ready, nodeCount: 0, edgeCount: 0 })).toBe(
      false,
    );
  });

  it('still autosaves when there are nodes but no edges', () => {
    expect(shouldAutosaveCanvas({ ...ready, edgeCount: 0 })).toBe(true);
  });
});
