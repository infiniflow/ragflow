import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import AgentSetting from './index';

const mockSaveAgent = jest.fn();
const MockAgentDetail = {
  title: 'Initial title',
  description: 'Initial description',
  avatar: null,
  permission: 'me',
};

// Radix RadioGroup measures items with ResizeObserver, which jsdom lacks.
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverMock;

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgent: () => ({
    data: MockAgentDetail,
  }),
  useSaveAgent: () => ({
    saveAgent: mockSaveAgent,
    loading: false,
  }),
}));

jest.mock('@/components/avatar-upload', () => {
  const { forwardRef } = jest.requireActual<typeof import('react')>('react');
  return { AvatarUpload: forwardRef(() => null) };
});

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('AgentSetting', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSaveAgent.mockResolvedValue({ code: 0 });
  });

  it('loads the current settings and saves edited values', async () => {
    render(
      <TooltipProvider>
        <AgentSetting />
      </TooltipProvider>,
    );

    const titleInput = await screen.findByDisplayValue('Initial title');
    fireEvent.change(titleInput, { target: { value: 'Updated title' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));

    await waitFor(() => {
      expect(mockSaveAgent).toHaveBeenCalledWith({
        title: 'Updated title',
        description: 'Initial description',
        avatar: null,
        permission: 'me',
      });
    });
  });

  it('does not submit an empty title', async () => {
    render(
      <TooltipProvider>
        <AgentSetting />
      </TooltipProvider>,
    );

    const titleInput = await screen.findByDisplayValue('Initial title');
    fireEvent.change(titleInput, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));

    await waitFor(() => {
      expect(titleInput).toHaveValue('');
    });
    expect(mockSaveAgent).not.toHaveBeenCalled();
  });
});
