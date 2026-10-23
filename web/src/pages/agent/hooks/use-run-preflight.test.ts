import message from '@/components/ui/message';
import { act, renderHook } from '@testing-library/react';
import useGraphStore from '../store';
import * as checklist from '../utils/canvas-checklist';
import {
  useSaveGraph,
  useSaveGraphBeforeOpeningDebugDrawer,
} from './use-save-graph';

const mockSetAgent = jest.fn();
const mockResetAgent = jest.fn();
const mockBuildDslData = jest.fn();

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgent: () => ({ data: { title: 'Draft' } }),
  useSetAgent: () => ({ setAgent: mockSetAgent, loading: false }),
  useResetAgent: () => ({ resetAgent: mockResetAgent }),
}));
jest.mock('./use-build-dsl', () => ({
  useBuildDslData: () => ({ buildDslData: mockBuildDslData }),
}));
jest.mock('react-router', () => ({ useParams: () => ({ id: 'agent-1' }) }));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));
jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

jest.mock('../utils/canvas-checklist', () => {
  const actual = jest.requireActual('../utils/canvas-checklist');
  return {
    ...actual,
    collectCanvasIssues: jest.fn(actual.collectCanvasIssues),
  };
});

const node = (label: string, form: Record<string, unknown>) =>
  ({ id: `${label}:1`, data: { label, name: label, form } }) as any;

describe('Run preflight', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSetAgent.mockResolvedValue({ code: 0 });
    mockResetAgent.mockResolvedValue({ code: 0 });
    useGraphStore.setState({ nodes: [], edges: [], editedNodeFormIds: [] });
  });

  it('blocks any run-blocking issue before saving, resetting or opening chat', async () => {
    jest.mocked(checklist.collectCanvasIssues).mockReturnValueOnce([
      {
        nodeId: 'future-node',
        nodeName: 'Future block',
        operatorLabel: 'Future',
        type: checklist.CanvasIssueType.MissingRequired,
        messageKey: 'future.emptyEntry',
        blocksRun: true,
      },
    ]);
    const show = jest.fn();
    const { result } = renderHook(() =>
      useSaveGraphBeforeOpeningDebugDrawer(show),
    );
    await act(async () => {
      await result.current.handleRun();
    });
    expect(message.error).toHaveBeenCalledWith(
      'Future block: future.emptyEntry',
    );
    expect(mockSetAgent).not.toHaveBeenCalled();
    expect(mockResetAgent).not.toHaveBeenCalled();
    expect(show).not.toHaveBeenCalled();
  });

  it.each([{}, { content: [] }, { content: ['hello', ''] }, { text: '' }])(
    'rejects incomplete Message %j without opening chat',
    async (form) => {
      useGraphStore.setState({ nodes: [node('Message', form)] });
      const show = jest.fn();
      const { result } = renderHook(() =>
        useSaveGraphBeforeOpeningDebugDrawer(show),
      );
      await act(async () => {
        await result.current.handleRun();
      });
      expect(message.error).toHaveBeenCalledWith('Message: flow.messageMsg');
      expect(mockResetAgent).not.toHaveBeenCalled();
      expect(show).not.toHaveBeenCalled();
    },
  );

  it('reads current nodes at click time and accepts valid text without gating advisory issues', async () => {
    useGraphStore.setState({ nodes: [node('Message', {})] });
    const show = jest.fn();
    const { result } = renderHook(() =>
      useSaveGraphBeforeOpeningDebugDrawer(show),
    );
    useGraphStore.setState({
      nodes: [node('Message', { text: 'hello' }), node('Parser', {})],
    });
    await act(async () => {
      await result.current.handleRun();
    });
    expect(message.error).not.toHaveBeenCalled();
    expect(mockSetAgent).toHaveBeenCalled();
    expect(mockResetAgent).toHaveBeenCalled();
    expect(show).toHaveBeenCalledTimes(1);
  });

  it.each([
    { content: ['hello'] },
    { content: 'hello' },
    { text: 'hello', content: [] },
  ])('opens chat for valid Message %j', async (form) => {
    useGraphStore.setState({ nodes: [node('Message', form)] });
    const show = jest.fn();
    const { result } = renderHook(() =>
      useSaveGraphBeforeOpeningDebugDrawer(show),
    );
    await act(async () => {
      await result.current.handleRun();
    });
    expect(message.error).not.toHaveBeenCalled();
    expect(show).toHaveBeenCalledTimes(1);
  });

  it('still saves drafts with incomplete Message content', async () => {
    useGraphStore.setState({ nodes: [node('Message', { content: [''] })] });
    const { result } = renderHook(() => useSaveGraph());
    await act(async () => {
      await result.current.saveGraph();
    });
    expect(mockSetAgent).toHaveBeenCalled();
    expect(message.error).not.toHaveBeenCalled();
  });

  it('validates explicit nextNodes before opening chat', async () => {
    useGraphStore.setState({
      nodes: [node('Message', { content: ['hello'] })],
    });
    const show = jest.fn();
    const { result } = renderHook(() =>
      useSaveGraphBeforeOpeningDebugDrawer(show),
    );
    await act(async () => {
      await result.current.handleRun([node('Message', { content: [''] })]);
    });
    expect(show).not.toHaveBeenCalled();
    expect(mockResetAgent).not.toHaveBeenCalled();
  });
});
