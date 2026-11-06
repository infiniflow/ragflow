import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useLocation } from 'react-router';
import { TooltipProvider } from '@/components/ui/tooltip';
import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import Agents from './index';

// Fully isolate the agents list from the request/router/modal layers.
// The two list/builtin hooks are controlled per-test; every other hook the
// rendered subtree touches (AgentCard's dropdown/tag-editor, etc.) is stubbed
// to return an empty object so render never calls into real request code.
jest.mock(
  '@/hooks/use-agent-request',
  () =>
    new Proxy(
      {
        useFetchAgentListByPage: jest.fn(),
        useFetchBuiltinPipelines: jest.fn(),
      },
      {
        get: (target, prop: string) =>
          prop in target
            ? (target as Record<string, unknown>)[prop]
            : jest.fn(() => ({})),
      },
    ),
);
jest.mock('@/hooks/use-compilation-template-group-request', () => ({
  useDeleteCompilationTemplateGroup: () => ({ deleteGroup: jest.fn() }),
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useGoToPreviousPageOnEmpty: jest.fn(),
}));
const mockNavigateToAgentTemplates = jest.fn();
const mockNavigateToCompilationEditor = jest.fn();
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgent: jest.fn(),
    navigateToAgentTemplates: mockNavigateToAgentTemplates,
    navigateToCompilationEditor: mockNavigateToCompilationEditor,
    navigateToBuiltinPipeline: jest.fn(() => jest.fn()),
  }),
}));
// Avoid loading routes.tsx (it builds a data router that needs the `Request`
// global jsdom lacks); the route constants are only passed to mocked navigators.
jest.mock('@/routes', () => ({
  Routes: new Proxy({}, { get: () => '' }),
}));
jest.mock('@/pages/agents/hooks/use-create-agent', () => ({
  useCreateAgentOrPipeline: () => ({
    creatingVisible: false,
    hideCreatingModal: jest.fn(),
    showCreatingModal: jest.fn(),
    loading: false,
    handleCreateAgentOrPipeline: jest.fn(),
  }),
}));
jest.mock('@/pages/agents/hooks/use-select-filters', () => ({
  useSelectFilters: jest.fn(() => []),
}));
const mockHandleImportJson = jest.fn();
jest.mock('@/pages/agents/use-import-json', () => ({
  useHandleImportJsonFile: jest.fn(() => ({
    handleImportJson: mockHandleImportJson,
    fileUploadVisible: false,
    onFileUploadOk: jest.fn(),
    hideFileUploadModal: jest.fn(),
  })),
}));
jest.mock('@/pages/agents/use-rename-agent', () => ({
  useRenameAgent: () => ({
    agentRenameLoading: false,
    initialAgentName: '',
    onAgentRenameOk: jest.fn(),
    agentRenameVisible: false,
    hideAgentRenameModal: jest.fn(),
    showAgentRenameModal: jest.fn(),
  }),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = jest.requireMock('@/hooks/use-agent-request');
// eslint-disable-next-line @typescript-eslint/no-var-requires
const importJson = jest.requireMock('@/pages/agents/use-import-json');

const catalog = [
  {
    id: 'general',
    title: 'General',
    description: 'Default',
    filename: 'a.json',
  },
  { id: 'book', title: 'Book', description: 'Long doc', filename: 'b.json' },
];

let currentSearch = '';
function LocationProbe() {
  currentSearch = useLocation().search;
  return null;
}

function renderAgents(initialEntry = '/') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={new QueryClient()}>
        <TooltipProvider>
          <Agents />
          <LocationProbe />
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

const defaultListPage = {
  data: [] as Array<Record<string, unknown>>,
  loading: false,
  pagination: { current: 1, pageSize: 10, total: 0 },
  setPagination: jest.fn(),
  searchString: '',
  setSearchString: jest.fn(),
  handleInputChange: jest.fn(),
  filterValue: undefined as Record<string, unknown> | undefined,
  setFilterValue: jest.fn(),
  handleFilterSubmit: jest.fn(),
  checkValue: jest.fn(),
  debouncedSearchString: '',
};

function setListPage(overrides: Partial<typeof defaultListPage> = {}) {
  agentRequest.useFetchAgentListByPage.mockReturnValue({
    ...defaultListPage,
    ...overrides,
  });
}

function setBuiltin(canvas = catalog, loading = false) {
  agentRequest.useFetchBuiltinPipelines.mockReturnValue({
    data: { canvas, total: canvas.length },
    loading,
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  setBuiltin();
  setListPage();
  currentSearch = '';
});

describe('Agents tabs', () => {
  it('defaults to the Workflow tab and requests the agent canvas list', () => {
    renderAgents();
    expect(agentRequest.useFetchAgentListByPage).toHaveBeenCalledWith(
      AgentCategory.AgentCanvas,
    );
  });

  it('renders the three tabs in Compilation / Pipeline / Workflow order', () => {
    renderAgents();
    expect(screen.getByText('Compilation operator')).toBeInTheDocument();
    expect(screen.getByText('Ingestion pipeline')).toBeInTheDocument();
    expect(screen.getByText('Workflow')).toBeInTheDocument();
  });

  it('switches the tab through the URL and resets the page', () => {
    renderAgents('/?page=3');
    fireEvent.click(screen.getByText('Ingestion pipeline'));

    expect(currentSearch).toContain(
      `canvas_category=${AgentCategory.DataflowCanvas}`,
    );
    expect(currentSearch).toContain('page=1');
    expect(agentRequest.useFetchAgentListByPage.mock.calls.at(-1)?.[0]).toBe(
      AgentCategory.DataflowCanvas,
    );
  });

  it('falls back to the Workflow tab for an unknown canvas_category', () => {
    renderAgents('/?canvas_category=bogus');
    expect(agentRequest.useFetchAgentListByPage).toHaveBeenCalledWith(
      AgentCategory.AgentCanvas,
    );
  });
});

describe('Agents create actions per tab', () => {
  // The + dropdown and the non-search empty state render the same action
  // list; the empty state exposes them as plain buttons, which jsdom can
  // click reliably (radix dropdowns require pointer-capture polyfills).
  it('shows all three actions on the Workflow tab', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    renderAgents();
    expect(screen.getByText('Create from blank')).toBeInTheDocument();
    expect(screen.getByText('Create from template')).toBeInTheDocument();
    expect(screen.getByText('Import JSON file')).toBeInTheDocument();
    expect(screen.getByTestId('create-agent')).toBeInTheDocument();
  });

  it('shows all three actions on the Pipeline tab', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    // An empty built-in catalog keeps the Pipeline tab on the empty card.
    setBuiltin([]);
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);
    expect(screen.getByText('Create from blank')).toBeInTheDocument();
    expect(screen.getByText('Create from template')).toBeInTheDocument();
    expect(screen.getByText('Import JSON file')).toBeInTheDocument();
  });

  it('shows only blank creation on the Compilation tab', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    renderAgents(
      `/?canvas_category=${AgentListItemType.CompilationTemplateGroup}`,
    );
    expect(screen.getByText('Create from blank')).toBeInTheDocument();
    expect(screen.queryByText('Create from template')).toBeNull();
    expect(screen.queryByText('Import JSON file')).toBeNull();
  });

  it('navigates straight to the compilation editor from the + button (no dropdown)', () => {
    renderAgents(
      `/?canvas_category=${AgentListItemType.CompilationTemplateGroup}`,
    );
    fireEvent.click(screen.getByTestId('create-agent'));
    expect(mockNavigateToCompilationEditor).toHaveBeenCalledWith();
    expect(screen.queryByTestId('agent-create-menu')).toBeNull();
  });

  it('carries the active category when navigating to templates', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    setBuiltin([]);
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);
    fireEvent.click(screen.getByText('Create from template'));
    expect(mockNavigateToAgentTemplates).toHaveBeenCalledWith(
      AgentCategory.DataflowCanvas,
    );
  });

  it('opens the compilation editor for blank creation on the Compilation tab', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    renderAgents(
      `/?canvas_category=${AgentListItemType.CompilationTemplateGroup}`,
    );
    fireEvent.click(screen.getByTestId('agents-empty-create'));
    expect(mockNavigateToCompilationEditor).toHaveBeenCalledWith();
  });

  it('scopes JSON import to the active tab category', () => {
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);
    expect(importJson.useHandleImportJsonFile).toHaveBeenCalledWith(
      AgentCategory.DataflowCanvas,
    );
  });

  it('scopes the filter popover data to the active tab category', () => {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    const selectFilters = jest.requireMock(
      '@/pages/agents/hooks/use-select-filters',
    );
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);
    expect(selectFilters.useSelectFilters).toHaveBeenCalledWith(
      AgentCategory.DataflowCanvas,
    );
  });
});

