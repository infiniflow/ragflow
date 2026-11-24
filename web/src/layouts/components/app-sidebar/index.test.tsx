import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { AppSidebar } from './index';

const mockToggleDatasetSidebar = jest.fn();
const mockToggleAgentSidebar = jest.fn();
const mockToggleChatSidebar = jest.fn();
const mockToggleSearchSidebar = jest.fn();
const mockToggleCompilationTemplateSidebar = jest.fn();
const mockTogglePrimarySidebar = jest.fn();
let mockPrimarySidebarCollapsed = false;
let mockDatasetSidebarCollapsed = false;
let mockAgentSidebarCollapsed = false;
let mockChatSidebarCollapsed = false;
let mockSearchSidebarCollapsed = false;
let mockCompilationTemplateSidebarCollapsed = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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

jest.mock('@/hooks/use-search-sidebar-collapse', () => ({
  useSearchSidebarCollapse: () => ({
    collapsed: mockSearchSidebarCollapsed,
    toggleCollapsed: mockToggleSearchSidebar,
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

jest.mock('@/ee/hooks/use-visible-nav-items', () => ({
  useVisibleNavItems: (items: unknown[]) => items,
}));

jest.mock('@/ee/layouts/components/app-sidebar/organization-nav-group', () => ({
  OrganizationNavGroup: () => null,
}));

jest.mock('./nav-item', () => ({
  IntegrationsNavGroup: () => <span data-testid="integrations-nav" />,
  NavItemLink: ({ item }: { item: { key: string } }) => (
    <span data-testid={`nav-item-${item.key}`} />
  ),
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

describe('AppSidebar navigation order', () => {
  it('places Team immediately after Integrations', () => {
    renderAppSidebar('/team');

    const integrations = screen.getByTestId('integrations-nav');
    const team = screen.getByTestId('nav-item-team');

    expect(integrations.parentElement?.nextElementSibling).toContainElement(
      team,
    );
  });
});

describe('AppSidebar secondary sidebar expansion', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPrimarySidebarCollapsed = false;
    mockDatasetSidebarCollapsed = false;
    mockAgentSidebarCollapsed = false;
    mockChatSidebarCollapsed = false;
    mockSearchSidebarCollapsed = false;
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

  it('expands the search secondary sidebar on search detail routes', () => {
    mockSearchSidebarCollapsed = true;
    renderAppSidebar('/search/search-1');

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.expandSidebar' }),
    );

    expect(mockToggleSearchSidebar).toHaveBeenCalledTimes(1);
    expect(mockToggleChatSidebar).not.toHaveBeenCalled();
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

describe('AppSidebar primary sidebar border', () => {
  beforeEach(() => {
    mockPrimarySidebarCollapsed = true;
    mockDatasetSidebarCollapsed = false;
    mockAgentSidebarCollapsed = false;
    mockChatSidebarCollapsed = false;
    mockSearchSidebarCollapsed = false;
    mockCompilationTemplateSidebarCollapsed = false;
  });

  it('is hidden on list pages without a secondary sidebar', () => {
    renderAppSidebar('/agents');

    expect(screen.getByRole('complementary')).not.toHaveClass('border-r');
  });

  it('is shown when the agent secondary sidebar is visible', () => {
    renderAppSidebar('/agent/agent-1');

    expect(screen.getByRole('complementary')).toHaveClass(
      'border-r',
      'border-border-button',
    );
  });

  it('is shown when the agent secondary sidebar is collapsed', () => {
    mockAgentSidebarCollapsed = true;
    renderAppSidebar('/agent/agent-1');

    expect(screen.getByRole('complementary')).toHaveClass('border-r');
  });

  it('is shown on memory detail routes with a secondary sidebar', () => {
    renderAppSidebar('/memory/memory-message/memory-1');

    expect(screen.getByRole('complementary')).toHaveClass('border-r');
  });

  it('is hidden when the primary sidebar is expanded', () => {
    mockPrimarySidebarCollapsed = false;
    renderAppSidebar('/agent/agent-1');

    expect(screen.getByRole('complementary')).not.toHaveClass('border-r');
  });
});
