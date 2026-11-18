import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes as RouterRoutes } from 'react-router';
import AgentLayout from './layout';

const mockSetAgentSidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockAgentSidebarCollapsed = false;

function MockAgentSidebar({
  title,
  onCollapse,
  onMenuSelect,
}: {
  title: string;
  onCollapse: () => void;
  onMenuSelect: () => void;
}) {
  return (
    <aside data-testid="agent-sidebar">
      <span>{title}</span>
      <button
        type="button"
        data-testid="agent-menu-item"
        onClick={onMenuSelect}
      >
        Menu
      </button>
      <button type="button" data-testid="agent-collapse" onClick={onCollapse}>
        Collapse
      </button>
    </aside>
  );
}

jest.mock('./hooks/use-fetch-data', () => ({
  useFetchDataOnMount: () => ({
    flowDetail: { title: 'Agent title' },
  }),
}));

jest.mock('@/hooks/use-agent-sidebar-collapse', () => ({
  useAgentSidebarCollapse: () => ({
    collapsed: mockAgentSidebarCollapsed,
    setCollapsed: mockSetAgentSidebarCollapsed,
  }),
}));

jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));

jest.mock('./sidebar', () => ({
  AgentSidebar: MockAgentSidebar,
}));

function renderAgentLayout() {
  return render(
    <MemoryRouter initialEntries={['/agent/agent-1/setting']}>
      <RouterRoutes>
        <Route path="/agent/:id" element={<AgentLayout />}>
          <Route
            path="setting"
            element={<div data-testid="agent-outlet">Setting content</div>}
          />
        </Route>
      </RouterRoutes>
    </MemoryRouter>,
  );
}

describe('AgentLayout', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAgentSidebarCollapsed = false;
  });

  it('renders the secondary sidebar and matched child page', () => {
    renderAgentLayout();

    expect(screen.getByTestId('agent-sidebar')).toHaveTextContent(
      'Agent title',
    );
    expect(screen.getByTestId('agent-outlet')).toHaveTextContent(
      'Setting content',
    );
  });

  it('expands the secondary sidebar on entry', () => {
    mockAgentSidebarCollapsed = true;
    renderAgentLayout();

    expect(mockSetAgentSidebarCollapsed).toHaveBeenCalledWith(false);
  });

  it('hides the secondary sidebar when collapsed', () => {
    mockAgentSidebarCollapsed = true;
    renderAgentLayout();

    expect(screen.queryByTestId('agent-sidebar')).not.toBeInTheDocument();
    expect(screen.getByTestId('agent-outlet')).toBeInTheDocument();
  });

  it('collapses only the primary sidebar after menu selection', () => {
    renderAgentLayout();
    mockSetAgentSidebarCollapsed.mockClear();

    fireEvent.click(screen.getByTestId('agent-menu-item'));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetAgentSidebarCollapsed).not.toHaveBeenCalled();
  });

  it('collapses both sidebars from the secondary sidebar control', () => {
    renderAgentLayout();

    fireEvent.click(screen.getByTestId('agent-collapse'));

    expect(mockSetAgentSidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });
});