describe('Agents empty-state (built-in pipeline regression)', () => {
  it('shows the search-empty card on a zero-match search in the Pipeline tab', () => {
    // The bug: builtinVisible (category eligibility) kept the empty
    // CardContainer mounted instead of the search-empty card.
    setListPage({
      data: [],
      debouncedSearchString: 'zzz-no-match',
    });
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);

    expect(screen.getByTestId('agents-search-empty')).toBeInTheDocument();
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('keeps the built-in section when the search matches a built-in pipeline', () => {
    setListPage({
      data: [],
      debouncedSearchString: 'general',
    });
    renderAgents(`/?canvas_category=${AgentCategory.DataflowCanvas}`);

    expect(screen.getByTestId('builtin-pipeline-section')).toBeInTheDocument();
    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
  });

  it('shows the non-search empty card with the tab-scoped actions', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    renderAgents();

    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    // Workflow tab: the empty card exposes all three create actions.
    expect(screen.getByTestId('agents-empty-create')).toBeInTheDocument();
    expect(screen.getByText('Create from template')).toBeInTheDocument();
    expect(screen.getByText('Import JSON file')).toBeInTheDocument();
  });

  it('shows only blank creation in the Compilation empty card', () => {
    setListPage({ data: [], debouncedSearchString: '' });
    renderAgents(
      `/?canvas_category=${AgentListItemType.CompilationTemplateGroup}`,
    );

    expect(screen.getByTestId('agents-empty-create')).toBeInTheDocument();
    expect(screen.queryByText('Create from template')).toBeNull();
    expect(screen.queryByText('Import JSON file')).toBeNull();
  });

  it('shows the search-empty card in the Workflow tab (built-ins not eligible)', () => {
    setListPage({
      data: [],
      debouncedSearchString: 'zzz-no-match',
    });
    renderAgents();

    expect(screen.getByTestId('agents-search-empty')).toBeInTheDocument();
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('renders nothing while the user list is loading', () => {
    setListPage({ data: [], loading: true });
    renderAgents();

    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    expect(screen.queryByTestId('agents-empty-create')).not.toBeInTheDocument();
  });

  it('renders the content branch with user items (and footer) without an empty card', () => {
    setListPage({
      data: [
        {
          id: 'user-1',
          title: 'My Agent',
          canvas_category: AgentCategory.AgentCanvas,
          type: 'agent',
        },
      ],
      debouncedSearchString: '',
    });
    renderAgents();

    // No empty state is shown when there is at least one user item.
    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    expect(screen.queryByTestId('agents-empty-create')).not.toBeInTheDocument();
    // The user card itself is rendered.
    expect(screen.getByText('My Agent')).toBeInTheDocument();
  });
});
