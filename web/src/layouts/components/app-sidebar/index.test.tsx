import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { AppSidebar } from './index';

const mockToggleDatasetSidebar = jest.fn();
const mockToggleAgentSidebar = jest.fn();
const mockToggleChatSidebar = jest.fn();
const mockToggleCompilationTemplateSidebar = jest.fn();
const mockTogglePrimarySidebar = jest.fn();
let mockPrimarySidebarCollapsed = false;
let mockDatasetSidebarCollapsed = false;
let mockAgentSidebarCollapsed = false;
let mockChatSidebarCollapsed = false;
let mockCompilationTemplateSidebarCollapsed = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/ee/layouts/components/header-enterprise', () => ({
  EnterpriseHeaderUpgradeBadge: () => null,
}));

jest.mock('@/hooks/use-dataset-sidebar-collapse', () => ({
  useDatasetSidebarCollapse: () => ({
    collapsed: mockDatasetSidebarCollapsed,
    toggleCollapsed: mockToggleDatasetSidebar,
  }),
}));

jest.mock('@/hooks/use-agent-sidebar-collapse', () => ({
  useAgentSidebarCollapse: () => ({
    collapsed: mockAgentSidebarCollapsed,
    toggleCollapsed: mockToggleAgentSidebar,
  }),
}));

jest.mock('@/hooks/use-chat-sidebar-collapse', () => ({
  useChatSidebarCollapse: () => ({
    collapsed: mockChatSidebarCollapsed,
    toggleCollapsed: mockToggleChatSidebar,
  }),
}));

jest.mock('@/hooks/use-compilation-template-sidebar-collapse', () => ({
  useCompilationTemplateSidebarCollapse: () => ({
    collapsed: mockCompilationTemplateSidebarCollapsed,
    toggleCollapsed: mockToggleCompilationTemplateSidebar,
  }),
}));

jest.mock('./use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    collapsed: mockPrimarySidebarCollapsed,
    toggleCollapsed: mockTogglePrimarySidebar,
  }),
}));

jest.mock('./nav-item', () => ({
  IntegrationsNavGroup: () => null,
  NavItemLink: () => null,
}));

jest.mock('./user-footer', () => ({
  UserFooter: () => null,
}));

function renderAppSidebar(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AppSidebar />
    </MemoryRouter>,
  );
}

describe('AppSidebar secondary sidebar expansion', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPrimarySidebarCollapsed = false;
    mockDatasetSidebarCollapsed = false;
    mockAgentSidebarCollapsed = false;
    mockChatSidebarCollapsed = false;
    mockCompilationTemplateSidebarCollapsed = false;
  });

  it('expands the agent secondary sidebar on agent detail routes', () => {
    mockAgentSidebarCollapsed = true;
    mockDatasetSidebarCollapsed = true;
    renderAppSidebar('/agent/agent-1/setting');

    const expandButton = screen.getByRole('button', {
      name: 'sidebar.expandSidebar',
    });

    expect(screen.getByRole('link', { name: 'RAGFlow' })).toBeInTheDocument();
    expect(expandButton.parentElement).toHaveClass('md:ml-auto');
    fireEvent.click(expandButton);

    expect(mockToggleAgentSidebar).toHaveBeenCalledTimes(1);
    expect(mockToggleDatasetSidebar).not.toHaveBeenCalled();
    expect(mockTogglePrimarySidebar).not.toHaveBeenCalled();
  });

  it('uses the header control only for the secondary sidebar when the primary sidebar is collapsed', () => {
    mockPrimarySidebarCollapsed = true;
    mockAgentSidebarCollapsed = true;
    const { container } = renderAppSidebar('/agent/agent-1/setting');
    const header = container.querySelector('header');

    expect(header).not.toBeNull();
    fireEvent.click(
      within(header as HTMLElement).getByRole('button', {
        name: 'sidebar.expandSidebar',
      }),
    );

    expect(mockToggleAgentSidebar).toHaveBeenCalledTimes(1);
    expect(mockTogglePrimarySidebar).not.toHaveBeenCalled();
  });

  it('expands the chat secondary sidebar on chat detail routes', () => {
    mockChatSidebarCollapsed = true;
    mockDatasetSidebarCollapsed = true;
    renderAppSidebar('/chat/chat-1');

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.expandSidebar' }),
    );

    expect(mockToggleChatSidebar).toHaveBeenCalledTimes(1);
    expect(mockToggleDatasetSidebar).not.toHaveBeenCalled();
  });

  it('does not show a secondary sidebar expander on built-in pipeline routes', () => {
    mockAgentSidebarCollapsed = true;
    renderAppSidebar('/pipeline/general');

    expect(
      screen.queryByRole('button', { name: 'sidebar.expandSidebar' }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'RAGFlow' })).toBeInTheDocument();
  });

  it('expands the compilation template secondary sidebar on its editor routes', () => {
    mockCompilationTemplateSidebarCollapsed = true;
    renderAppSidebar('/compilation-templates/edit-next/group-1');

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.expandSidebar' }),
    );

    expect(mockToggleCompilationTemplateSidebar).toHaveBeenCalledTimes(1);
    expect(mockToggleAgentSidebar).not.toHaveBeenCalled();
    expect(mockToggleDatasetSidebar).not.toHaveBeenCalled();
  });

  it('keeps the dataset expansion behavior on dataset routes', () => {
    mockDatasetSidebarCollapsed = true;
    mockAgentSidebarCollapsed = true;
    renderAppSidebar('/dataset/files/dataset-1');

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.expandSidebar' }),
    );

    expect(mockToggleDatasetSidebar).toHaveBeenCalledTimes(1);
    expect(mockToggleAgentSidebar).not.toHaveBeenCalled();
  });

  it('shows the logo instead of an expand button when the agent sidebar is visible', () => {
    renderAppSidebar('/agent/agent-1');

    expect(
      screen.queryByRole('button', { name: 'sidebar.expandSidebar' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'sidebar.collapseSidebar' }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'RAGFlow' })).toBeInTheDocument();
  });
});
