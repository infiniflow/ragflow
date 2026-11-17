import { fireEvent, render, screen } from '@testing-library/react';
import { AgentCategory } from '@/constants/agent';
import { AgentCard } from './agent-card';

const mockSetPrimarySidebarCollapsed = jest.fn();
const mockNavigateToAgent = jest.fn();

function MockHomeCard({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" data-testid="agent-card" onClick={onClick}>
      Agent card
    </button>
  );
}

function MockAgentDropdown({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('@/components/home-card', () => ({
  HomeCard: MockHomeCard,
}));
jest.mock('@/components/more-button', () => ({
  MoreButton: () => null,
}));
jest.mock('@/components/shared-badge', () => ({
  SharedBadge: () => null,
}));
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgent: (id: string, category: string) => () =>
      mockNavigateToAgent(id, category),
  }),
}));
jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));
jest.mock('./agent-dropdown', () => ({
  AgentDropdown: MockAgentDropdown,
}));

describe('AgentCard navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('collapses the primary sidebar before navigating to agent detail', () => {
    render(
      <AgentCard
        data={
          {
            id: 'pipeline-1',
            title: 'Pipeline',
            canvas_category: AgentCategory.DataflowCanvas,
          } as never
        }
        showAgentRenameModal={jest.fn()}
        showPrivilegeModal={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId('agent-card'));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      'pipeline-1',
      AgentCategory.DataflowCanvas,
    );
    expect(
      mockSetPrimarySidebarCollapsed.mock.invocationCallOrder[0],
    ).toBeLessThan(mockNavigateToAgent.mock.invocationCallOrder[0]);
  });
});
