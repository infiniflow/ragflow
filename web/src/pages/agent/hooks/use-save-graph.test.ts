import { shouldAutosaveCanvas } from './use-save-graph';

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
