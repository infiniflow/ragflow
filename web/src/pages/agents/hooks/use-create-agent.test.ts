import { act, renderHook } from '@testing-library/react';
import { AgentCategory } from '@/constants/agent';
import { FlowType } from '../constant';
import { useCreateAgentOrPipeline } from './use-create-agent';

jest.mock('@/hooks/common-hooks', () => ({
  useSetModalState: jest.fn(),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: jest.fn(),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useSetAgent: jest.fn(),
}));

jest.mock('@/pages/agent/utils/dsl-bridge', () => ({
  initialEmptyDsl: jest.fn(() => ({})),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockCommonHooks = require('@/hooks/common-hooks');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockNavigateHooks = require('@/hooks/logic-hooks/navigate-hooks');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockAgentRequest = require('@/hooks/use-agent-request');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const mockDslBridge = require('@/pages/agent/utils/dsl-bridge');

const hideCreatingModal = jest.fn();
const setAgent = jest.fn();
const navigate = jest.fn();
const navigateToAgent = jest.fn(() => navigate);

const CreateScenarios = [
  {
    flowType: FlowType.Agent,
    canvasCategory: AgentCategory.AgentCanvas,
  },
  {
    flowType: FlowType.Flow,
    canvasCategory: AgentCategory.DataflowCanvas,
  },
];

beforeEach(() => {
  jest.clearAllMocks();
  mockCommonHooks.useSetModalState.mockReturnValue({
    visible: true,
    hideModal: hideCreatingModal,
    showModal: jest.fn(),
  });
  mockNavigateHooks.useNavigatePage.mockReturnValue({ navigateToAgent });
  mockAgentRequest.useSetAgent.mockReturnValue({
    loading: false,
    setAgent,
  });
});

describe('useCreateAgentOrPipeline', () => {
  it.each(CreateScenarios)(
    'navigates $flowType to its canvas after creation',
    async ({ flowType, canvasCategory }) => {
      setAgent.mockResolvedValue({
        code: 0,
        data: { id: 'canvas-id' },
      });

      const { result } = renderHook(() => useCreateAgentOrPipeline());
      await act(async () => {
        await result.current.handleCreateAgentOrPipeline({
          name: 'New agent',
          type: flowType,
        });
      });

      expect(setAgent).toHaveBeenCalledWith({
        title: 'New agent',
        dsl: {},
        canvas_category: canvasCategory,
      });
      expect(mockDslBridge.initialEmptyDsl).toHaveBeenCalledWith(
        flowType === FlowType.Agent,
      );
      expect(hideCreatingModal).toHaveBeenCalled();
      expect(navigateToAgent).toHaveBeenCalledWith('canvas-id', canvasCategory);
      expect(navigate).toHaveBeenCalledTimes(1);
    },
  );

  it('stays on the list when creation fails', async () => {
    setAgent.mockResolvedValue({ code: 1 });

    const { result } = renderHook(() => useCreateAgentOrPipeline());
    await act(async () => {
      await result.current.handleCreateAgentOrPipeline({
        name: 'New agent',
        type: FlowType.Agent,
      });
    });

    expect(hideCreatingModal).not.toHaveBeenCalled();
    expect(navigateToAgent).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
  });
});
